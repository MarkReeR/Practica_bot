package config
import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
	"github.com/joho/godotenv"
)
// Config содержит всю конфигурацию приложения
type Config struct {
	TelegramBotToken         string
	GoogleAPIKey             string
	GoogleServiceAccountFile string
	GoogleSheetID            string
	DatabaseURL              string
	AdminUserIDs             []int64
	CacheTTL                 time.Duration
	NotifyDefaultTime        string
	LogLevel                 string
}
// Load загружает конфигурацию из переменных окружения
func Load() (*Config, error) {
	// Загружаем .env файл (если существует)
	_ = godotenv.Load()
	cfg := &Config{}
	cfg.TelegramBotToken = os.Getenv("TELEGRAM_BOT_TOKEN")
	if cfg.TelegramBotToken == "" {
		return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN не установлен")
	}
	// Поддерживаем оба способа аутентификации: API Key или Service Account
	cfg.GoogleAPIKey = os.Getenv("GOOGLE_API_KEY")
	cfg.GoogleServiceAccountFile = os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	// Если не указан ни один из способов аутентификации
	if cfg.GoogleAPIKey == "" && cfg.GoogleServiceAccountFile == "" {
		return nil, fmt.Errorf("необходимо установить GOOGLE_API_KEY или GOOGLE_SERVICE_ACCOUNT_FILE")
	}
	cfg.GoogleSheetID = os.Getenv("GOOGLE_SHEET_ID")
	if cfg.GoogleSheetID == "" {
		return nil, fmt.Errorf("GOOGLE_SHEET_ID не установлен")
	}
	cfg.DatabaseURL = os.Getenv("DATABASE_URL")
	if cfg.DatabaseURL == "" {
		cfg.DatabaseURL = "sqlite://bot.db"
	}
	// Парсим ADMIN_USER_IDS
	adminIDsStr := os.Getenv("ADMIN_USER_IDS")
	if adminIDsStr != "" {
		parts := strings.Split(adminIDsStr, ",")
		for _, p := range parts {
			id, err := strconv.ParseInt(strings.TrimSpace(p), 10, 64)
			if err != nil {
				return nil, fmt.Errorf("неверный ADMIN_USER_IDS: %v", err)
			}
			cfg.AdminUserIDs = append(cfg.AdminUserIDs, id)
		}
	}
	// Cache TTL
	ttlStr := os.Getenv("CACHE_TTL_MINUTES")
	if ttlStr == "" {
		ttlStr = "10"
	}
	ttlMinutes, err := strconv.Atoi(ttlStr)
	if err != nil {
		return nil, fmt.Errorf("неверный CACHE_TTL_MINUTES: %v", err)
	}
	cfg.CacheTTL = time.Duration(ttlMinutes) * time.Minute
	cfg.NotifyDefaultTime = os.Getenv("NOTIFY_DEFAULT_TIME")
	if cfg.NotifyDefaultTime == "" {
		cfg.NotifyDefaultTime = "20:00"
	}
	cfg.LogLevel = os.Getenv("LOG_LEVEL")
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}
	return cfg, nil
}
// IsAdmin проверяет, является ли пользователь администратором
func (c *Config) IsAdmin(userID int64) bool {
	for _, id := range c.AdminUserIDs {
		if id == userID {
			return true
		}
	}
	return false
}