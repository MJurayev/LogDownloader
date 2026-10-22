package handler

import (
	"net/http"
	"strings"
	"time"

	"logdownloader/internal/auth"
	"logdownloader/internal/model"
	"logdownloader/internal/scheduler"

	"github.com/google/uuid"
)

type scheduleRequest struct {
	Name            string   `json:"name"`
	Query           string   `json:"query"`
	DatasourceID    string   `json:"datasource_id"`
	Cron            string   `json:"cron"`
	LookbackMinutes int      `json:"lookback_minutes"`
	Align           bool     `json:"align"`
	SortOrder       string   `json:"sort_order"`
	Format          string   `json:"format"`
	Content         string   `json:"content"`
	Fields          []string `json:"fields"`
	ArchiveSplitMB  int      `json:"archive_split_mb"`
	Notify          bool     `json:"notify"`
	AutoShare       bool     `json:"auto_share"`
	Enabled         bool     `json:"enabled"`
}

func (h *Handler) validateSchedule(w http.ResponseWriter, r *http.Request, req *scheduleRequest) bool {
	u := auth.GetUser(r)
	req.Name = strings.TrimSpace(req.Name)
	req.Query = strings.TrimSpace(req.Query)
	req.Cron = strings.TrimSpace(req.Cron)
	if req.Name == "" || req.Query == "" || req.DatasourceID == "" || req.Cron == "" {
		http.Error(w, "name, query, datasource_id and cron are required", http.StatusBadRequest)
		return false
	}
	if h.canUseDatasource(u, req.DatasourceID) == nil {
		http.Error(w, "datasource not found", http.StatusNotFound)
		return false
	}
	if _, err := scheduler.Parse(req.Cron); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return false
	}
	if req.LookbackMinutes <= 0 {
		req.LookbackMinutes = 60
	}
	return true
}

func applySchedule(sc *model.Schedule, req *scheduleRequest) {
	sc.Name = req.Name
	sc.Query = req.Query
	sc.DatasourceID = req.DatasourceID
	sc.Cron = req.Cron
	sc.LookbackMinutes = req.LookbackMinutes
	sc.Align = req.Align
	sc.SortOrder = req.SortOrder
	sc.Format = req.Format
	sc.Content = req.Content
	sc.Fields = req.Fields
	sc.ArchiveSplitMB = req.ArchiveSplitMB
	sc.Notify = req.Notify
	sc.AutoShare = req.AutoShare
	sc.Enabled = req.Enabled
}

func (h *Handler) loadScheduleForUser(w http.ResponseWriter, r *http.Request, id string) *model.Schedule {
	u := auth.GetUser(r)
	sc := h.store.GetSchedule(id)
	if sc == nil {
		http.Error(w, "schedule not found", http.StatusNotFound)
		return nil
	}
	if sc.OwnerID != u.ID && u.Role != model.RoleAdmin {
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil
	}
	return sc
}

func (h *Handler) getSchedules(w http.ResponseWriter, r *http.Request) {
	u := auth.GetUser(r)
	writeJSON(w, h.store.GetSchedulesForUser(u.ID, u.Role == model.RoleAdmin))
}

func (h *Handler) createSchedule(w http.ResponseWriter, r *http.Request) {
	u := auth.GetUser(r)
	var req scheduleRequest
	if !decode(w, r, &req) {
		return
	}
	if !h.validateSchedule(w, r, &req) {
		return
	}
	sc := &model.Schedule{ID: uuid.NewString(), OwnerID: u.ID, OwnerName: u.Username, CreatedAt: time.Now()}
	applySchedule(sc, &req)
	if sc.Enabled {
		next, err := scheduler.ComputeNext(sc.Cron, time.Now())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		sc.NextRunAt = next
	}
	if err := h.store.SaveSchedule(sc); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.audit(r, "schedule.create", sc.Name, sc.Cron)
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, sc)
}

func (h *Handler) updateSchedule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sc := h.loadScheduleForUser(w, r, id)
	if sc == nil {
		return
	}
	var req scheduleRequest
	if !decode(w, r, &req) {
		return
	}
	if !h.validateSchedule(w, r, &req) {
		return
	}
	cronChanged := sc.Cron != req.Cron
	applySchedule(sc, &req)
	if !sc.Enabled {
		sc.NextRunAt = nil
	} else if cronChanged || sc.NextRunAt == nil {
		next, err := scheduler.ComputeNext(sc.Cron, time.Now())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		sc.NextRunAt = next
	}
	if err := h.store.SaveSchedule(sc); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.audit(r, "schedule.update", sc.Name, sc.Cron)
	writeJSON(w, sc)
}

func (h *Handler) deleteSchedule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sc := h.loadScheduleForUser(w, r, id)
	if sc == nil {
		return
	}
	if err := h.store.DeleteSchedule(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.audit(r, "schedule.delete", sc.Name, "")
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) runSchedule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sc := h.loadScheduleForUser(w, r, id)
	if sc == nil {
		return
	}
	jobID, err := h.scheduler.RunNow(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if jobID == "" {
		http.Error(w, "export could not be started (see schedule last_error)", http.StatusBadGateway)
		return
	}
	h.audit(r, "schedule.run", sc.Name, jobID)
	writeJSON(w, map[string]string{"job_id": jobID})
}
