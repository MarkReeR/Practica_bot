package formatter

import (
        "fmt"
        "strings"

        "schedule-bot/internal/models"
)

// FormatSchedule форматирует расписание для отправки в Telegram
func FormatSchedule(lessons []models.Lesson, weekType string, dayName string) string {
        if len(lessons) == 0 {
                return "📭 На этот день занятий нет"
        }

        var sb strings.Builder

        sb.WriteString(fmt.Sprintf("📅 *Расписание на %s*\n", dayName))
        sb.WriteString(fmt.Sprintf("📌 Неделя: *%s*\n\n", getWeekTypeLabel(weekType)))

        // Группируем занятия по времени
        timeLessons := make(map[string][]models.Lesson)
        for _, lesson := range lessons {
                timeLessons[lesson.Time] = append(timeLessons[lesson.Time], lesson)
        }

        // Сортируем время
        times := []string{"8:00", "9:40", "11:50", "13:30", "15:40", "17:20"}

        for _, timeSlot := range times {
                ls, ok := timeLessons[timeSlot]
                if !ok {
                        continue
                }

                sb.WriteString(fmt.Sprintf("⏰ *%s*\n", timeSlot))

                for _, lesson := range ls {
                        sb.WriteString(formatLesson(lesson))
                        sb.WriteString("\n")
                }
                sb.WriteString("\n")
        }

        return sb.String()
}

// formatLesson форматирует одно занятие
func formatLesson(lesson models.Lesson) string {
        var sb strings.Builder

        // Дисциплина и тип
        sb.WriteString(fmt.Sprintf("📚 *%s*", lesson.Discipline))
        if lesson.LessonType != "" {
                sb.WriteString(fmt.Sprintf(" (%s)", getLessonTypeLabel(lesson.LessonType)))
        }
        sb.WriteString("\n")

        // Преподаватель
        if lesson.Teacher != "" {
                sb.WriteString(fmt.Sprintf("👨‍🏫 %s\n", lesson.Teacher))
        }

        // Место проведения
        if lesson.Building != "" || lesson.Classroom != "" {
                location := ""
                if lesson.Building != "" {
                        location = lesson.Building
                }
                if lesson.Classroom != "" {
                        if location != "" {
                                location += fmt.Sprintf(", ауд. %s", lesson.Classroom)
                        } else {
                                location = fmt.Sprintf("ауд. %s", lesson.Classroom)
                        }
                }
                sb.WriteString(fmt.Sprintf("📍 %s\n", location))
        }

        // Кафедра
        if lesson.Department != "" {
                sb.WriteString(fmt.Sprintf("🏛 %s\n", lesson.Department))
        }

        // Примечания
        if lesson.Notes != "" {
                sb.WriteString(fmt.Sprintf("⚠️ %s\n", lesson.Notes))
        }

        return sb.String()
}

// FormatGroupList форматирует список групп для выбора
func FormatGroupList(groups []models.Group) string {
        if len(groups) == 0 {
                return "❌ Группы не найдены"
        }

        var sb strings.Builder
        sb.WriteString("📋 *Доступные группы:*\n\n")

        for i, group := range groups {
                sb.WriteString(fmt.Sprintf("%d. *%s* (%s)\n", i+1, group.Code, group.Specialty))
        }

        sb.WriteString("\n💡 Введите код группы или выберите из списка")
        return sb.String()
}

// FormatWelcomeMessage создаёт приветственное сообщение
func FormatWelcomeMessage(firstName string) string {
        return fmt.Sprintf(
                "👋 Привет, %s!\n\n"+
                        "Я бот-расписание университета.\n\n"+
                        "📌 *Доступные команды:*\n"+
                        "/start - Начать работу, выбрать группу\n"+
                        "/schedule - Показать расписание на сегодня\n"+
                        "/schedule завтра - Расписание на завтра\n"+
                        "/schedule неделя - Расписание на неделю\n"+
                        "/next - Следующая пара\n"+
                        "/search <код группы> - Найти группу\n"+
                        "/notify on - Включить уведомления\n"+
                        "/notify off - Выключить уведомления\n"+
                        "/notify time <HH:MM> - Установить время уведомлений\n"+
                        "/changes subscribe - Подписаться на изменения\n"+
                        "/help - Помощь\n\n"+
                        "🔧 *Для руководителей:*\n"+
                        "/admin sync - Синхронизировать расписание\n"+
                        "/admin edit <группа> <день> - Редактировать\n"+
                        "/admin broadcast <текст> - Рассылка\n"+
                        "/admin logs - Журнал изменений",
                firstName,
        )
}

// FormatHelpMessage создаёт сообщение с помощью
func FormatHelpMessage() string {
        return "ℹ️ *Помощь по использованию бота*\n\n" +
                "📅 *Просмотр расписания:*\n" +
                "• /schedule - расписание на сегодня\n" +
                "• /schedule завтра - расписание на завтра\n" +
                "• /schedule неделя - расписание на всю неделю\n" +
                "• /next - информация о следующей паре\n\n" +
                "🔔 *Уведомления:*\n" +
                "• /notify on - включить ежедневные уведомления\n" +
                "• /notify off - выключить уведомления\n" +
                "• /notify time 20:00 - установить время напоминаний\n" +
                "• /changes subscribe - получать уведомления об изменениях\n\n" +
                "🔍 *Поиск:*\n" +
                "• /search 8251160 - найти группу по коду\n\n" +
                "👨‍💼 *Для руководителей:*\n" +
                "• /admin sync - принудительная синхронизация\n" +
                "• /admin edit - редактирование расписания\n" +
                "• /admin broadcast - рассылка сообщений\n" +
                "• /admin logs - просмотр журнала изменений"
}

// FormatNextLesson форматирует информацию о следующей паре
func FormatNextLesson(lesson *models.Lesson, minutesUntil int) string {
        if lesson == nil {
                return "✅ На сегодня больше нет занятий"
        }

        var sb strings.Builder

        if minutesUntil > 0 {
                sb.WriteString(fmt.Sprintf("⏰ *Следующая пара через %d минут!*\n\n", minutesUntil))
        } else {
                sb.WriteString("⏰ *Следующая пара:*\n\n")
        }

        sb.WriteString(formatLesson(*lesson))

        return sb.String()
}

// FormatAdminMessage форматирует сообщение для администратора
func FormatAdminMessage(action, details string) string {
        return fmt.Sprintf("🔧 *Администрирование*\n\n*Действие:* %s\n%s", action, details)
}

// FormatErrorMessage форматирует сообщение об ошибке
func FormatErrorMessage(err error) string {
        return fmt.Sprintf("❌ *Ошибка:* %s\n\nПопробуйте позже или обратитесь к администратору.", err.Error())
}

// getWeekTypeLabel возвращает читаемое название типа недели
func getWeekTypeLabel(weekType string) string {
        switch weekType {
        case "в":
                return "Верхняя"
        case "н":
                return "Нижняя"
        default:
                return weekType
        }
}

// getLessonTypeLabel возвращает читаемое название типа занятия
func getLessonTypeLabel(lessonType string) string {
        switch strings.ToLower(lessonType) {
        case "лек":
                return "Лекция"
        case "прак":
                return "Практика"
        case "лаб":
                return "Лабораторная"
        case "кз":
                return "Контрольный замер"
        default:
                return lessonType
        }
}