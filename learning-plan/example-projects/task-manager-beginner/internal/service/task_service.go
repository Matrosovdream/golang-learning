package service

import (
	"context"
	"strings"
	"time"

	"taskmanager/internal/domain"
)

// TaskService holds the business rules: validation, defaults, orchestration.
type TaskService struct {
	repo domain.TaskRepository
}

func NewTaskService(repo domain.TaskRepository) *TaskService {
	return &TaskService{repo: repo}
}

// CreateTaskInput carries the raw values for a new task from the edge.
type CreateTaskInput struct {
	Title       string
	Description string
	Status      string
	Priority    string
	DueDate     *time.Time
}

// UpdateTaskInput carries replacement values for an existing task.
type UpdateTaskInput struct {
	Title       string
	Description string
	Status      string
	Priority    string
	DueDate     *time.Time
}

func (s *TaskService) Create(ctx context.Context, in CreateTaskInput) (*domain.Task, error) {
	title, err := validateTitle(in.Title)
	if err != nil {
		return nil, err
	}
	// Status/priority default when omitted, otherwise must be valid.
	status := domain.Status(in.Status)
	if in.Status == "" {
		status = domain.StatusTodo
	} else if !status.Valid() {
		return nil, domain.ValidationError{Field: "status", Message: "must be one of todo, in_progress, done"}
	}
	priority := domain.Priority(in.Priority)
	if in.Priority == "" {
		priority = domain.PriorityMedium
	} else if !priority.Valid() {
		return nil, domain.ValidationError{Field: "priority", Message: "must be one of low, medium, high"}
	}

	task := &domain.Task{
		Title:       title,
		Description: strings.TrimSpace(in.Description),
		Status:      status,
		Priority:    priority,
		DueDate:     in.DueDate,
	}
	if err := s.repo.Create(ctx, task); err != nil {
		return nil, err
	}
	return task, nil
}

func (s *TaskService) Get(ctx context.Context, id int64) (*domain.Task, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *TaskService) List(ctx context.Context, f domain.TaskFilter) ([]domain.Task, error) {
	// Validate filter enums when provided so a typo returns 400, not silent empty.
	if f.Status != "" && !domain.Status(f.Status).Valid() {
		return nil, domain.ValidationError{Field: "status", Message: "invalid filter value"}
	}
	if f.Priority != "" && !domain.Priority(f.Priority).Valid() {
		return nil, domain.ValidationError{Field: "priority", Message: "invalid filter value"}
	}
	return s.repo.List(ctx, f)
}

func (s *TaskService) Update(ctx context.Context, id int64, in UpdateTaskInput) (*domain.Task, error) {
	task, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	title, err := validateTitle(in.Title)
	if err != nil {
		return nil, err
	}
	status := domain.Status(in.Status)
	if !status.Valid() {
		return nil, domain.ValidationError{Field: "status", Message: "must be one of todo, in_progress, done"}
	}
	priority := domain.Priority(in.Priority)
	if !priority.Valid() {
		return nil, domain.ValidationError{Field: "priority", Message: "must be one of low, medium, high"}
	}

	task.Title = title
	task.Description = strings.TrimSpace(in.Description)
	task.Status = status
	task.Priority = priority
	task.DueDate = in.DueDate

	if err := s.repo.Update(ctx, task); err != nil {
		return nil, err
	}
	return task, nil
}

func (s *TaskService) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

func validateTitle(raw string) (string, error) {
	title := strings.TrimSpace(raw)
	if title == "" {
		return "", domain.ValidationError{Field: "title", Message: "is required"}
	}
	if len(title) > 200 {
		return "", domain.ValidationError{Field: "title", Message: "must be at most 200 characters"}
	}
	return title, nil
}
