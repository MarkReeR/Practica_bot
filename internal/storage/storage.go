package storage

import (
        "database/sql"
        "encoding/json"
        "fmt"
        "time"

        _ "github.com/mattn/go-sqlite3"

        "schedule-bot/internal/models"
)

// Storage представляет хранилище данных
type Storage struct {
        db *sql.DB
}

// NewStorage создаёт новое хранилище
func NewStorage(dsn string) (*Storage, error) {
        db, err := sql.Open("sqlite3", dsn)
        if err != nil {
                return nil, fmt.Errorf("ошибка подключения к БД: %w", err)
        }

        // Включаем поддержку внешних ключей
        _, err = db.Exec("PRAGMA foreign_keys = ON")
        if err != nil {
                return nil, fmt.Errorf("ошибка включения FK: %w", err)
        }

        s := &Storage{db: db}

        // Создаём таблицы
        if err := s.createTables(); err != nil {
                return nil, fmt.Errorf("ошибка создания таблиц: %w", err)
        }

        return s, nil
}

// createTables создаёт необходимые таблицы
func (s *Storage) createTables() error {
        queries := []string{
                `CREATE TABLE IF NOT EXISTS users (
                        id INTEGER PRIMARY KEY AUTOINCREMENT,
                        telegram_id INTEGER UNIQUE NOT NULL,
                        username TEXT,
                        first_name TEXT NOT NULL,
                        last_name TEXT,
                        group_code TEXT,
                        is_admin BOOLEAN DEFAULT FALSE,
                        notify_enabled BOOLEAN DEFAULT FALSE,
                        notify_time TEXT DEFAULT '20:00',
                        changes_subscribed BOOLEAN DEFAULT FALSE,
                        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
                        updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
                )`,
                `CREATE TABLE IF NOT EXISTS schedule_cache (
                        id INTEGER PRIMARY KEY AUTOINCREMENT,
                        group_code TEXT NOT NULL,
                        week_type TEXT NOT NULL,
                        data TEXT NOT NULL,
                        expires_at TIMESTAMP NOT NULL,
                        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
                        UNIQUE(group_code, week_type)
                )`,
                `CREATE TABLE IF NOT EXISTS change_logs (
                        id INTEGER PRIMARY KEY AUTOINCREMENT,
                        group_code TEXT NOT NULL,
                        day TEXT NOT NULL,
                        time TEXT NOT NULL,
                        old_value TEXT,
                        new_value TEXT NOT NULL,
                        changed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
                )`,
                `CREATE TABLE IF NOT EXISTS notifications (
                        id INTEGER PRIMARY KEY AUTOINCREMENT,
                        user_id INTEGER NOT NULL,
                        message TEXT NOT NULL,
                        sent_at TIMESTAMP,
                        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
                        FOREIGN KEY(user_id) REFERENCES users(id)
                )`,
                `CREATE INDEX IF NOT EXISTS idx_users_telegram ON users(telegram_id)`,
                `CREATE INDEX IF NOT EXISTS idx_users_group ON users(group_code)`,
                `CREATE INDEX IF NOT EXISTS idx_cache_expires ON schedule_cache(expires_at)`,
                `CREATE INDEX IF NOT EXISTS idx_logs_changed ON change_logs(changed_at)`,
        }

        for _, q := range queries {
                _, err := s.db.Exec(q)
                if err != nil {
                        return fmt.Errorf("ошибка выполнения запроса: %w", err)
                }
        }

        return nil
}

// Close закрывает соединение с БД
func (s *Storage) Close() error {
        return s.db.Close()
}

// ==================== Users ====================

// CreateUser создаёт нового пользователя
func (s *Storage) CreateUser(user *models.User) error {
        query := `
                INSERT INTO users (telegram_id, username, first_name, last_name, group_code, is_admin, notify_enabled, notify_time, changes_subscribed)
                VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
                ON CONFLICT(telegram_id) DO UPDATE SET
                        username = excluded.username,
                        first_name = excluded.first_name,
                        last_name = excluded.last_name,
                        updated_at = CURRENT_TIMESTAMP
        `
        _, err := s.db.Exec(query,
                user.TelegramID,
                user.Username,
                user.FirstName,
                user.LastName,
                user.GroupCode,
                user.IsAdmin,
                user.NotifyEnabled,
                user.NotifyTime,
                user.ChangesSubscribed,
        )
        return err
}

// GetUserByTelegramID получает пользователя по Telegram ID
func (s *Storage) GetUserByTelegramID(telegramID int64) (*models.User, error) {
        query := `
                SELECT id, telegram_id, username, first_name, last_name, group_code, is_admin,
                       notify_enabled, notify_time, changes_subscribed, created_at, updated_at
                FROM users WHERE telegram_id = ?
        `
        row := s.db.QueryRow(query, telegramID)

        var user models.User
        err := row.Scan(
                &user.ID,
                &user.TelegramID,
                &user.Username,
                &user.FirstName,
                &user.LastName,
                &user.GroupCode,
                &user.IsAdmin,
                &user.NotifyEnabled,
                &user.NotifyTime,
                &user.ChangesSubscribed,
                &user.CreatedAt,
                &user.UpdatedAt,
        )

        if err == sql.ErrNoRows {
                return nil, nil
        }
        if err != nil {
                return nil, err
        }

        return &user, nil
}

// SetUserGroup устанавливает группу для пользователя
func (s *Storage) SetUserGroup(telegramID int64, groupCode string) error {
        query := `UPDATE users SET group_code = ?, updated_at = CURRENT_TIMESTAMP WHERE telegram_id = ?`
        _, err := s.db.Exec(query, groupCode, telegramID)
        return err
}

// SetNotifySettings устанавливает настройки уведомлений
func (s *Storage) SetNotifySettings(telegramID int64, enabled bool, notifyTime string) error {
        query := `UPDATE users SET notify_enabled = ?, notify_time = ?, updated_at = CURRENT_TIMESTAMP WHERE telegram_id = ?`
        _, err := s.db.Exec(query, enabled, notifyTime, telegramID)
        return err
}

// SetChangesSubscription устанавливает подписку на изменения
func (s *Storage) SetChangesSubscription(telegramID int64, subscribed bool) error {
        query := `UPDATE users SET changes_subscribed = ?, updated_at = CURRENT_TIMESTAMP WHERE telegram_id = ?`
        _, err := s.db.Exec(query, subscribed, telegramID)
        return err
}

// GetUsersWithNotify получает всех пользователей с включенными уведомлениями
func (s *Storage) GetUsersWithNotify() ([]models.User, error) {
        query := `
                SELECT id, telegram_id, username, first_name, last_name, group_code, is_admin,
                       notify_enabled, notify_time, changes_subscribed, created_at, updated_at
                FROM users WHERE notify_enabled = TRUE
        `
        rows, err := s.db.Query(query)
        if err != nil {
                return nil, err
        }
        defer rows.Close()

        var users []models.User
        for rows.Next() {
                var user models.User
                err := rows.Scan(
                        &user.ID,
                        &user.TelegramID,
                        &user.Username,
                        &user.FirstName,
                        &user.LastName,
                        &user.GroupCode,
                        &user.IsAdmin,
                        &user.NotifyEnabled,
                        &user.NotifyTime,
                        &user.ChangesSubscribed,
                        &user.CreatedAt,
                        &user.UpdatedAt,
                )
                if err != nil {
                        return nil, err
                }
                users = append(users, user)
        }

        return users, rows.Err()
}

// GetUsersSubscribedToChanges получает всех пользователей, подписанных на изменения
func (s *Storage) GetUsersSubscribedToChanges() ([]models.User, error) {
        query := `
                SELECT id, telegram_id, username, first_name, last_name, group_code, is_admin,
                       notify_enabled, notify_time, changes_subscribed, created_at, updated_at
                FROM users WHERE changes_subscribed = TRUE
        `
        rows, err := s.db.Query(query)
        if err != nil {
                return nil, err
        }
        defer rows.Close()

        var users []models.User
        for rows.Next() {
                var user models.User
                err := rows.Scan(
                        &user.ID,
                        &user.TelegramID,
                        &user.Username,
                        &user.FirstName,
                        &user.LastName,
                        &user.GroupCode,
                        &user.IsAdmin,
                        &user.NotifyEnabled,
                        &user.NotifyTime,
                        &user.ChangesSubscribed,
                        &user.CreatedAt,
                        &user.UpdatedAt,
                )
                if err != nil {
                        return nil, err
                }
                users = append(users, user)
        }

        return users, rows.Err()
}

// GetAllUsers получает всех пользователей
func (s *Storage) GetAllUsers() ([]models.User, error) {
        query := `
                SELECT id, telegram_id, username, first_name, last_name, group_code, is_admin,
                       notify_enabled, notify_time, changes_subscribed, created_at, updated_at
                FROM users
        `
        rows, err := s.db.Query(query)
        if err != nil {
                return nil, err
        }
        defer rows.Close()

        var users []models.User
        for rows.Next() {
                var user models.User
                err := rows.Scan(
                        &user.ID,
                        &user.TelegramID,
                        &user.Username,
                        &user.FirstName,
                        &user.LastName,
                        &user.GroupCode,
                        &user.IsAdmin,
                        &user.NotifyEnabled,
                        &user.NotifyTime,
                        &user.ChangesSubscribed,
                        &user.CreatedAt,
                        &user.UpdatedAt,
                )
                if err != nil {
                        return nil, err
                }
                users = append(users, user)
        }

        return users, rows.Err()
}

// ==================== Cache ====================

// SetScheduleCache сохраняет расписание в кэш
func (s *Storage) SetScheduleCache(groupCode, weekType, data string, expiresAt time.Time) error {
        query := `
                INSERT INTO schedule_cache (group_code, week_type, data, expires_at)
                VALUES (?, ?, ?, ?)
                ON CONFLICT(group_code, week_type) DO UPDATE SET
                        data = excluded.data,
                        expires_at = excluded.expires_at
        `
        _, err := s.db.Exec(query, groupCode, weekType, data, expiresAt)
        return err
}

// GetScheduleCache получает расписание из кэша
func (s *Storage) GetScheduleCache(groupCode, weekType string) ([]models.Lesson, error) {
        query := `SELECT data FROM schedule_cache WHERE group_code = ? AND week_type = ? AND expires_at > ?`

        var data string
        err := s.db.QueryRow(query, groupCode, weekType, time.Now()).Scan(&data)
        if err == sql.ErrNoRows {
                return nil, nil
        }
        if err != nil {
                return nil, err
        }

        var lessons []models.Lesson
        err = json.Unmarshal([]byte(data), &lessons)
        return lessons, err
}

// ClearExpiredCache очищает устаревший кэш
func (s *Storage) ClearExpiredCache() error {
        query := `DELETE FROM schedule_cache WHERE expires_at <= ?`
        _, err := s.db.Exec(query, time.Now())
        return err
}

// ==================== Change Logs ====================

// LogChange записывает изменение в расписании
func (s *Storage) LogChange(groupCode, day, timeSlot, oldValue, newValue string) error {
        query := `INSERT INTO change_logs (group_code, day, time, old_value, new_value) VALUES (?, ?, ?, ?, ?)`
        _, err := s.db.Exec(query, groupCode, day, timeSlot, oldValue, newValue)
        return err
}

// GetRecentChanges получает последние изменения
func (s *Storage) GetRecentChanges(limit int) ([]models.ChangeLog, error) {
        query := `
                SELECT id, group_code, day, time, old_value, new_value, changed_at
                FROM change_logs ORDER BY changed_at DESC LIMIT ?
        `
        rows, err := s.db.Query(query, limit)
        if err != nil {
                return nil, err
        }
        defer rows.Close()

        var changes []models.ChangeLog
        for rows.Next() {
                var change models.ChangeLog
                err := rows.Scan(
                        &change.ID,
                        &change.GroupCode,
                        &change.Day,
                        &change.Time,
                        &change.OldValue,
                        &change.NewValue,
                        &change.ChangedAt,
                )
                if err != nil {
                        return nil, err
                }
                changes = append(changes, change)
        }

        return changes, rows.Err()
}

// ==================== Notifications ====================

// CreateNotification создаёт уведомление
func (s *Storage) CreateNotification(userID int64, message string) error {
        query := `INSERT INTO notifications (user_id, message) VALUES (?, ?)`
        _, err := s.db.Exec(query, userID, message)
        return err
}

// MarkNotificationSent помечает уведомление как отправленное
func (s *Storage) MarkNotificationSent(id int64) error {
        query := `UPDATE notifications SET sent_at = CURRENT_TIMESTAMP WHERE id = ?`
        _, err := s.db.Exec(query, id)
        return err
}

// GetPendingNotifications получает неотправленные уведомления
func (s *Storage) GetPendingNotifications() ([]models.Notification, error) {
        query := `
                SELECT id, user_id, message, sent_at, created_at
                FROM notifications WHERE sent_at IS NULL
        `
        rows, err := s.db.Query(query)
        if err != nil {
                return nil, err
        }
        defer rows.Close()

        var notifications []models.Notification
        for rows.Next() {
                var notif models.Notification
                err := rows.Scan(
                        &notif.ID,
                        &notif.UserID,
                        &notif.Message,
                        &notif.SentAt,
                        &notif.CreatedAt,
                )
                if err != nil {
                        return nil, err
                }
                notifications = append(notifications, notif)
        }

        return notifications, rows.Err()
}