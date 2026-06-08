package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"taskmanager/internal/domain"
	"taskmanager/internal/service"
)

// TaskHandler turns HTTP requests into service calls and writes responses.
type TaskHandler struct {
	svc *service.TaskService
}

func NewTaskHandler(svc *service.TaskService) *TaskHandler {
	return &TaskHandler{svc: svc}
}

const dateLayout = "2006-01-02"

type taskRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Priority    string `json:"priority"`
	DueDate     string `json:"due_date"` // YYYY-MM-DD, optional
}

type taskResponse struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Priority    string `json:"priority"`
	DueDate     string `json:"due_date,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

func (h *TaskHandler) Create(w http.ResponseWriter, r *http.Request) {
	req, ok := decode(w, r)
	if !ok {
		return
	}
	due, err := parseDue(req.DueDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "due_date must be YYYY-MM-DD")
		return
	}
	task, err := h.svc.Create(r.Context(), service.CreateTaskInput{
		Title:       req.Title,
		Description: req.Description,
		Status:      req.Status,
		Priority:    req.Priority,
		DueDate:     due,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toResponse(task))
}

func (h *TaskHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	tasks, err := h.svc.List(r.Context(), domain.TaskFilter{
		Status:   q.Get("status"),
		Priority: q.Get("priority"),
		Search:   q.Get("q"),
		Sort:     q.Get("sort"),
		Order:    q.Get("order"),
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	resp := make([]taskResponse, len(tasks))
	for i := range tasks {
		resp[i] = toResponse(&tasks[i])
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *TaskHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	task, err := h.svc.Get(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toResponse(task))
}

func (h *TaskHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	req, ok := decode(w, r)
	if !ok {
		return
	}
	due, err := parseDue(req.DueDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "due_date must be YYYY-MM-DD")
		return
	}
	task, err := h.svc.Update(r.Context(), id, service.UpdateTaskInput{
		Title:       req.Title,
		Description: req.Description,
		Status:      req.Status,
		Priority:    req.Priority,
		DueDate:     due,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toResponse(task))
}

func (h *TaskHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- helpers ---

func decode(w http.ResponseWriter, r *http.Request) (taskRequest, bool) {
	var req taskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return req, false
	}
	return req, true
}

func parseID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid task id")
		return 0, false
	}
	return id, true
}

func parseDue(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func toResponse(t *domain.Task) taskResponse {
	resp := taskResponse{
		ID:          t.ID,
		Title:       t.Title,
		Description: t.Description,
		Status:      string(t.Status),
		Priority:    string(t.Priority),
		CreatedAt:   t.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   t.UpdatedAt.Format(time.RFC3339),
	}
	if t.DueDate != nil {
		resp.DueDate = t.DueDate.Format(dateLayout)
	}
	return resp
}

// writeServiceError maps domain errors to HTTP status codes.
func writeServiceError(w http.ResponseWriter, err error) {
	var ve domain.ValidationError
	switch {
	case errors.As(err, &ve):
		writeError(w, http.StatusBadRequest, ve.Error())
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, "task not found")
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}
