package sheets

import (
	"context"
	"encoding/json"
	"fmt"
	// "os"
	"strings"
	"time"

	"golang.org/x/exp/slog"
	// "golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"

	"schedule-bot/internal/models"
)

// Client представляет клиент для работы с Google Sheets
type Client struct {
srv      *sheets.Service
sheetID  string
cache    map[string]*cacheEntry
cacheTTL time.Duration
logger   *slog.Logger
}

type cacheEntry struct {
data      []models.Lesson
expiresAt time.Time
}

// NewClient создаёт новый клиент Google Sheets
// Поддерживает два способа аутентификации:
// 1. Через API ключ (apiKey не пустой)
// 2. Через Service Account файл (serviceAccountFile не пустой)
func NewClient(apiKey, credentialsFile, sheetID string, cacheTTL time.Duration, logger *slog.Logger) (*Client, error) {
        ctx := context.Background()

        var srv *sheets.Service
        var err error

        // Приоритет: файл учетных данных (Service Account), иначе API ключ
        if credentialsFile != "" {
                srv, err = sheets.NewService(ctx, option.WithCredentialsFile(credentialsFile))
                if err != nil {
                        return nil, fmt.Errorf("ошибка создания клиента Sheets с credentials: %w", err)
                }
                logger.Info("Инициализация Google Sheets через Service Account")
        } else {
                srv, err = sheets.NewService(ctx, option.WithAPIKey(apiKey))
                if err != nil {
                        return nil, fmt.Errorf("ошибка создания клиента Sheets: %w", err)
                }
                logger.Info("Инициализация Google Sheets через API Key")
        }
	return &Client{
	srv:      srv,
	sheetID:  sheetID,
	cache:    make(map[string]*cacheEntry),
	cacheTTL: cacheTTL,
	logger:   logger,
	}, nil
}

// GetGroups получает список всех групп из таблицы
func (c *Client) GetGroups(ctx context.Context) ([]models.Group, error) {
cacheKey := "groups_list"

// Проверяем кэш
if entry, ok := c.cache[cacheKey]; ok && time.Now().Before(entry.expiresAt) {
c.logger.Debug("Группы получены из кэша")
// Преобразуем данные из кэша в группы
groups := make([]models.Group, 0)
seen := make(map[string]bool)
for _, lesson := range entry.data {
if !seen[lesson.GroupCode] {
seen[lesson.GroupCode] = true
groups = append(groups, models.Group{
Code:      lesson.GroupCode,
Specialty: lesson.Specialty,
})
}
}
return groups, nil
}

// Читаем данные из таблицы
readRange := "Лист1!A1:Z100" // Предполагаемый диапазон

resp, err := c.srv.Spreadsheets.Values.Get(c.sheetID, readRange).Do()
if err != nil {
return nil, c.handleError(err)
}

if len(resp.Values) == 0 {
return nil, fmt.Errorf("таблица пуста")
}

// Парсим заголовки для получения кодов групп и специальностей
groups := c.parseGroups(resp.Values)

// Кэшируем результат
c.cache[cacheKey] = &cacheEntry{
data:      c.lessonsToCacheData(groups),
expiresAt: time.Now().Add(c.cacheTTL),
}

return groups, nil
}

// parseGroups извлекает информацию о группах из заголовков таблицы
func (c *Client) parseGroups(rows [][]interface{}) []models.Group {
var groups []models.Group

if len(rows) < 2 {
return groups
}

// Первая строка - коды групп, вторая - специальности
headerRow := rows[0]
specialtyRow := rows[1]

for i, cell := range headerRow {
code := strings.TrimSpace(fmt.Sprintf("%v", cell))
if code == "" || strings.Contains(strings.ToLower(code), "день") {
continue
}

specialty := ""
if i < len(specialtyRow) {
specialty = strings.TrimSpace(fmt.Sprintf("%v", specialtyRow[i]))
}

groups = append(groups, models.Group{
Code:      code,
Specialty: specialty,
})
}

return groups
}

// lessonsToCacheData преобразует группы в данные для кэша
func (c *Client) lessonsToCacheData(groups []models.Group) []models.Lesson {
lessons := make([]models.Lesson, len(groups))
for i, g := range groups {
lessons[i] = models.Lesson{
GroupCode: g.Code,
Specialty: g.Specialty,
}
}
return lessons
}

// GetSchedule получает расписание для конкретной группы
func (c *Client) GetSchedule(ctx context.Context, groupCode string, weekType string) ([]models.Lesson, error) {
cacheKey := fmt.Sprintf("schedule_%s_%s", groupCode, weekType)

// Проверяем кэш
if entry, ok := c.cache[cacheKey]; ok && time.Now().Before(entry.expiresAt) {
c.logger.Debug("Расписание получено из кэша", "group", groupCode, "week", weekType)
return entry.data, nil
}

// Читаем все данные из таблицы
readRange := "Лист1!A1:ZZ500"

resp, err := c.srv.Spreadsheets.Values.Get(c.sheetID, readRange).Do()
if err != nil {
return nil, c.handleError(err)
}

if len(resp.Values) == 0 {
return nil, fmt.Errorf("таблица пуста")
}

// Парсим расписание
lessons := c.parseSchedule(resp.Values, groupCode, weekType)

// Кэшируем результат
c.cache[cacheKey] = &cacheEntry{
data:      lessons,
expiresAt: time.Now().Add(c.cacheTTL),
}

return lessons, nil
}

// parseSchedule парсит данные таблицы в структуру Lesson
func (c *Client) parseSchedule(rows [][]interface{}, targetGroup string, weekType string) []models.Lesson {
var lessons []models.Lesson

if len(rows) < 4 {
return lessons
}

// Находим индекс нужной группы в заголовках
headerRow := rows[0]
groupIndex := -1
groupSpecialty := ""

for i, cell := range headerRow {
code := strings.TrimSpace(fmt.Sprintf("%v", cell))
if code == targetGroup {
groupIndex = i
break
}
}

if groupIndex == -1 {
return lessons
}

// Получаем специальность из второй строки
if len(rows) > 1 && groupIndex < len(rows[1]) {
groupSpecialty = strings.TrimSpace(fmt.Sprintf("%v", rows[1][groupIndex]))
}

// Парсим строки с расписанием
// Ожидаемая структура: День | Неделя | Время | Данные занятия...
for _, row := range rows[3:] { // Пропускаем заголовки
if len(row) <= groupIndex {
continue
}

day := ""
rowWeekType := ""
timeSlot := ""

// Первые колонки содержат метаданные
if len(row) >= 1 {
day = strings.TrimSpace(fmt.Sprintf("%v", row[0]))
}
if len(row) >= 2 {
rowWeekType = strings.TrimSpace(fmt.Sprintf("%v", row[1]))
}
if len(row) >= 3 {
timeSlot = strings.TrimSpace(fmt.Sprintf("%v", row[2]))
}

// Пропускаем строки без дня или времени
if day == "" || timeSlot == "" {
continue
}

// Фильтруем по типу недели
if weekType != "" && rowWeekType != "" && rowWeekType != weekType {
continue
}

// Получаем данные занятия из колонки группы
cellValue := strings.TrimSpace(fmt.Sprintf("%v", row[groupIndex]))
if cellValue == "" {
continue
}

// Парсим содержимое ячейки
lesson := c.parseLessonCell(cellValue, targetGroup, groupSpecialty, day, timeSlot, rowWeekType)
if lesson != nil {
lessons = append(lessons, *lesson)
}
}

return lessons
}

// parseLessonCell парсит содержимое одной ячейки в структуру Lesson
func (c *Client) parseLessonCell(cellValue, groupCode, specialty, day, timeSlot, weekType string) *models.Lesson {
// Формат ячейки может быть разным, например:
// "Дисциплина | лек | Здание | Аудитория | Кафедра | Преподаватель | Примечания"

parts := strings.Split(cellValue, "|")

lesson := &models.Lesson{
GroupCode:  groupCode,
Specialty:  specialty,
Day:        day,
Time:       timeSlot,
WeekType:   weekType,
}

// Очищаем и назначаем поля
clean := func(s string) string {
return strings.TrimSpace(s)
}

if len(parts) >= 1 {
lesson.Discipline = clean(parts[0])
}
if len(parts) >= 2 {
lesson.LessonType = clean(parts[1])
}
if len(parts) >= 3 {
lesson.Building = clean(parts[2])
}
if len(parts) >= 4 {
lesson.Classroom = clean(parts[3])
}
if len(parts) >= 5 {
lesson.Department = clean(parts[4])
}
if len(parts) >= 6 {
lesson.Teacher = clean(parts[5])
}
if len(parts) >= 7 {
lesson.Notes = clean(parts[6])
}

return lesson
}

// ClearCache очищает кэш
func (c *Client) ClearCache() {
c.cache = make(map[string]*cacheEntry)
c.logger.Info("Кэш очищен")
}

// handleError обрабатывает ошибки API
func (c *Client) handleError(err error) error {
if err == nil {
return nil
}

errStr := err.Error()

// Обработка rate limit (429)
if strings.Contains(errStr, "429") {
c.logger.Warn("Превышен лимит запросов к Google Sheets API")
return fmt.Errorf("превышен лимит запросов, попробуйте позже")
}

// Обработка доступа (403)
if strings.Contains(errStr, "403") {
c.logger.Error("Отказано в доступе к Google Sheets")
return fmt.Errorf("нет доступа к таблице, проверьте права доступа Service Account или API ключ")
}

// Таймауты
if strings.Contains(errStr, "timeout") {
c.logger.Warn("Таймаут при запросе к Google Sheets")
return fmt.Errorf("таймаут соединения, попробуйте позже")
}

return err
}

// MarshalLessons сериализует lessons в JSON
func MarshalLessons(lessons []models.Lesson) (string, error) {
data, err := json.Marshal(lessons)
if err != nil {
return "", err
}
return string(data), nil
}

// UnmarshalLessons десериализует JSON в lessons
func UnmarshalLessons(data string) ([]models.Lesson, error) {
var lessons []models.Lesson
err := json.Unmarshal([]byte(data), &lessons)
return lessons, err
}