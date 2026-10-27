package task

import "errors"

var (
	// ErrNotFound — задачи нет или она уже выполнена.
	ErrNotFound = errors.New("task: not found")
	// ErrForbidden — действие запрещено (задача не на этом пользователе).
	ErrForbidden = errors.New("task: forbidden")
	// ErrEmptyTitle — пустое название задачи.
	ErrEmptyTitle = errors.New("task: empty title")
)
