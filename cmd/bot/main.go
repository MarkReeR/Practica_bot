package main

import (
        "context"
        "fmt"
        "log"
        "os"
        "os/signal"
        "syscall"
        "time"

        tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

        "schedule-bot/internal/bot"
        "schedule-bot/internal/config"
        "schedule-bot/internal/scheduler"
        "schedule-bot/internal/sheets"
        "schedule-bot/internal/storage"
)

func main() {
        // Загружаем конфигурацию
        cfg, err := config.Load()
        if err != nil {
                fmt.Fprintf(os.Stderr, "Ошибка загрузки конфигурации: %v\n", err)
                os.Exit(1)
        }

        // Настраиваем логгер
        logger := log.New(os.Stdout, "[SCHEDULE-BOT] ", log.LstdFlags|log.Lshortfile)

        logger.Println("Запуск бота расписания...")

        // Создаём хранилище
        // Извлекаем путь к БД из DATABASE_URL
        dbPath := cfg.DatabaseURL
        if len(dbPath) > 9 && dbPath[:9] == "sqlite://" {
                dbPath = dbPath[9:]
        }

        storage, err := storage.NewStorage(dbPath)
        if err != nil {
                logger.Printf("Ошибка создания хранилища: %v", err)
                os.Exit(1)
        }
        defer storage.Close()

        logger.Printf("Хранилище инициализировано: %s", dbPath)

        // Создаём клиент Google Sheets
        sheetsClient, err := sheets.NewClient(cfg.GoogleAPIKey, cfg.GoogleSheetID, cfg.CacheTTL, logger)
        if err != nil {
                logger.Printf("Ошибка создания клиента Google Sheets: %v", err)
                os.Exit(1)
        }

        logger.Println("Клиент Google Sheets инициализирован")

        // Создаём Telegram бота
        botAPI, err := tgbotapi.NewBotAPI(cfg.TelegramBotToken)
        if err != nil {
                logger.Printf("Ошибка создания Telegram бота: %v", err)
                os.Exit(1)
        }

        logger.Printf("Telegram бот авторизован: %s", botAPI.Self.UserName)

        // Создаём обработчик бота
        botHandler := bot.NewBot(botAPI, cfg, storage, sheetsClient, logger)

        // Создаём планировщик уведомлений
        sched := scheduler.NewScheduler(botAPI, storage, sheetsClient, logger)

        // Контекст для отмены
        ctx, cancel := context.WithCancel(context.Background())
        defer cancel()

        // Запускаем планировщик в горутине
        go sched.Start(ctx, cfg.NotifyDefaultTime)

        // Начинаем получать обновления
        u := tgbotapi.NewUpdate(0)
        u.Timeout = 60

        updates := botAPI.GetUpdatesChan(u)

        logger.Println("Бот запущен и ожидает сообщения...")

        // Обработка сигналов завершения
        sigChan := make(chan os.Signal, 1)
        signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

        go func() {
                botHandler.HandleUpdates(updates)
        }()

        // Ждём сигнал завершения
        sig := <-sigChan
        logger.Printf("Получен сигнал завершения: %v", sig)

        // Graceful shutdown
        cancel()
        sched.Stop()

        // Закрываем канал обновлений
        botAPI.StopReceivingUpdates()

        logger.Println("Бот завершил работу")

        // Небольшая задержка перед выходом
        time.Sleep(500 * time.Millisecond)
}