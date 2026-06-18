package handler

import (
	"net/http"
	"strconv"
	"time"

	"midigator/internal/domain"
	"midigator/internal/service"
)

// NotificationHandler serves the current user's in-app notifications and their
// per-user delivery preferences.
type NotificationHandler struct {
	svc *service.NotificationService
}

func NewNotificationHandler(svc *service.NotificationService) *NotificationHandler {
	return &NotificationHandler{svc: svc}
}

type notificationResponse struct {
	ID             string  `json:"id"`
	Type           string  `json:"type"`
	Title          string  `json:"title"`
	Body           string  `json:"body"`
	NotifiableType string  `json:"notifiable_type"`
	NotifiableID   *int64  `json:"notifiable_id"`
	ReadAt         *string `json:"read_at"`
	CreatedAt      string  `json:"created_at"`
}

func toNotificationResponse(n domain.Notification) notificationResponse {
	return notificationResponse{
		ID:             n.ID,
		Type:           n.Type,
		Title:          n.Title,
		Body:           n.Body,
		NotifiableType: n.NotifiableType,
		NotifiableID:   n.NotifiableID,
		ReadAt:         rfc3339(n.ReadAt),
		CreatedAt:      n.CreatedAt.Format(time.RFC3339),
	}
}

func toNotificationResponses(items []domain.Notification) []notificationResponse {
	out := make([]notificationResponse, len(items))
	for i, n := range items {
		out[i] = toNotificationResponse(n)
	}
	return out
}

// Index lists the current user's notifications and the unread badge count.
func (h *NotificationHandler) Index(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	unreadOnly := q.Get("unread") == "true"
	limit := 0
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	list, err := h.svc.List(r.Context(), unreadOnly, limit)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data":         toNotificationResponses(list.Notifications),
		"unread_count": list.UnreadCount,
	})
}

// MarkRead marks one notification (by its UUID path value) as read.
func (h *NotificationHandler) MarkRead(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.svc.MarkRead(r.Context(), id); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "read": true})
}

// MarkAllRead marks every unread notification for the current user as read.
func (h *NotificationHandler) MarkAllRead(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.MarkAllRead(r.Context()); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"read": true})
}

type settingsResponse struct {
	Channel          string `json:"channel"`
	ChargebackNew    bool   `json:"chargeback_new"`
	ChargebackResult bool   `json:"chargeback_result"`
	PreventionNew    bool   `json:"prevention_new"`
	DailyDigest      bool   `json:"daily_digest"`
	WeeklyReport     bool   `json:"weekly_report"`
}

func toSettingsResponse(s *domain.NotificationSettings) settingsResponse {
	return settingsResponse{
		Channel:          s.Channel,
		ChargebackNew:    s.ChargebackNew,
		ChargebackResult: s.ChargebackResult,
		PreventionNew:    s.PreventionNew,
		DailyDigest:      s.DailyDigest,
		WeeklyReport:     s.WeeklyReport,
	}
}

// Settings returns the current user's notification preferences.
func (h *NotificationHandler) Settings(w http.ResponseWriter, r *http.Request) {
	s, err := h.svc.GetSettings(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toSettingsResponse(s))
}

// settingsRequest is the editable preference set (pointers => only set fields apply).
type settingsRequest struct {
	Channel          *string `json:"channel"`
	ChargebackNew    *bool   `json:"chargeback_new"`
	ChargebackResult *bool   `json:"chargeback_result"`
	PreventionNew    *bool   `json:"prevention_new"`
	DailyDigest      *bool   `json:"daily_digest"`
	WeeklyReport     *bool   `json:"weekly_report"`
}

// apply overlays the request onto the user's current settings, leaving omitted
// fields untouched.
func (req settingsRequest) apply(cur *domain.NotificationSettings) *domain.NotificationSettings {
	s := *cur
	if req.Channel != nil {
		s.Channel = *req.Channel
	}
	if req.ChargebackNew != nil {
		s.ChargebackNew = *req.ChargebackNew
	}
	if req.ChargebackResult != nil {
		s.ChargebackResult = *req.ChargebackResult
	}
	if req.PreventionNew != nil {
		s.PreventionNew = *req.PreventionNew
	}
	if req.DailyDigest != nil {
		s.DailyDigest = *req.DailyDigest
	}
	if req.WeeklyReport != nil {
		s.WeeklyReport = *req.WeeklyReport
	}
	return &s
}

// UpdateSettings edits the current user's notification preferences.
func (h *NotificationHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	cur, err := h.svc.GetSettings(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	var req settingsRequest
	if !decode(w, r, &req) {
		return
	}
	out, err := h.svc.UpdateSettings(r.Context(), req.apply(cur))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toSettingsResponse(out))
}
