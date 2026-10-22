package handler

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"logdownloader/internal/auth"
	"logdownloader/internal/model"
	"logdownloader/internal/worker"
)

// canAccessJob — job egasi yoki admin.
func canAccessJob(u *auth.AuthUser, job *model.ExportJob) bool {
	return u != nil && (u.Role == model.RoleAdmin || job.OwnerID == u.ID)
}

// loadJobForUser job'ni topadi va ruxsatni tekshiradi; muammo bo'lsa javobni
// o'zi yozadi va nil qaytaradi.
func (h *Handler) loadJobForUser(w http.ResponseWriter, r *http.Request, id string) *model.ExportJob {
	job := h.store.GetJob(id)
	if job == nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return nil
	}
	if !canAccessJob(auth.GetUser(r), job) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil
	}
	return job
}

// exportRequest — POST /api/export va schedule body'laridagi umumiy maydonlar.
type exportRequest struct {
	Query          string   `json:"query"`
	DatasourceID   string   `json:"datasource_id"`
	Start          string   `json:"start"`
	End            string   `json:"end"`
	SortOrder      string   `json:"sort_order"`
	Name           string   `json:"name"`
	Format         string   `json:"format"`
	Content        string   `json:"content"`
	Fields         []string `json:"fields"`
	ArchiveSplitMB int      `json:"archive_split_mb"`
	Notify         bool     `json:"notify"`
	AutoShare      bool     `json:"auto_share"`
}

func (h *Handler) startExport(w http.ResponseWriter, r *http.Request) {
	u := auth.GetUser(r)
	var req exportRequest
	if !decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Query) == "" || req.DatasourceID == "" {
		http.Error(w, "query and datasource_id are required", http.StatusBadRequest)
		return
	}
	if h.canUseDatasource(u, req.DatasourceID) == nil {
		http.Error(w, "datasource not found", http.StatusNotFound)
		return
	}
	jobID, err := h.worker.StartExport(worker.ExportParams{
		Query:          req.Query,
		DatasourceID:   req.DatasourceID,
		Start:          req.Start,
		End:            req.End,
		SortOrder:      req.SortOrder,
		Name:           req.Name,
		Format:         req.Format,
		Content:        req.Content,
		Fields:         req.Fields,
		ArchiveSplitMB: req.ArchiveSplitMB,
		Notify:         req.Notify,
		AutoShare:      req.AutoShare,
		OwnerID:        u.ID,
		OwnerName:      u.Username,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	h.audit(r, "export.start", jobID, fmt.Sprintf("ds=%s format=%s content=%s", req.DatasourceID, req.Format, req.Content))
	writeJSON(w, map[string]string{"job_id": jobID})
}

// getJobs — admin barcha joblarni, oddiy user faqat o'zinikini ko'radi.
func (h *Handler) getJobs(w http.ResponseWriter, r *http.Request) {
	u := auth.GetUser(r)
	writeJSON(w, h.store.GetJobsForUser(u.ID, u.Role == model.RoleAdmin))
}

// deleteJob — egasi yoki admin. Queued bo'lsa navbatdan, running bo'lsa to'xtatiladi.
func (h *Handler) deleteJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	job := h.loadJobForUser(w, r, id)
	if job == nil {
		return
	}
	h.worker.Cancel(id)
	h.store.DeleteJob(id)
	h.audit(r, "job.delete", id, string(job.Status))
	w.WriteHeader(http.StatusNoContent)
}

// contentTypeForFile fayl kengaytmasiga qarab Content-Type qaytaradi.
func contentTypeForFile(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".gz"), strings.HasSuffix(lower, ".tgz"):
		return "application/gzip"
	case strings.HasSuffix(lower, ".csv"):
		return "text/csv; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}

// jobFile — `file` parametri bo'yicha job fayllaridan birini tanlaydi (default birinchi).
func jobFile(job *model.ExportJob, name string) (string, bool) {
	if name == "" {
		return job.FileName, true
	}
	if name == job.FileName {
		return name, true
	}
	for _, f := range job.Files {
		if f.Name == name {
			return f.Name, true
		}
	}
	return "", false
}

func (h *Handler) downloadJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	job := h.loadJobForUser(w, r, id)
	if job == nil {
		return
	}
	if job.Status != model.JobDone {
		http.Error(w, "job not completed yet", http.StatusBadRequest)
		return
	}
	name, ok := jobFile(job, r.URL.Query().Get("file"))
	if !ok {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}
	filePath := filepath.Join(h.store.ExportsDir(), name)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}
	h.audit(r, "job.download", id, name)
	w.Header().Set("Content-Disposition", "attachment; filename="+name)
	w.Header().Set("Content-Type", contentTypeForFile(name))
	http.ServeFile(w, r, filePath)
}

// downloadToken — brauzer <a download> uchun qisqa muddatli (5 min), shu
// job'ga bog'langan token. Frontend `?dt=<token>&file=<nom>` bilan yuklaydi.
func (h *Handler) downloadToken(w http.ResponseWriter, r *http.Request) {
	u := auth.GetUser(r)
	id := r.PathValue("id")
	job := h.loadJobForUser(w, r, id)
	if job == nil {
		return
	}
	if job.Status != model.JobDone {
		http.Error(w, "job not completed yet", http.StatusBadRequest)
		return
	}
	tok, err := auth.GenerateDownloadToken(job.ID, u.ID)
	if err != nil {
		http.Error(w, "token generation failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"token": tok, "expires_in": int(auth.DownloadTokenTTL.Seconds())})
}

// stats — Jobs sahifasi sarlavhasi uchun: disk, kvota, navbat.
func (h *Handler) stats(w http.ResponseWriter, r *http.Request) {
	bytes, files := h.store.ExportsUsage()
	st := h.store.GetSettings()
	running, queued := h.worker.Stats()
	writeJSON(w, map[string]any{
		"exports_bytes": bytes,
		"exports_files": files,
		"jobs":          h.store.JobCounts(),
		"retention":     st.Retention,
		"worker": map[string]any{
			"max_concurrent": st.Worker.MaxConcurrent,
			"running":        running,
			"queued":         queued,
		},
	})
}
