package handler

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"logdownloader/internal/model"
	"logdownloader/internal/store"
	"logdownloader/internal/worker"

	"github.com/google/uuid"
)

type Handler struct {
	store  *store.Store
	worker *worker.Worker
}

func New(s *store.Store, w *worker.Worker) *Handler {
	return &Handler{store: s, worker: w}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/settings", h.getSettings)
	mux.HandleFunc("PUT /api/settings", h.updateSettings)

	mux.HandleFunc("GET /api/queries", h.getQueries)
	mux.HandleFunc("POST /api/queries", h.createQuery)
	mux.HandleFunc("DELETE /api/queries/{id}", h.deleteQuery)

	mux.HandleFunc("POST /api/export", h.startExport)
	mux.HandleFunc("GET /api/jobs", h.getJobs)
	mux.HandleFunc("DELETE /api/jobs/{id}", h.deleteJob)
	mux.HandleFunc("GET /api/jobs/{id}/download", h.downloadJob)
}

// Settings

func (h *Handler) getSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, h.store.GetSettings())
}

func (h *Handler) updateSettings(w http.ResponseWriter, r *http.Request) {
	var s model.Settings
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.store.SaveSettings(s); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, s)
}

// Queries

func (h *Handler) getQueries(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, h.store.GetQueries())
}

func (h *Handler) createQuery(w http.ResponseWriter, r *http.Request) {
	var q model.SavedQuery
	if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	q.ID = uuid.NewString()
	if err := h.store.SaveQuery(q); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, q)
}

func (h *Handler) deleteQuery(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.store.DeleteQuery(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Export & Jobs

func (h *Handler) startExport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Query string `json:"query"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Query == "" {
		http.Error(w, "query is required", http.StatusBadRequest)
		return
	}

	jobID := h.worker.StartExport(req.Query)
	writeJSON(w, map[string]string{"job_id": jobID})
}

func (h *Handler) getJobs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, h.store.GetJobs())
}

func (h *Handler) deleteJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	h.store.DeleteJob(id)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) downloadJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	job := h.store.GetJob(id)
	if job == nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}
	if job.Status != model.JobDone {
		http.Error(w, "job not completed yet", http.StatusBadRequest)
		return
	}

	filePath := filepath.Join(h.store.ExportsDir(), job.FileName)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Disposition", "attachment; filename="+job.FileName)
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeFile(w, r, filePath)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
