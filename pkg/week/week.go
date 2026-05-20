package week

import (
        "time"
)

// WeekType представляет тип недели
type WeekType string

const (
        UpperWeek WeekType = "в" // Верхняя неделя
        LowerWeek WeekType = "н" // Нижняя неделя
)

// GetWeekType определяет тип текущей недели
// Используется дата 1 сентября 2024 как точка отсчёта для верхней недели
func GetWeekType(t time.Time) WeekType {
        // Базовая дата - 1 сентября 2024 (воскресенье), считаем её началом верхней недели
        baseDate := time.Date(2024, time.September, 2, 0, 0, 0, 0, t.Location())

        // Разница в днях между текущей датой и базовой
        daysDiff := int(t.Sub(baseDate).Hours() / 24)

        // Если разница отрицательная, используем другую логику
        if daysDiff < 0 {
                // Для дат до 2 сентября 2024 считаем от 1 января 2024
                baseDate = time.Date(2024, time.January, 1, 0, 0, 0, 0, t.Location())
                daysDiff = int(t.Sub(baseDate).Hours() / 24)
        }

        // Номер недели (начиная с 0)
        weekNum := daysDiff / 7

        // Чётные недели - верхние, нечётные - нижние
        if weekNum%2 == 0 {
                return UpperWeek
        }
        return LowerWeek
}

// GetWeekTypeString возвращает строковое представление типа недели
func GetWeekTypeString(t time.Time) string {
        return string(GetWeekType(t))
}

// IsUpperWeek проверяет, является ли текущая неделя верхней
func IsUpperWeek(t time.Time) bool {
        return GetWeekType(t) == UpperWeek
}

// GetNextWeekType возвращает тип следующей недели
func GetNextWeekType(t time.Time) WeekType {
        current := GetWeekType(t)
        if current == UpperWeek {
                return LowerWeek
        }
        return UpperWeek
}

// GetWeekNumber возвращает номер недели в учебном году
func GetWeekNumber(t time.Time) int {
        // Начало учебного года - 1 сентября
        year := t.Year()
        startOfYear := time.Date(year, time.September, 1, 0, 0, 0, 0, t.Location())

        // Если дата до 1 сентября, считаем от предыдущего года
        if t.Before(startOfYear) {
                startOfYear = time.Date(year-1, time.September, 1, 0, 0, 0, 0, t.Location())
        }

        daysDiff := int(t.Sub(startOfYear).Hours() / 24)
        return daysDiff/7 + 1
}