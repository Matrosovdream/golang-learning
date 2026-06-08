package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"taskmanager/internal/domain"
)

// taskModel is the GORM persistence model. It is deliberately separate from
// domain.Task so the ORM's tags never leak into the business core.
type taskModel struct {
	ID          int64  `gorm:"primaryKey"`
	Title       string `gorm:"size:200;not null"`
	Description string
	Status      string `gorm:"size:20;not null;index"`
	Priority    string `gorm:"size:20;not null;index"`
	DueDate     *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (taskModel) TableName() string { return "tasks" }

// AutoMigrate creates/updates the tasks table to match the model.
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(&taskModel{})
}

// TaskRepository implements domain.TaskRepository with GORM.
type TaskRepository struct {
	db *gorm.DB
}

func NewTaskRepository(db *gorm.DB) *TaskRepository {
	return &TaskRepository{db: db}
}

func (r *TaskRepository) Create(ctx context.Context, t *domain.Task) error {
	m := toModel(t)
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return fmt.Errorf("create task: %w", err)
	}
	*t = toDomain(m) // copy back generated id + timestamps
	return nil
}

func (r *TaskRepository) GetByID(ctx context.Context, id int64) (*domain.Task, error) {
	var m taskModel
	if err := r.db.WithContext(ctx).First(&m, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("get task: %w", err)
	}
	t := toDomain(m)
	return &t, nil
}

func (r *TaskRepository) List(ctx context.Context, f domain.TaskFilter) ([]domain.Task, error) {
	q := r.db.WithContext(ctx).Model(&taskModel{})
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.Priority != "" {
		q = q.Where("priority = ?", f.Priority)
	}
	if f.Search != "" {
		q = q.Where("title ILIKE ?", "%"+f.Search+"%")
	}
	q = q.Order(orderClause(f.Sort, f.Order))

	var models []taskModel
	if err := q.Find(&models).Error; err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	tasks := make([]domain.Task, len(models))
	for i, m := range models {
		tasks[i] = toDomain(m)
	}
	return tasks, nil
}

func (r *TaskRepository) Update(ctx context.Context, t *domain.Task) error {
	m := toModel(t)
	// A map update writes every column (including a nil due_date -> NULL) and
	// lets GORM bump updated_at automatically.
	res := r.db.WithContext(ctx).Model(&taskModel{}).Where("id = ?", t.ID).Updates(map[string]any{
		"title":       m.Title,
		"description": m.Description,
		"status":      m.Status,
		"priority":    m.Priority,
		"due_date":    m.DueDate,
	})
	if res.Error != nil {
		return fmt.Errorf("update task: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return r.reload(ctx, t)
}

func (r *TaskRepository) Delete(ctx context.Context, id int64) error {
	res := r.db.WithContext(ctx).Delete(&taskModel{}, id)
	if res.Error != nil {
		return fmt.Errorf("delete task: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// reload refreshes the domain task from the row (to pick up updated_at).
func (r *TaskRepository) reload(ctx context.Context, t *domain.Task) error {
	var m taskModel
	if err := r.db.WithContext(ctx).First(&m, t.ID).Error; err != nil {
		return fmt.Errorf("reload task: %w", err)
	}
	*t = toDomain(m)
	return nil
}

// orderClause maps user-supplied sort/order to a safe ORDER BY string,
// whitelisting columns so the input can never reach SQL directly.
func orderClause(sort, order string) string {
	col, ok := map[string]string{
		"created_at": "created_at",
		"due_date":   "due_date",
		"title":      "title",
		"priority":   "priority",
	}[sort]
	if !ok {
		col = "created_at"
	}
	dir := "desc"
	if strings.ToLower(order) == "asc" {
		dir = "asc"
	}
	return col + " " + dir
}

func toModel(t *domain.Task) taskModel {
	return taskModel{
		ID:          t.ID,
		Title:       t.Title,
		Description: t.Description,
		Status:      string(t.Status),
		Priority:    string(t.Priority),
		DueDate:     t.DueDate,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
	}
}

func toDomain(m taskModel) domain.Task {
	return domain.Task{
		ID:          m.ID,
		Title:       m.Title,
		Description: m.Description,
		Status:      domain.Status(m.Status),
		Priority:    domain.Priority(m.Priority),
		DueDate:     m.DueDate,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}
