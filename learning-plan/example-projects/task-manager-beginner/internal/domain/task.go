package domain

import (
	"context"
	"errors"
	"time"
)

// Status and Priority are small enumerations with their valid values.
type Status string
type Priority string

const (
	StatusTodo       Status = "todo"
	StatusInProgress Status = "in_progress"
	StatusDone       Status = "done"
)

const (
	PriorityLow    Priority = "low"
	PriorityMedium Priority = "medium"
	PriorityHigh   Priority = "high"
)

func (s Status) Valid() bool {
	switch s {
	case StatusTodo, StatusInProgress, StatusDone:
		return true
	}
	return false
}

func (p Priority) Valid() bool {
	switch p {
	case PriorityLow, PriorityMedium, PriorityHigh:
		return true
	}
	return false
}

// ErrNotFound is returned when a task does not exist.
var ErrNotFound = errors.New("task not found")

// ValidationError signals invalid input; the handler maps it to HTTP 400.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	if e.Field != "" {
		return e.Field + ": " + e.Message
	}
	return e.Message
}

// Task is the core entity. It carries no ORM tags — persistence concerns
// live in the repository's own model.
type Task struct {
	ID          int64
	Title       string
	Description string
	Status      Status
	Priority    Priority
	DueDate     *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// TaskFilter describes the optional filters/sorting for listing tasks.
type TaskFilter struct {
	Status   string
	Priority string
	Search   string
	Sort     string // created_at | due_date | title | priority
	Order    string // asc | desc
}

// TaskRepository is the storage contract for tasks.
type TaskRepository interface {
	Create(ctx context.Context, t *Task) error
	GetByID(ctx context.Context, id int64) (*Task, error)
	List(ctx context.Context, f TaskFilter) ([]Task, error)
	Update(ctx context.Context, t *Task) error
	Delete(ctx context.Context, id int64) error
}
