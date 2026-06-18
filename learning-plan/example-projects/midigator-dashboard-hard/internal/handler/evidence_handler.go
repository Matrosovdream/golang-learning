package handler

import (
	"net/http"
	"time"

	"midigator/internal/domain"
	"midigator/internal/service"
)

// EvidenceHandler serves the polymorphic evidence-file endpoints nested under a
// case/order subject: list ({type}/{id}), upload metadata, and delete
// ({evidenceId}). The subject type/id come from the path.
type EvidenceHandler struct {
	svc *service.EvidenceService
}

func NewEvidenceHandler(svc *service.EvidenceService) *EvidenceHandler {
	return &EvidenceHandler{svc: svc}
}

type evidenceResponse struct {
	ID               int64  `json:"id"`
	EvidenceableType string `json:"evidenceable_type"`
	EvidenceableID   int64  `json:"evidenceable_id"`
	UploadedBy       int64  `json:"uploaded_by"`
	UploaderName     string `json:"uploader_name"`
	Name             string `json:"name"`
	Path             string `json:"path"`
	MimeType         string `json:"mime_type"`
	SizeBytes        int64  `json:"size_bytes"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

func toEvidenceResponse(e domain.EvidenceFile) evidenceResponse {
	return evidenceResponse{
		ID:               e.ID,
		EvidenceableType: e.EvidenceableType,
		EvidenceableID:   e.EvidenceableID,
		UploadedBy:       e.UploadedBy,
		UploaderName:     e.UploaderName,
		Name:             e.Name,
		Path:             e.Path,
		MimeType:         e.MimeType,
		SizeBytes:        e.SizeBytes,
		CreatedAt:        e.CreatedAt.Format(time.RFC3339),
		UpdatedAt:        e.UpdatedAt.Format(time.RFC3339),
	}
}

func toEvidenceResponses(items []domain.EvidenceFile) []evidenceResponse {
	out := make([]evidenceResponse, len(items))
	for i, e := range items {
		out[i] = toEvidenceResponse(e)
	}
	return out
}

// Index lists a subject's evidence files.
func (h *EvidenceHandler) Index(w http.ResponseWriter, r *http.Request) {
	subjectID, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	items, err := h.svc.List(r.Context(), r.PathValue("type"), subjectID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": toEvidenceResponses(items)})
}

// evidenceCreateRequest is the metadata-only upload body.
type evidenceCreateRequest struct {
	Name      string `json:"name"`
	MimeType  string `json:"mime_type"`
	SizeBytes int64  `json:"size_bytes"`
}

// Store records evidence metadata for a subject.
func (h *EvidenceHandler) Store(w http.ResponseWriter, r *http.Request) {
	subjectID, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	var req evidenceCreateRequest
	if !decode(w, r, &req) {
		return
	}
	e, err := h.svc.Create(r.Context(), r.PathValue("type"), subjectID, req.Name, req.MimeType, req.SizeBytes)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toEvidenceResponse(*e))
}

// Destroy removes one evidence file.
func (h *EvidenceHandler) Destroy(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "evidenceId")
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": id})
}
