package handler

import (
	"bytes"
	"net/http"

	"midigator/internal/service"
)

// ExportHandler serves CSV downloads for orders and the four case types.
type ExportHandler struct {
	svc *service.ExportService
}

func NewExportHandler(svc *service.ExportService) *ExportHandler {
	return &ExportHandler{svc: svc}
}

// CSV streams a tenant-scoped CSV export for the {type} path value. The service
// validates the kind before producing any rows, so we render into a buffer
// first: a validation/lookup error maps cleanly via writeServiceError, while a
// successful export is sent as a text/csv attachment.
func (h *ExportHandler) CSV(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("type")
	var buf bytes.Buffer
	if err := h.svc.WriteCSV(r.Context(), &buf, kind); err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename="+kind+".csv")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}
