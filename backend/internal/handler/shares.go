package handler

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"logdownloader/internal/auth"
	"logdownloader/internal/model"
	"logdownloader/internal/store"
)

func (h *Handler) createShare(w http.ResponseWriter, r *http.Request) {
	u := auth.GetUser(r)
	jobID := r.PathValue("id")

	job := h.loadJobForUser(w, r, jobID)
	if job == nil {
		return
	}
	if job.Status != model.JobDone {
		http.Error(w, "job is not completed yet", http.StatusBadRequest)
		return
	}

	var req struct {
		MaxDownloads int    `json:"max_downloads"`
		ExpiresAt    string `json:"expires_at"` // RFC3339 UTC, ixtiyoriy
	}
	if !decode(w, r, &req) {
		return
	}
	if req.MaxDownloads < 0 {
		req.MaxDownloads = 0
	}
	var expiresAt *time.Time
	if s := strings.TrimSpace(req.ExpiresAt); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			http.Error(w, "invalid expires_at: "+err.Error(), http.StatusBadRequest)
			return
		}
		if !t.After(time.Now()) {
			http.Error(w, "expires_at must be in the future", http.StatusBadRequest)
			return
		}
		expiresAt = &t
	}

	token, err := store.NewShareToken()
	if err != nil {
		http.Error(w, "failed to generate token", http.StatusInternalServerError)
		return
	}
	share := &model.ShareLink{
		Token:        token,
		JobID:        jobID,
		CreatedBy:    u.ID,
		CreatedAt:    time.Now(),
		ExpiresAt:    expiresAt,
		MaxDownloads: req.MaxDownloads,
	}
	if err := h.store.SaveShare(share); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.audit(r, "share.create", jobID, fmt.Sprintf("max=%d expires=%s", req.MaxDownloads, req.ExpiresAt))
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, h.shareResponse(r, share, job))
}

func (h *Handler) getShare(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("id")
	job := h.loadJobForUser(w, r, jobID)
	if job == nil {
		return
	}
	share := h.store.GetShareByJob(jobID)
	if share == nil {
		http.Error(w, "no share for this job", http.StatusNotFound)
		return
	}
	writeJSON(w, h.shareResponse(r, share, job))
}

func (h *Handler) revokeShare(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("id")
	if h.loadJobForUser(w, r, jobID) == nil {
		return
	}
	share := h.store.GetShareByJob(jobID)
	if share == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := h.store.DeleteShare(share.Token); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.audit(r, "share.revoke", jobID, "")
	w.WriteHeader(http.StatusNoContent)
}

// publicBase — PublicURL sozlangan bo'lsa u, aks holda so'rovdagi scheme+host.
func (h *Handler) publicBase(r *http.Request) string {
	if base := strings.TrimRight(h.store.PublicURL(), "/"); base != "" {
		return base
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	host := r.Host
	if fh := r.Header.Get("X-Forwarded-Host"); fh != "" {
		host = fh
	}
	return scheme + "://" + host
}

// shareResponse — share modelni response uchun tayyorlaydi; ko'p faylli
// export uchun har fayl URL'i (`/dl/<token>/<n>`).
func (h *Handler) shareResponse(r *http.Request, sh *model.ShareLink, job *model.ExportJob) map[string]any {
	base := h.publicBase(r)
	mainURL := fmt.Sprintf("%s/dl/%s", base, sh.Token)
	files := make([]map[string]any, 0, len(job.Files))
	if len(job.Files) == 0 {
		files = append(files, map[string]any{"name": job.FileName, "url": mainURL, "size": job.Size})
	}
	for i, f := range job.Files {
		u := mainURL
		if i > 0 {
			u = fmt.Sprintf("%s/%d", mainURL, i+1)
		}
		files = append(files, map[string]any{"name": f.Name, "url": u, "size": f.Size})
	}
	return map[string]any{
		"token":          sh.Token,
		"job_id":         sh.JobID,
		"created_by":     sh.CreatedBy,
		"created_at":     sh.CreatedAt,
		"expires_at":     sh.ExpiresAt,
		"max_downloads":  sh.MaxDownloads,
		"download_count": sh.DownloadCount,
		"url":            mainURL,
		"files":          files,
	}
}

// downloadShare — public yuklab olish. Har bir HTTP so'rov = bitta "yuklab
// olish", shuning uchun Range qo'llab-quvvatlanmaydi (`Accept-Ranges: none`).
// `/dl/{token}/{n}` — n-fayl (1-asosli), ko'p arxivli export uchun.
func (h *Handler) downloadShare(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	idx := 0
	if n := r.PathValue("n"); n != "" {
		v, err := strconv.Atoi(n)
		if err != nil || v < 1 {
			http.Error(w, "bad file index", http.StatusBadRequest)
			return
		}
		idx = v - 1
	}

	share, filePath, fileName, exhaustedAfter, err := h.store.ConsumeShare(token, idx)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrShareNotFound):
			http.Error(w, "link not found", http.StatusNotFound)
		case errors.Is(err, store.ErrShareExpired), errors.Is(err, store.ErrShareExhausted):
			http.Error(w, err.Error(), http.StatusGone)
		default:
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	f, err := os.Open(filePath)
	if err != nil {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}
	h.store.Audit(model.AuditEntry{Action: "share.download", Target: share.JobID, IP: auth.ClientIP(r), Details: fileName})

	w.Header().Set("Content-Disposition", "attachment; filename="+fileName)
	w.Header().Set("Content-Type", contentTypeForFile(fileName))
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	w.Header().Set("Accept-Ranges", "none")
	w.WriteHeader(http.StatusOK)
	io.Copy(w, f)

	if exhaustedAfter {
		h.store.PurgeShare(token)
	}
}
