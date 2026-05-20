package models

import "time"

// Lesson представляет одно занятие
type Lesson struct {
        GroupCode   string // Код группы (8251160, 1251160...)
        Specialty   string // Специальность (23.05.01, 44.03.04...)
        Day         string // День недели (Понедельник, Вторник...)
        Time        string // Время начала (8:00, 9:40...)
        Discipline  string // Дисциплина
        LessonType  string // Тип (лек/прак/лаб)
        Building    string // Здание
        Classroom   string // Аудитория
        Department  string // Кафедра
        Teacher     string // Преподаватель
        Notes       string // Примечания (переносы, подгруппы)
        WeekType    string // "в" - верхняя, "н" - нижняя
}

// Group представляет учебную группу
type Group struct {
        Code      string // Код группы
        Specialty string // Специальность
}

// User представляет пользователя бота
type User struct {
        ID              int64     `json:"id"`
        TelegramID      int64     `json:"telegram_id"`
        Username        string    `json:"username,omitempty"`
        FirstName       string    `json:"first_name"`
        LastName        string    `json:"last_name,omitempty"`
        GroupCode       string    `json:"group_code,omitempty"`
        IsAdmin         bool      `json:"is_admin"`
        NotifyEnabled   bool      `json:"notify_enabled"`
        NotifyTime      string    `json:"notify_time"`
        ChangesSubscribed bool    `json:"changes_subscribed"`
        CreatedAt       time.Time `json:"created_at"`
        UpdatedAt       time.Time `json:"updated_at"`
}

// ScheduleCache представляет кэшированное расписание
type ScheduleCache struct {
        ID          int64     `json:"id"`
        GroupCode   string    `json:"group_code"`
        WeekType    string    `json:"week_type"` // "в" или "н"
        Data        string    `json:"data"`      // JSON с данными расписания
        ExpiresAt   time.Time `json:"expires_at"`
        CreatedAt   time.Time `json:"created_at"`
}

// ChangeLog представляет запись об изменении в расписании
type ChangeLog struct {
        ID          int64     `json:"id"`
        GroupCode   string    `json:"group_code"`
        Day         string    `json:"day"`
        Time        string    `json:"time"`
        OldValue    string    `json:"old_value,omitempty"`
        NewValue    string    `json:"new_value"`
        ChangedAt   time.Time `json:"changed_at"`
}

// Notification представляет уведомление для пользователя
type Notification struct {
        ID        int64     `json:"id"`
        UserID    int64     `json:"user_id"`
        Message   string    `json:"message"`
        SentAt    time.Time `json:"sent_at,omitempty"`
        CreatedAt time.Time `json:"created_at"`
}