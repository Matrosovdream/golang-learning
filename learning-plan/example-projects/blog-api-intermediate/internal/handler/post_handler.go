package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"blogapi/internal/domain"
	"blogapi/internal/service"
)

// PostHandler turns HTTP requests into service calls and writes responses.
type PostHandler struct {
	svc *service.PostService
}

func NewPostHandler(svc *service.PostService) *PostHandler {
	return &PostHandler{svc: svc}
}

// --- request / response DTOs ---

type postRequest struct {
	Title     string   `json:"title"`
	Body      string   `json:"body"`
	Published bool     `json:"published"`
	Tags      []string `json:"tags"`
}

type commentRequest struct {
	Author string `json:"author"`
	Body   string `json:"body"`
}

type postSummary struct {
	ID           int64    `json:"id"`
	Title        string   `json:"title"`
	Slug         string   `json:"slug"`
	Published    bool     `json:"published"`
	Tags         []string `json:"tags"`
	CommentCount int64    `json:"comment_count"`
	CreatedAt    string   `json:"created_at"`
}

type postDetail struct {
	ID        int64             `json:"id"`
	Title     string            `json:"title"`
	Slug      string            `json:"slug"`
	Body      string            `json:"body"`
	Published bool              `json:"published"`
	Tags      []string          `json:"tags"`
	Comments  []commentResponse `json:"comments"`
	CreatedAt string            `json:"created_at"`
	UpdatedAt string            `json:"updated_at"`
}

type commentResponse struct {
	ID        int64  `json:"id"`
	PostID    int64  `json:"post_id"`
	Author    string `json:"author"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

type pageResponse struct {
	Items      []postSummary `json:"items"`
	Page       int           `json:"page"`
	PageSize   int           `json:"page_size"`
	Total      int64         `json:"total"`
	TotalPages int           `json:"total_pages"`
}

// --- handlers ---

func (h *PostHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req postRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	post, err := h.svc.CreatePost(r.Context(), service.PostInput(req))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toDetail(post))
}

func (h *PostHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var published *bool
	if v := q.Get("published"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "published must be true or false")
			return
		}
		published = &b
	}
	page, _ := strconv.Atoi(q.Get("page"))
	size, _ := strconv.Atoi(q.Get("page_size"))

	result, err := h.svc.ListPosts(r.Context(), domain.PostFilter{
		Search:    q.Get("q"),
		Tag:       q.Get("tag"),
		Published: published,
		Page:      page,
		PageSize:  size,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	items := make([]postSummary, len(result.Items))
	for i := range result.Items {
		items[i] = toSummary(&result.Items[i])
	}
	writeJSON(w, http.StatusOK, pageResponse{
		Items:      items,
		Page:       result.Page,
		PageSize:   result.PageSize,
		Total:      result.Total,
		TotalPages: result.TotalPages,
	})
}

func (h *PostHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	post, err := h.svc.GetPost(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toDetail(post))
}

func (h *PostHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	var req postRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	post, err := h.svc.UpdatePost(r.Context(), id, service.PostInput(req))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toDetail(post))
}

func (h *PostHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	if err := h.svc.DeletePost(r.Context(), id); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *PostHandler) AddComment(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	var req commentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	comment, err := h.svc.AddComment(r.Context(), id, service.CommentInput(req))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toComment(comment))
}

func (h *PostHandler) ListComments(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	comments, err := h.svc.ListComments(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	out := make([]commentResponse, len(comments))
	for i := range comments {
		out[i] = toComment(&comments[i])
	}
	writeJSON(w, http.StatusOK, out)
}

// --- helpers ---

func parseID(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid "+name)
		return 0, false
	}
	return id, true
}

func toSummary(p *domain.Post) postSummary {
	return postSummary{
		ID:           p.ID,
		Title:        p.Title,
		Slug:         p.Slug,
		Published:    p.Published,
		Tags:         nonNil(p.Tags),
		CommentCount: p.CommentCount,
		CreatedAt:    p.CreatedAt.Format(time.RFC3339),
	}
}

func toDetail(p *domain.Post) postDetail {
	comments := make([]commentResponse, len(p.Comments))
	for i := range p.Comments {
		comments[i] = toComment(&p.Comments[i])
	}
	return postDetail{
		ID:        p.ID,
		Title:     p.Title,
		Slug:      p.Slug,
		Body:      p.Body,
		Published: p.Published,
		Tags:      nonNil(p.Tags),
		Comments:  comments,
		CreatedAt: p.CreatedAt.Format(time.RFC3339),
		UpdatedAt: p.UpdatedAt.Format(time.RFC3339),
	}
}

func toComment(c *domain.Comment) commentResponse {
	return commentResponse{
		ID:        c.ID,
		PostID:    c.PostID,
		Author:    c.Author,
		Body:      c.Body,
		CreatedAt: c.CreatedAt.Format(time.RFC3339),
	}
}

// nonNil makes a nil slice serialise as [] instead of null.
func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// writeServiceError maps domain errors to HTTP status codes.
func writeServiceError(w http.ResponseWriter, err error) {
	var ve domain.ValidationError
	switch {
	case errors.As(err, &ve):
		writeError(w, http.StatusBadRequest, ve.Error())
	case errors.Is(err, domain.ErrPostNotFound):
		writeError(w, http.StatusNotFound, "post not found")
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}
