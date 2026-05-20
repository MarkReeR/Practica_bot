package scheduler

import (
        "context"
        "fmt"
        "log"
        "strings"
        "time"

        tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

        "schedule-bot/internal/models"
        "schedule-bot/internal/sheets"
        "schedule-bot/internal/storage"
        "schedule-bot/pkg/formatter"
        "schedule-bot/pkg/week"
)

// Scheduler отвечает за планирование и отправку уведомлений
type Scheduler struct {
        bot     *tgbotapi.BotAPI
        storage *storage.Storage
        sheets  *sheets.Client
        logger  *log.Logger
        done    chan struct{}
}

// NewScheduler создаёт новый планировщик
func NewScheduler(
        bot *tgbotapi.BotAPI,
        storage *storage.Storage,
        sheetsClient *sheets.Client,
        logger *log.Logger,
) *Scheduler {
        return &Scheduler{
                bot:     bot,
                storage: storage,
                sheets:  sheetsClient,
                logger:  logger,
                done:    make(chan struct{}),
        }
}

// Start запускает планировщик
func (s *Scheduler) Start(ctx context.Context, defaultNotifyTime string) {
        s.logger.Info("Планировщик запущен")

        // Таймер для ежедневных уведомлений
        dailyTicker := time.NewTicker(1 * time.Minute)
        defer dailyTicker.Stop()

        // Таймер для проверки изменений в расписании
        checkTicker := time.NewTicker(5 * time.Minute)
        defer checkTicker.Stop()

        for {
                select {
                case <-ctx.Done():
                        s.logger.Info("Планировщик остановлен")
                        return
                case <-s.done:
                        s.logger.Info("Планировщик остановлен по сигналу")
                        return
                case t := <-dailyTicker.C:
                        s.sendDailyNotifications(t, defaultNotifyTime)
                case <-checkTicker.C:
                        s.checkScheduleChanges(ctx)
                }
        }
}

// Stop останавливает планировщик
func (s *Scheduler) Stop() {
        close(s.done)
}

// sendDailyNotifications отправляет ежедневные уведомления
func (s *Scheduler) sendDailyNotifications(now time.Time, defaultTime string) {
        currentTime := now.Format("15:04")

        // Получаем пользователей с включенными уведомлениями
        users, err := s.storage.GetUsersWithNotify()
        if err != nil {
                s.logger.Error("Ошибка получения пользователей с уведомлениями", "error", err)
                return
        }

        for _, user := range users {
                // Проверяем время уведомления пользователя
                notifyTime := user.NotifyTime
                if notifyTime == "" {
                        notifyTime = defaultTime
                }

                // Если текущее время совпадает со временем уведомления
                if currentTime == notifyTime {
                        s.sendTomorrowSchedule(user)
                }
        }
}

// sendTomorrowSchedule отправляет расписание на завтра
func (s *Scheduler) sendTomorrowSchedule(user models.User) {
        if user.GroupCode == "" {
                s.logger.Debug("У пользователя не установлена группа", "telegram_id", user.TelegramID)
                return
        }

        // Определяем завтрашний день
        tomorrow := time.Now().AddDate(0, 0, 1)
        dayName := getDayName(tomorrow)
        weekType := week.GetWeekTypeString(tomorrow)

        // Получаем расписание
        lessons, err := s.sheets.GetSchedule(context.Background(), user.GroupCode, weekType)
        if err != nil {
                s.logger.Error("Ошибка получения расписания", "error", err)

                msg := tgbotapi.NewMessage(user.TelegramID,
                        fmt.Sprintf("🔔 *Напоминание*\n\nЗавтра %s (%s неделя)\n\n❌ Не удалось загрузить расписание. Попробуйте позже.",
                                dayName, weekTypeLabel(weekType)))
                msg.ParseMode = tgbotapi.ModeMarkdown
                _, _ = s.bot.Send(msg)
                return
        }

        // Фильтруем по дню
        var tomorrowLessons []models.Lesson
        for _, lesson := range lessons {
                if normalizeDayName(lesson.Day) == dayName {
                        tomorrowLessons = append(tomorrowLessons, lesson)
                }
        }

        message := formatter.FormatSchedule(tomorrowLessons, weekType, dayName)

        msg := tgbotapi.NewMessage(user.TelegramID, "🔔 *Напоминание о завтрашнем дне*\n\n"+message)
        msg.ParseMode = tgbotapi.ModeMarkdown

        _, err = s.bot.Send(msg)
        if err != nil {
                s.logger.Error("Ошибка отправки уведомления", "error", err, "user", user.TelegramID)
        } else {
                s.logger.Info("Отправлено ежедневное уведомление", "user", user.TelegramID)
        }
}

// checkScheduleChanges проверяет изменения в расписании
func (s *Scheduler) checkScheduleChanges(ctx context.Context) {
        s.logger.Debug("Проверка изменений в расписании")

        // Получаем группы из кэша или загружаем новые
        groups, err := s.sheets.GetGroups(ctx)
        if err != nil {
                s.logger.Error("Ошибка получения списка групп", "error", err)
                return
        }

        // Для каждой группы проверяем изменения
        for _, group := range groups {
                s.checkGroupChanges(group.Code)
        }
}

// checkGroupChanges проверяет изменения для конкретной группы
func (s *Scheduler) checkGroupChanges(groupCode string) {
        weekTypes := []string{"в", "н"}

        for _, wt := range weekTypes {
                // Получаем текущее расписание
                currentLessons, err := s.sheets.GetSchedule(context.Background(), groupCode, wt)
                if err != nil {
                        continue
                }

                // Получаем кэшированное расписание
                cachedLessons, _ := s.storage.GetScheduleCache(groupCode, wt)

                // Сравниваем
                if cachedLessons != nil && len(cachedLessons) > 0 {
                        changes := detectChanges(cachedLessons, currentLessons)

                        if len(changes) > 0 {
                                s.logger.Info("Обнаружены изменения в расписании",
                                        "group", groupCode, "week", wt, "count", len(changes))

                                // Логируем изменения
                                for _, change := range changes {
                                        _ = s.storage.LogChange(groupCode, change.Day, change.Time, change.OldValue, change.NewValue)
                                }

                                // Уведомляем подписчиков
                                s.notifyAboutChanges(groupCode, changes)
                        }
                }

                // Обновляем кэш
                data, err := sheets.MarshalLessons(currentLessons)
                if err == nil {
                        _ = s.storage.SetScheduleCache(groupCode, wt, data, time.Now().Add(10*time.Minute))
                }
        }
}

// detectChanges обнаруживает изменения между двумя версиями расписания
func detectChanges(old, new []models.Lesson) []models.ChangeLog {
        var changes []models.ChangeLog

        oldMap := make(map[string]models.Lesson)
        for _, l := range old {
                key := fmt.Sprintf("%s_%s", l.Day, l.Time)
                oldMap[key] = l
        }

        newMap := make(map[string]models.Lesson)
        for _, l := range new {
                key := fmt.Sprintf("%s_%s", l.Day, l.Time)
                newMap[key] = l
        }

        // Проверяем изменения в существующих занятиях
        for key, oldLesson := range oldMap {
                newLesson, exists := newMap[key]
                if !exists {
                        // Занятие удалено
                        changes = append(changes, models.ChangeLog{
                                GroupCode: oldLesson.GroupCode,
                                Day:       oldLesson.Day,
                                Time:      oldLesson.Time,
                                OldValue:  formatLessonShort(oldLesson),
                                NewValue:  "(удалено)",
                        })
                        continue
                }

                // Проверяем, изменилось ли занятие
                if lessonsDifferent(oldLesson, newLesson) {
                        changes = append(changes, models.ChangeLog{
                                GroupCode: oldLesson.GroupCode,
                                Day:       oldLesson.Day,
                                Time:      oldLesson.Time,
                                OldValue:  formatLessonShort(oldLesson),
                                NewValue:  formatLessonShort(newLesson),
                        })
                }
        }

        // Проверяем новые занятия
        for key, newLesson := range newMap {
                if _, exists := oldMap[key]; !exists {
                        // Новое занятие
                        _ = key // suppress unused warning
                        changes = append(changes, models.ChangeLog{
                                GroupCode: newLesson.GroupCode,
                                Day:       newLesson.Day,
                                Time:      newLesson.Time,
                                OldValue:  "(не было)",
                                NewValue:  formatLessonShort(newLesson),
                        })
                }
        }

        return changes
}

// lessonsDifferent проверяет, различаются ли два занятия
func lessonsDifferent(a, b models.Lesson) bool {
        return a.Discipline != b.Discipline ||
                a.LessonType != b.LessonType ||
                a.Building != b.Building ||
                a.Classroom != b.Classroom ||
                a.Teacher != b.Teacher ||
                a.Notes != b.Notes
}

// formatLessonShort форматирует занятие для отображения в изменении
func formatLessonShort(lesson models.Lesson) string {
        parts := []string{lesson.Discipline}
        if lesson.LessonType != "" {
                parts = append(parts, lesson.LessonType)
        }
        if lesson.Classroom != "" {
                parts = append(parts, fmt.Sprintf("ауд.%s", lesson.Classroom))
        }
        return strings.Join(parts, " | ")
}

// notifyAboutChanges отправляет уведомления об изменениях
func (s *Scheduler) notifyAboutChanges(groupCode string, changes []models.ChangeLog) {
        // Получаем всех подписчиков на изменения
        users, err := s.storage.GetUsersSubscribedToChanges()
        if err != nil {
                s.logger.Error("Ошибка получения подписчиков на изменения", "error", err)
                return
        }

        // Фильтруем пользователей по группе
        var targetUsers []models.User
        for _, user := range users {
                if user.GroupCode == groupCode {
                        targetUsers = append(targetUsers, user)
                }
        }

        if len(targetUsers) == 0 {
                return
        }

        // Формируем сообщение об изменениях
        var sb strings.Builder
        sb.WriteString("🔔 *Изменения в расписании!*\n\n")
        sb.WriteString(fmt.Sprintf("Группа: *%s*\n\n", groupCode))

        for i, change := range changes {
                if i >= 10 { // Ограничиваем количество изменений в сообщении
                        sb.WriteString("\n_... и другие изменения_")
                        break
                }
                sb.WriteString(fmt.Sprintf("📍 %s, %s\n", change.Day, change.Time))
                sb.WriteString(fmt.Sprintf("❌ %s\n", change.OldValue))
                sb.WriteString(fmt.Sprintf("✅ %s\n\n", change.NewValue))
        }

        message := sb.String()

        // Отправляем каждому пользователю
        for _, user := range targetUsers {
                msg := tgbotapi.NewMessage(user.TelegramID, message)
                msg.ParseMode = tgbotapi.ModeMarkdown

                _, err := s.bot.Send(msg)
                if err != nil {
                        s.logger.Error("Ошибка отправки уведомления об изменении",
                                "error", err, "user", user.TelegramID)
                }
        }
}

// getDayName возвращает название дня недели
func getDayName(t time.Time) string {
        switch t.Weekday() {
        case time.Monday:
                return "Понедельник"
        case time.Tuesday:
                return "Вторник"
        case time.Wednesday:
                return "Среда"
        case time.Thursday:
                return "Четверг"
        case time.Friday:
                return "Пятница"
        case time.Saturday:
                return "Суббота"
        case time.Sunday:
                return "Воскресенье"
        default:
                return ""
        }
}

// normalizeDayName нормализует название дня
func normalizeDayName(day string) string {
        day = strings.TrimSpace(strings.ToLower(day))

        dayAliases := map[string]string{
                "понедельник": "Понедельник",
                "вторник":     "Вторник",
                "среда":       "Среда",
                "четверг":     "Четверг",
                "пятница":     "Пятница",
                "суббота":     "Суббота",
                "воскресенье": "Воскресенье",
                "пн":          "Понедельник",
                "вт":          "Вторник",
                "ср":          "Среда",
                "чт":          "Четверг",
                "пт":          "Пятница",
                "сб":          "Суббота",
                "вс":          "Воскресенье",
        }

        if normalized, ok := dayAliases[day]; ok {
                return normalized
        }

        // Capitalize first letter
        if len(day) > 0 {
                return strings.ToUpper(string(day[0])) + day[1:]
        }

        return day
}

// weekTypeLabel возвращает читаемое название типа недели
func weekTypeLabel(wt string) string {
        switch wt {
        case "в":
                return "Верхняя"
        case "н":
                return "Нижняя"
        default:
                return wt
        }
}