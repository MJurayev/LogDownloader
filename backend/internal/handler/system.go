package handler

import (
	"net/http"
	"time"

	"logdownloader/internal/metrics"
	"logdownloader/internal/model"
)

var startedAt = time.Now()

// healthz / readyz — auth'siz, Kubernetes probe'lari va monitoring uchun.
func (h *Handler) healthz(w http.ResponseWriter, r *http.Request) {
	running, queued := h.worker.Stats()
	writeJSON(w, map[string]any{
		"status":  "ok",
		"version": h.version,
		"uptime":  int64(time.Since(startedAt).Seconds()),
		"worker":  map[string]int{"running": running, "queued": queued},
	})
}

func (h *Handler) metricsSnapshot() metrics.Snapshot {
	counts := h.store.JobCounts()
	running, queued := h.worker.Stats()
	bytes, files := h.store.ExportsUsage()
	return metrics.Snapshot{
		Version:        h.version,
		JobsQueued:     counts[model.JobQueued],
		JobsRunning:    counts[model.JobRunning],
		JobsDone:       counts[model.JobDone],
		JobsFailed:     counts[model.JobFailed],
		WorkerRunning:  running,
		WorkerQueued:   queued,
		WorkerMax:      h.store.MaxConcurrent(),
		ExportsDirByte: bytes,
		ExportsDirFile: files,
		Schedules:      len(h.store.AllSchedules()),
		AuditLines:     h.store.AuditLineCount(),
	}
}
