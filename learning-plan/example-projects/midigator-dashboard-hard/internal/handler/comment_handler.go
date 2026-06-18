package handler

import (
	"net/http"
	"time"

	"midigator/internal/domain"
	"midigator/internal/service"
)

// CommentHandler serves the polymorphic comment endpoints (index / store /
// destroy) for any commentable subject. The subject type and id come from the
// route path values ({type}, {id}); destroy also reads {commentId}.
type CommentHandler struct {
	svc *service.CommentService
}

func NewCommentHandler(svc *service.CommentService) *CommentHandler {
	return &CommentHandler{svc: svc}
}

type commentResponse struct {
	ID         int64  `json:"id"`
	Type       string `json:"type"`
	SubjectID  int64  `json:"subject_id"`
	UserID     int64  `json:"user_id"`
	UserName   string `json:"user_name"`
	Body       string `json:"body"`
	IsInternal bool   `json:"is_internal"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

func toCommentResponse(c domain.Comment) commentResponse {
	return commentResponse{
		ID:         c.ID,
		Type:       c.CommentableType,
		SubjectID:  c.CommentableID,
		UserID:     c.UserID,
		UserName:   c.UserName,
		Body:       c.Body,
		IsInternal: c.IsInternal,
		CreatedAt:  c.CreatedAt.Format(time.RFC3339),
		UpdatedAt:  c.UpdatedAt.Format(time.RFC3339),
	}
}

func toCommentResponses(items []domain.Comment) []commentResponse {
	out := make([]commentResponse, len(items))
	for i, c := range items {
		out[i] = toCommentResponse(c)
	}
	return out
}

// Index lists the comments attached to a subject.
func (h *CommentHandler) Index(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	items, err := h.svc.List(r.Context(), r.PathValue("type"), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": toCommentResponses(items)})
}

// commentCreateRequest is the body for adding a comment.
type commentCreateRequest struct {
	Body       string `json:"body"`
	IsInternal bool   `json:"is_internal"`
}

// Store adds a comment to a subject.
func (h *CommentHandler) Store(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	var req commentCreateRequest
	if !decode(w, r, &req) {
		return
	}
	c, err := h.svc.Create(r.Context(), r.PathValue("type"), id, req.Body, req.IsInternal)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toCommentResponse(*c))
}

// Destroy removes a comment.
func (h *CommentHandler) Destroy(w http.ResponseWriter, r *http.Request) {
	commentID, ok := parseID(w, r, "commentId")
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), commentID); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": commentID, "deleted": true})
}
