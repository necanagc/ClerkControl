// Package task содержит модель задачи и сценарии работы с ней.
// Пакет ничего не знает про Telegram и про базу данных.
package task

// Task — задача внутри одного чата.
type Task struct {
	ID         int
	ChatID     int64 // арендатор: user_id в личке, chat_id в группе
	Title      string
	CreatorID  int64
	Creator    string
	AssigneeID *int64
	Assignee   string
	Done       bool
}

// AssignedTo сообщает, назначена ли задача на пользователя.
func (t Task) AssignedTo(userID int64) bool {
	return t.AssigneeID != nil && *t.AssigneeID == userID
}

// IsOpen сообщает, что задача ещё не выполнена.
func (t Task) IsOpen() bool {
	return !t.Done
}
