package bot

import (
        "context"
        "fmt"
        "log"
        "strings"
        "time"

        tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

        "schedule-bot/internal/config"
        "schedule-bot/internal/models"
        "schedule-bot/internal/sheets"
        "schedule-bot/internal/storage"
        "schedule-bot/pkg/formatter"
        "schedule-bot/pkg/week"
)

// Bot представляет Telegram бота
type Bot struct {
        bot     *tgbotapi.BotAPI
        config  *config.Config
        storage *storage.Storage
        sheets  *sheets.Client
        logger  *log.Logger
}

// NewBot создаёт новый экземпляр бота
func NewBot(
        bot *tgbotapi.BotAPI,
        cfg *config.Config,
        storage *storage.Storage,
        sheetsClient *sheets.Client,
        logger *log.Logger,
) *Bot {
        return &Bot{
                bot:     bot,
                config:  cfg,
                storage: storage,
                sheets:  sheetsClient,
                logger:  logger,
        }
}

// HandleUpdates обрабатывает обновления от Telegram
func (b *Bot) HandleUpdates(updates tgbotapi.UpdatesChannel) {
        for update := range updates {
                if update.Message != nil {
                        b.handleMessage(update.Message)
                } else if update.CallbackQuery != nil {
                        b.handleCallback(update.CallbackQuery)
                }
        }
}

// handleMessage обрабатывает текстовые сообщения
func (b *Bot) handleMessage(msg *tgbotapi.Message) {
        // Создаём или обновляем пользователя в БД
        user := b.getOrCreateUser(msg.From)

        // Игнорируем команды администратора от обычных пользователей
        if strings.HasPrefix(msg.Text, "/admin") && !b.isAdmin(user) {
                b.sendMessage(msg.Chat.ID, "❌ Эта команда доступна только руководителям")
                return
        }

        // Парсим команду
        command := msg.Command()
        args := msg.CommandArguments()

        switch command {
        case "start":
                b.handleStart(msg, user)
        case "help":
                b.handleHelp(msg)
        case "schedule":
                b.handleSchedule(msg, user, args)
        case "next":
                b.handleNext(msg, user)
        case "search":
                b.handleSearch(msg, args)
        case "notify":
                b.handleNotify(msg, user, args)
        case "changes":
                b.handleChanges(msg, user, args)
        case "admin":
                b.handleAdmin(msg, user, args)
        default:
                // Если это не команда, проверяем, не ввёл ли пользователь код группы
                if user.GroupCode == "" && b.isGroupCode(msg.Text) {
                        b.setGroupForUser(msg, user, msg.Text)
                }
        }
}

// handleCallback обрабатывает callback-запросы от inline-кнопок
func (b *Bot) handleCallback(callback *tgbotapi.CallbackQuery) {
        data := callback.Data

        switch {
        case strings.HasPrefix(data, "group_"):
                groupCode := strings.TrimPrefix(data, "group_")
                b.handleGroupSelection(callback, groupCode)
        case strings.HasPrefix(data, "day_"):
                day := strings.TrimPrefix(data, "day_")
                b.handleDaySelection(callback, day)
        case data == "cancel":
                b.handleCancel(callback)
        }
}

// getOrCreateUser получает или создаёт пользователя в БД
func (b *Bot) getOrCreateUser(from *tgbotapi.User) *models.User {
        user, err := b.storage.GetUserByTelegramID(from.ID)
        if err != nil {
                b.logger.Error("Ошибка получения пользователя", "error", err)
                return nil
        }

        if user == nil {
                // Создаём нового пользователя
                user = &models.User{
                        TelegramID:      from.ID,
                        Username:        from.UserName,
                        FirstName:       from.FirstName,
                        LastName:        from.LastName,
                        IsAdmin:         b.config.IsAdmin(from.ID),
                        NotifyEnabled:   false,
                        NotifyTime:      b.config.NotifyDefaultTime,
                        ChangesSubscribed: false,
                }

                if err := b.storage.CreateUser(user); err != nil {
                        b.logger.Error("Ошибка создания пользователя", "error", err)
                        return nil
                }

                b.logger.Info("Создан новый пользователь", "id", from.ID, "username", from.UserName)
        } else {
                // Обновляем данные пользователя
                user.Username = from.UserName
                user.FirstName = from.FirstName
                user.LastName = from.LastName
                _ = b.storage.CreateUser(user)
        }

        return user
}

// isAdmin проверяет, является ли пользователь администратором
func (b *Bot) isAdmin(user *models.User) bool {
        if user == nil {
                return false
        }
        return user.IsAdmin || b.config.IsAdmin(user.TelegramID)
}

// handleStart обрабатывает команду /start
func (b *Bot) handleStart(msg *tgbotapi.Message, user *models.User) {
        welcomeMsg := formatter.FormatWelcomeMessage(user.FirstName)

        // Если у пользователя уже есть группа, показываем её
        var message string
        if user.GroupCode != "" {
                message = fmt.Sprintf("%s\n\n✅ Ваша текущая группа: *%s*", welcomeMsg, user.GroupCode)
        } else {
                message = welcomeMsg + "\n\n📝 Пожалуйста, выберите группу или введите её код:"
        }

        b.sendMessageWithMarkdown(msg.Chat.ID, message)

        // Показываем список групп
        b.sendGroupList(msg.Chat.ID)
}

// handleHelp обрабатывает команду /help
func (b *Bot) handleHelp(msg *tgbotapi.Message) {
        helpMsg := formatter.FormatHelpMessage()
        b.sendMessageWithMarkdown(msg.Chat.ID, helpMsg)
}

// handleSchedule обрабатывает команду /schedule
func (b *Bot) handleSchedule(msg *tgbotapi.Message, user *models.User, args string) {
        if user.GroupCode == "" {
                b.sendMessage(msg.Chat.ID, "❌ Сначала выберите группу командой /start")
                return
        }

        args = strings.ToLower(strings.TrimSpace(args))

        var targetDate time.Time
        var dateLabel string

        switch args {
        case "завтра", "tomorrow":
                targetDate = time.Now().AddDate(0, 0, 1)
                dateLabel = "завтра"
        case "неделя", "week":
                b.sendWeekSchedule(msg.Chat.ID, user.GroupCode)
                return
        case "":
                targetDate = time.Now()
                dateLabel = "сегодня"
        default:
                b.sendMessage(msg.Chat.ID, "❌ Неизвестный параметр. Используйте: /schedule, /schedule завтра, /schedule неделя")
                return
        }

        dayName := getDayName(targetDate)
        weekType := week.GetWeekTypeString(targetDate)

        lessons, err := b.sheets.GetSchedule(context.Background(), user.GroupCode, weekType)
        if err != nil {
                b.sendMessage(msg.Chat.ID, formatter.FormatErrorMessage(err))
                return
        }

        // Фильтруем по дню
        var dayLessons []models.Lesson
        for _, lesson := range lessons {
                if normalizeDayName(lesson.Day) == dayName {
                        dayLessons = append(dayLessons, lesson)
                }
        }

        message := formatter.FormatSchedule(dayLessons, weekType, dayName)
        b.sendMessageWithMarkdown(msg.Chat.ID, message)
}

// sendWeekSchedule отправляет расписание на неделю
func (b *Bot) sendWeekSchedule(chatID int64, groupCode string) {
        today := time.Now()
        weekType := week.GetWeekTypeString(today)

        lessons, err := b.sheets.GetSchedule(context.Background(), groupCode, weekType)
        if err != nil {
                b.sendMessage(chatID, formatter.FormatErrorMessage(err))
                return
        }

        days := []string{"Понедельник", "Вторник", "Среда", "Четверг", "Пятница", "Суббота"}

        for _, dayName := range days {
                var dayLessons []models.Lesson
                for _, lesson := range lessons {
                        if normalizeDayName(lesson.Day) == dayName {
                                dayLessons = append(dayLessons, lesson)
                        }
                }

                message := formatter.FormatSchedule(dayLessons, weekType, dayName)
                b.sendMessageWithMarkdown(chatID, message)
                time.Sleep(500 * time.Millisecond) // Небольшая задержка между сообщениями
        }
}

// handleNext обрабатывает команду /next
func (b *Bot) handleNext(msg *tgbotapi.Message, user *models.User) {
        if user.GroupCode == "" {
                b.sendMessage(msg.Chat.ID, "❌ Сначала выберите группу командой /start")
                return
        }

        now := time.Now()
        weekType := week.GetWeekTypeString(now)
        dayName := getDayName(now)

        lessons, err := b.sheets.GetSchedule(context.Background(), user.GroupCode, weekType)
        if err != nil {
                b.sendMessage(msg.Chat.ID, formatter.FormatErrorMessage(err))
                return
        }

        // Фильтруем по сегодняшнему дню
        var todayLessons []models.Lesson
        for _, lesson := range lessons {
                if normalizeDayName(lesson.Day) == dayName {
                        todayLessons = append(todayLessons, lesson)
                }
        }

        // Находим следующую пару
        nextLesson := findNextLesson(todayLessons, now)

        var minutesUntil int
        if nextLesson != nil {
                lessonTime, _ := parseTime(nextLesson.Time)
                minutesUntil = int(lessonTime.Sub(now).Minutes())
        }

        message := formatter.FormatNextLesson(nextLesson, minutesUntil)
        b.sendMessageWithMarkdown(msg.Chat.ID, message)
}

// handleSearch обрабатывает команду /search
func (b *Bot) handleSearch(msg *tgbotapi.Message, args string) {
        if args == "" {
                b.sendMessage(msg.Chat.ID, "❌ Введите код группы для поиска. Пример: /search 8251160")
                return
        }

        groupCode := strings.TrimSpace(args)

        // Получаем список всех групп
        groups, err := b.sheets.GetGroups(context.Background())
        if err != nil {
                b.sendMessage(msg.Chat.ID, formatter.FormatErrorMessage(err))
                return
        }

        // Ищем группу
        var found bool
        for _, group := range groups {
                if group.Code == groupCode {
                        found = true
                        break
                }
        }

        if !found {
                b.sendMessage(msg.Chat.ID, fmt.Sprintf("❌ Группа %s не найдена", groupCode))
                return
        }

        // Предлагаем выбрать эту группу
        keyboard := tgbotapi.NewInlineKeyboardMarkup(
                tgbotapi.NewInlineKeyboardRow(
                        tgbotapi.NewInlineKeyboardButtonData("✅ Выбрать эту группу", "group_"+groupCode),
                ),
        )

        b.sendMessageWithKeyboard(msg.Chat.ID, fmt.Sprintf("🔍 Найдена группа: *%s*\n\nНажмите кнопку, чтобы выбрать её.", groupCode), keyboard)
}

// handleNotify обрабатывает команду /notify
func (b *Bot) handleNotify(msg *tgbotapi.Message, user *models.User, args string) {
        args = strings.ToLower(strings.TrimSpace(args))

        var response string

        switch {
        case args == "on", args == "вкл":
                err := b.storage.SetNotifySettings(user.TelegramID, true, user.NotifyTime)
                if err != nil {
                        b.sendMessage(msg.Chat.ID, formatter.FormatErrorMessage(err))
                        return
                }
                response = "✅ Уведомления включены. Вы будете получать расписание на завтра каждый день."

        case args == "off", args == "выкл":
                err := b.storage.SetNotifySettings(user.TelegramID, false, user.NotifyTime)
                if err != nil {
                        b.sendMessage(msg.Chat.ID, formatter.FormatErrorMessage(err))
                        return
                }
                response = "🔕 Уведомления выключены."

        case strings.HasPrefix(args, "time"):
                parts := strings.SplitN(args, " ", 2)
                if len(parts) < 2 {
                        b.sendMessage(msg.Chat.ID, "❌ Укажите время в формате HH:MM. Пример: /notify time 20:00")
                        return
                }

                newTime := strings.TrimSpace(parts[1])
                if !isValidTime(newTime) {
                        b.sendMessage(msg.Chat.ID, "❌ Неверный формат времени. Используйте HH:MM (например, 20:00)")
                        return
                }

                err := b.storage.SetNotifySettings(user.TelegramID, user.NotifyEnabled, newTime)
                if err != nil {
                        b.sendMessage(msg.Chat.ID, formatter.FormatErrorMessage(err))
                        return
                }
                response = fmt.Sprintf("⏰ Время уведомлений установлено на %s", newTime)

        default:
                response = "🔔 *Управление уведомлениями*\n\n" +
                        "/notify on - Включить уведомления\n" +
                        "/notify off - Выключить уведомления\n" +
                        "/notify time HH:MM - Установить время (например, /notify time 20:00)"
        }

        b.sendMessageWithMarkdown(msg.Chat.ID, response)
}

// handleChanges обрабатывает команду /changes
func (b *Bot) handleChanges(msg *tgbotapi.Message, user *models.User, args string) {
        args = strings.ToLower(strings.TrimSpace(args))

        if args == "subscribe" || args == "подписаться" {
                err := b.storage.SetChangesSubscription(user.TelegramID, true)
                if err != nil {
                        b.sendMessage(msg.Chat.ID, formatter.FormatErrorMessage(err))
                        return
                }
                b.sendMessage(msg.Chat.ID, "✅ Вы подписаны на изменения в расписании. Вы будете получать уведомления при любых изменениях.")
                return
        }

        if args == "unsubscribe" || args == "отписаться" {
                err := b.storage.SetChangesSubscription(user.TelegramID, false)
                if err != nil {
                        b.sendMessage(msg.Chat.ID, formatter.FormatErrorMessage(err))
                        return
                }
                b.sendMessage(msg.Chat.ID, "🔕 Вы отписались от уведомлений об изменениях.")
                return
        }

        b.sendMessageWithMarkdown(msg.Chat.ID,
                "🔔 *Подписка на изменения*\n\n"+
                        "/changes subscribe - Подписаться на изменения\n"+
                        "/changes unsubscribe - Отписаться от изменений")
}

// handleAdmin обрабатывает административные команды
func (b *Bot) handleAdmin(msg *tgbotapi.Message, user *models.User, args string) {
        if !b.isAdmin(user) {
                b.sendMessage(msg.Chat.ID, "❌ Эта команда доступна только руководителям")
                return
        }

        parts := strings.SplitN(args, " ", 2)
        if len(parts) == 0 {
                b.sendMessage(msg.Chat.ID, "❌ Укажите подкоманду: sync, edit, broadcast, logs")
                return
        }

        subcommand := parts[0]
        subargs := ""
        if len(parts) > 1 {
                subargs = parts[1]
        }

        switch subcommand {
        case "sync":
                b.handleAdminSync(msg)
        case "edit":
                b.handleAdminEdit(msg, subargs)
        case "broadcast":
                b.handleAdminBroadcast(msg, subargs)
        case "logs":
                b.handleAdminLogs(msg)
        default:
                b.sendMessage(msg.Chat.ID, "❌ Неизвестная подкоманда. Доступные: sync, edit, broadcast, logs")
        }
}

// handleAdminSync обрабатывает /admin sync
func (b *Bot) handleAdminSync(msg *tgbotapi.Message) {
        b.sheets.ClearCache()

        // Принудительно загружаем группы
        _, err := b.sheets.GetGroups(context.Background())
        if err != nil {
                b.sendMessageWithMarkdown(msg.Chat.ID, formatter.FormatAdminMessage("Синхронизация", fmt.Sprintf("❌ Ошибка: %v", err)))
                return
        }

        b.sendMessageWithMarkdown(msg.Chat.ID, formatter.FormatAdminMessage("Синхронизация", "✅ Кэш очищен и данные обновлены"))
}

// handleAdminEdit обрабатывает /admin edit
func (b *Bot) handleAdminEdit(msg *tgbotapi.Message, args string) {
        // Простая реализация - в будущем можно добавить inline-редактирование
        b.sendMessageWithMarkdown(msg.Chat.ID,
                formatter.FormatAdminMessage("Редактирование",
                        "🚧 Функция редактирования в разработке...\n\n"+
                                "Используйте: /admin edit <группа> <день>\n"+
                                "Пример: /admin edit 8251160 Понедельник"))
}

// handleAdminBroadcast обрабатывает /admin broadcast
func (b *Bot) handleAdminBroadcast(msg *tgbotapi.Message, text string) {
        if text == "" {
                b.sendMessage(msg.Chat.ID, "❌ Укажите текст рассылки. Пример: /admin broadcast Важное объявление!")
                return
        }

        users, err := b.storage.GetAllUsers()
        if err != nil {
                b.sendMessage(msg.Chat.ID, formatter.FormatErrorMessage(err))
                return
        }

        sentCount := 0
        failedCount := 0

        for _, user := range users {
                msgToSend := tgbotapi.NewMessage(user.TelegramID, "📢 *Объявление от администрации*\n\n"+text)
                msgToSend.ParseMode = tgbotapi.ModeMarkdown

                _, err := b.bot.Send(msgToSend)
                if err != nil {
                        failedCount++
                        b.logger.Error("Ошибка рассылки", "error", err, "user", user.TelegramID)
                } else {
                        sentCount++
                }
                time.Sleep(50 * time.Millisecond) // Небольшая задержка
        }

        b.sendMessageWithMarkdown(msg.Chat.ID,
                fmt.Sprintf("✅ Рассылка завершена\n\nОтправлено: %d\nНе доставлено: %d", sentCount, failedCount))
}

// handleAdminLogs обрабатывает /admin logs
func (b *Bot) handleAdminLogs(msg *tgbotapi.Message) {
        changes, err := b.storage.GetRecentChanges(20)
        if err != nil {
                b.sendMessage(msg.Chat.ID, formatter.FormatErrorMessage(err))
                return
        }

        if len(changes) == 0 {
                b.sendMessage(msg.Chat.ID, "📭 За последние изменения не найдено")
                return
        }

        var sb strings.Builder
        sb.WriteString("📋 *Последние изменения в расписании*\n\n")

        for i, change := range changes {
                if i >= 10 {
                        sb.WriteString("\n_... и другие изменения_")
                        break
                }
                sb.WriteString(fmt.Sprintf("📍 %s, %s, %s\n", change.GroupCode, change.Day, change.Time))
                sb.WriteString(fmt.Sprintf("❌ %s\n", change.OldValue))
                sb.WriteString(fmt.Sprintf("✅ %s\n\n", change.NewValue))
        }

        b.sendMessageWithMarkdown(msg.Chat.ID, sb.String())
}

// handleGroupSelection обрабатывает выбор группы
func (b *Bot) handleGroupSelection(callback *tgbotapi.CallbackQuery, groupCode string) {
        user, err := b.storage.GetUserByTelegramID(callback.From.ID)
        if err != nil {
                b.logger.Error("Ошибка получения пользователя", "error", err)
                return
        }

        if user == nil {
                return
        }

        err = b.storage.SetUserGroup(user.TelegramID, groupCode)
        if err != nil {
                b.logger.Error("Ошибка установки группы", "error", err)
                return
        }

        // Отвечаем на callback
        answer := tgbotapi.NewCallback(callback.ID, fmt.Sprintf("Группа %s выбрана!", groupCode))
        _, _ = b.bot.AnswerCallbackQuery(answer)

        // Обновляем сообщение
        editMsg := tgbotapi.NewEditMessageText(callback.Message.Chat.ID, callback.Message.MessageID,
                fmt.Sprintf("✅ Вы выбрали группу: *%s*\n\nТеперь используйте /schedule для просмотра расписания.", groupCode))
        editMsg.ParseMode = tgbotapi.ModeMarkdown
        _, _ = b.bot.Send(editMsg)
}

// handleDaySelection обрабатывает выбор дня (для будущего функционала)
func (b *Bot) handleDaySelection(callback *tgbotapi.CallbackQuery, day string) {
        answer := tgbotapi.NewCallback(callback.ID, "Выбран день: "+day)
        _, _ = b.bot.AnswerCallbackQuery(answer)
}

// handleCancel обрабатывает отмену
func (b *Bot) handleCancel(callback *tgbotapi.CallbackQuery) {
        answer := tgbotapi.NewCallback(callback.ID, "Отменено")
        _, _ = b.bot.AnswerCallbackQuery(answer)

        editMsg := tgbotapi.NewEditMessageText(callback.Message.Chat.ID, callback.Message.MessageID, "❌ Отменено")
        _, _ = b.bot.Send(editMsg)
}

// sendGroupList отправляет список групп
func (b *Bot) sendGroupList(chatID int64) {
        groups, err := b.sheets.GetGroups(context.Background())
        if err != nil {
                b.sendMessage(chatID, formatter.FormatErrorMessage(err))
                return
        }

        // Создаём inline-клавиатуру с группами
        var rows [][]tgbotapi.InlineKeyboardButton

        for _, group := range groups {
                row := tgbotapi.NewInlineKeyboardRow(
                        tgbotapi.NewInlineKeyboardButtonData(
                                fmt.Sprintf("%s (%s)", group.Code, group.Specialty),
                                "group_"+group.Code,
                        ),
                )
                rows = append(rows, row)
        }

        keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)

        b.sendMessageWithKeyboard(chatID, "📋 *Доступные группы:*\n\nВыберите вашу группу из списка:", keyboard)
}

// setGroupForUser устанавливает группу для пользователя
func (b *Bot) setGroupForUser(msg *tgbotapi.Message, user *models.User, groupCode string) {
        // Проверяем, существует ли такая группа
        groups, err := b.sheets.GetGroups(context.Background())
        if err != nil {
                b.sendMessage(msg.Chat.ID, formatter.FormatErrorMessage(err))
                return
        }

        found := false
        for _, group := range groups {
                if group.Code == groupCode {
                        found = true
                        break
                }
        }

        if !found {
                b.sendMessage(msg.Chat.ID, fmt.Sprintf("❌ Группа %s не найдена. Пожалуйста, выберите из списка.", groupCode))
                return
        }

        err = b.storage.SetUserGroup(user.TelegramID, groupCode)
        if err != nil {
                b.sendMessage(msg.Chat.ID, formatter.FormatErrorMessage(err))
                return
        }

        b.sendMessageWithMarkdown(msg.Chat.ID, fmt.Sprintf("✅ Вы выбрали группу: *%s*\n\nТеперь используйте /schedule для просмотра расписания.", groupCode))
}

// isGroupCode проверяет, является ли текст кодом группы
func (b *Bot) isGroupCode(text string) bool {
        text = strings.TrimSpace(text)
        // Код группы обычно 7 цифр
        if len(text) != 7 {
                return false
        }
        for _, c := range text {
                if c < '0' || c > '9' {
                        return false
                }
        }
        return true
}

// Вспомогательные функции

func (b *Bot) sendMessage(chatID int64, text string) {
        msg := tgbotapi.NewMessage(chatID, text)
        _, _ = b.bot.Send(msg)
}

func (b *Bot) sendMessageWithMarkdown(chatID int64, text string) {
        msg := tgbotapi.NewMessage(chatID, text)
        msg.ParseMode = tgbotapi.ModeMarkdown
        _, _ = b.bot.Send(msg)
}

func (b *Bot) sendMessageWithKeyboard(chatID int64, text string, keyboard tgbotapi.InlineKeyboardMarkup) {
        msg := tgbotapi.NewMessage(chatID, text)
        msg.ParseMode = tgbotapi.ModeMarkdown
        msg.ReplyMarkup = keyboard
        _, _ = b.bot.Send(msg)
}

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

        if len(day) > 0 {
                return strings.ToUpper(string(day[0])) + day[1:]
        }

        return day
}

func findNextLesson(lessons []models.Lesson, now time.Time) *models.Lesson {
        currentTime := now.Hour()*60 + now.Minute()

        var nextLesson *models.Lesson
        minDiff := 24 * 60 // Максимальная разница

        for _, lesson := range lessons {
                lessonTime, err := parseTime(lesson.Time)
                if err != nil {
                        continue
                }

                lessonMinutes := lessonTime.Hour()*60 + lessonTime.Minute()
                diff := lessonMinutes - currentTime

                if diff > 0 && diff < minDiff {
                        minDiff = diff
                        nextLesson = &lesson
                }
        }

        return nextLesson
}

func parseTime(timeStr string) (time.Time, error) {
        now := time.Now()

        // Пробуем разные форматы
        formats := []string{"15:04", "15:04:05", "3:04 PM"}

        for _, format := range formats {
                t, err := time.ParseInLocation(format, timeStr, now.Location())
                if err == nil {
                        return time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), t.Second(), 0, now.Location()), nil
                }
        }

        return time.Time{}, fmt.Errorf("неверный формат времени: %s", timeStr)
}

func isValidTime(timeStr string) bool {
        _, err := time.Parse("15:04", timeStr)
        return err == nil
}