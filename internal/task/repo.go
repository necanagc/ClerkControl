package task

import "context"

// Filter описывает выборку задач. Нулевые поля означают «любое значение».
type Filter struct {
	ChatID     int64
	AssigneeID *int64
	CreatorID  *int64
	OnlyOpen   bool
}

// Repository — хранилище задач.
type Repository interface {
	Create(ctx context.Context, t Task) (Task, error)
	Update(ctx context.Context, t Task) error
	Get(ctx context.Context, chatID int64, id int) (Task, error)
	List(ctx context.Context, f Filter) ([]Task, error)
}
