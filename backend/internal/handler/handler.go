// Package handler — barcha HTTP endpointlar. Fayllar bo'yicha:
// handler.go (auth, users, profil), datasources.go, queries.go, logs.go
// (viewer proxy'lari), jobs.go, shares.go, schedules.go, admin.go (audit,
// sozlamalar), system.go (healthz, metrics).
package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"logdownloader/internal/auth"
	"logdownloader/internal/metrics"
	"logdownloader/internal/model"
	"logdownloader/internal/notify"
	"logdownloader/internal/scheduler"
	"logdownloader/internal/store"
	"logdownloader/internal/worker"

	"github.com/google/uuid"
)

type Handler struct {
	store     *store.Store
	worker    *worker.Worker
	notifier  *notify.Notifier
	scheduler *scheduler.Scheduler
	limiter   *auth.Limiter
	version   string

	exportChunkMB int
}

func New(s *store.Store, w *worker.Worker, n *notify.Notifier, sc *scheduler.Scheduler, version string) *Handler {
	return &Handler{store: s, worker: w, notifier: n, scheduler: sc, limiter: auth.NewLimiter(), version: version}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// Auth / profil
	mux.HandleFunc("POST /api/login", h.login)
	mux.HandleFunc("GET /api/me", h.me)
	mux.HandleFunc("PUT /api/me", h.updateMe)

	// Datasources
	mux.HandleFunc("GET /api/datasources", h.getDatasources)
	mux.HandleFunc("POST /api/datasources", h.createDatasource)
	mux.HandleFunc("POST /api/datasources/test", h.testDatasource)
	mux.HandleFunc("PUT /api/datasources/{id}", h.updateDatasource)
	mux.HandleFunc("DELETE /api/datasources/{id}", h.deleteDatasource)

	// Saved queries
	mux.HandleFunc("GET /api/queries", h.getQueries)
	mux.HandleFunc("POST /api/queries", h.createQuery)
	mux.HandleFunc("PUT /api/queries/{id}", h.updateQuery)
	mux.HandleFunc("DELETE /api/queries/{id}", h.deleteQuery)

	// Log viewer (VictoriaLogs proxy)
	mux.HandleFunc("GET /api/query", h.queryLogs)
	mux.HandleFunc("GET /api/fields", h.fieldNames)
	mux.HandleFunc("GET /api/field_values", h.fieldValues)
	mux.HandleFunc("GET /api/streams", h.streams)
	mux.HandleFunc("GET /api/hits", h.hits)
	mux.HandleFunc("GET /api/tail", h.tail)
	mux.HandleFunc("GET /api/history", h.getHistory)
	mux.HandleFunc("DELETE /api/history", h.clearHistory)

	// Export & jobs
	mux.HandleFunc("POST /api/export", h.startExport)
	mux.HandleFunc("GET /api/jobs", h.getJobs)
	mux.HandleFunc("DELETE /api/jobs/{id}", h.deleteJob)
	mux.HandleFunc("GET /api/jobs/{id}/download", h.downloadJob)
	mux.HandleFunc("POST /api/jobs/{id}/download-token", h.downloadToken)
	mux.HandleFunc("GET /api/stats", h.stats)

	// Share links
	mux.HandleFunc("POST /api/jobs/{id}/share", h.createShare)
	mux.HandleFunc("GET /api/jobs/{id}/share", h.getShare)
	mux.HandleFunc("DELETE /api/jobs/{id}/share", h.revokeShare)
	// Public download — `/dl/...` /api/ prefiksiga kirmaydi, auth middleware o'tkazib yuboradi.
	mux.HandleFunc("GET /dl/{token}", h.downloadShare)
	mux.HandleFunc("GET /dl/{token}/{n}", h.downloadShare)

	// Schedules
	mux.HandleFunc("GET /api/schedules", h.getSchedules)
	mux.HandleFunc("POST /api/schedules", h.createSchedule)
	mux.HandleFunc("PUT /api/schedules/{id}", h.updateSchedule)
	mux.HandleFunc("DELETE /api/schedules/{id}", h.deleteSchedule)
	mux.HandleFunc("POST /api/schedules/{id}/run", h.runSchedule)

	// Admin
	mux.HandleFunc("GET /api/users", auth.RequireAdmin(h.getUsers))
	mux.HandleFunc("POST /api/users", auth.RequireAdmin(h.createUser))
	mux.HandleFunc("PUT /api/users/{id}", auth.RequireAdmin(h.updateUser))
	mux.HandleFunc("DELETE /api/users/{id}", auth.RequireAdmin(h.deleteUser))
	mux.HandleFunc("GET /api/audit", auth.RequireAdmin(h.getAudit))
	mux.HandleFunc("GET /api/audit/download", auth.RequireAdmin(h.downloadAudit))
	mux.HandleFunc("GET /api/settings", auth.RequireAdmin(h.getSettings))
	mux.HandleFunc("PUT /api/settings", auth.RequireAdmin(h.updateSettings))
	mux.HandleFunc("POST /api/settings/notify/test", auth.RequireAdmin(h.testNotify))

	// System (auth'siz)
	mux.HandleFunc("GET /healthz", h.healthz)
	mux.HandleFunc("GET /readyz", h.healthz)
	mux.HandleFunc("GET /metrics", metrics.Handler(h.metricsSnapshot))
}

// Helpers

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

// audit — joriy user (bo'lsa) bilan audit yozuvi.
func (h *Handler) audit(r *http.Request, action, target, details string) {
	e := model.AuditEntry{Time: time.Now(), Action: action, Target: target, IP: auth.ClientIP(r), Details: details}
	if u := auth.GetUser(r); u != nil {
		e.UserID = u.ID
		e.User = u.Username
	}
	h.store.Audit(e)
}

func userView(u model.User) map[string]any {
	return map[string]any{
		"id":               u.ID,
		"username":         u.Username,
		"role":             u.Role,
		"email":            u.Email,
		"telegram_chat_id": u.TelegramChatID,
		"notify_on_done":   u.NotifyOnDone,
	}
}

// Auth

const (
	loginIPWindow      = time.Minute
	loginIPMax         = 30
	loginFailWindow    = 15 * time.Minute
	loginFailMax       = 5
	loginBlockedMsg    = "too many failed attempts, try again later"
	loginRateLimitMsg  = "too many requests"
	retryAfterSeconds  = "900"
	retryAfterIPSecond = "60"
)

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	ip := auth.ClientIP(r)
	if h.limiter.Hit("ip:"+ip, loginIPWindow) > loginIPMax {
		metrics.LoginBlocked.Add(1)
		w.Header().Set("Retry-After", retryAfterIPSecond)
		http.Error(w, loginRateLimitMsg, http.StatusTooManyRequests)
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decode(w, r, &req) {
		return
	}
	failKey := "fail:" + strings.ToLower(req.Username) + "|" + ip
	if h.limiter.Count(failKey, loginFailWindow) >= loginFailMax {
		metrics.LoginBlocked.Add(1)
		h.store.Audit(model.AuditEntry{Action: "login.blocked", User: req.Username, IP: ip})
		w.Header().Set("Retry-After", retryAfterSeconds)
		http.Error(w, loginBlockedMsg, http.StatusTooManyRequests)
		return
	}

	user := h.store.GetUserByUsername(req.Username)
	if user == nil || !auth.CheckPassword(user.Password, req.Password) {
		h.limiter.Hit(failKey, loginFailWindow)
		metrics.LoginFailed.Add(1)
		h.store.Audit(model.AuditEntry{Action: "login.fail", User: req.Username, IP: ip})
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	h.limiter.Reset(failKey)

	token, err := auth.GenerateToken(*user)
	if err != nil {
		http.Error(w, "token generation failed", http.StatusInternalServerError)
		return
	}
	h.store.Audit(model.AuditEntry{Action: "login.ok", UserID: user.ID, User: user.Username, IP: ip})
	writeJSON(w, map[string]any{
		"token":    token,
		"user_id":  user.ID,
		"username": user.Username,
		"role":     user.Role,
	})
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	u := auth.GetUser(r)
	if u == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	full := h.store.GetUser(u.ID)
	if full == nil {
		http.Error(w, "user not found", http.StatusUnauthorized)
		return
	}
	writeJSON(w, map[string]any{
		"user_id":          full.ID,
		"username":         full.Username,
		"role":             full.Role,
		"email":            full.Email,
		"telegram_chat_id": full.TelegramChatID,
		"notify_on_done":   full.NotifyOnDone,
	})
}

// updateMe — profil (bildirishnoma manzillari) va o'z parolini o'zgartirish.
func (h *Handler) updateMe(w http.ResponseWriter, r *http.Request) {
	u := auth.GetUser(r)
	var req struct {
		Email           *string `json:"email"`
		TelegramChatID  *string `json:"telegram_chat_id"`
		NotifyOnDone    *bool   `json:"notify_on_done"`
		CurrentPassword string  `json:"current_password"`
		NewPassword     string  `json:"new_password"`
	}
	if !decode(w, r, &req) {
		return
	}
	user := h.store.GetUser(u.ID)
	if user == nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}
	if req.Email != nil {
		user.Email = strings.TrimSpace(*req.Email)
	}
	if req.TelegramChatID != nil {
		user.TelegramChatID = strings.TrimSpace(*req.TelegramChatID)
	}
	if req.NotifyOnDone != nil {
		user.NotifyOnDone = *req.NotifyOnDone
	}
	changedPassword := false
	if req.NewPassword != "" {
		if !auth.CheckPassword(user.Password, req.CurrentPassword) {
			http.Error(w, "current password is wrong", http.StatusForbidden)
			return
		}
		if len(req.NewPassword) < 6 {
			http.Error(w, "password must be at least 6 characters", http.StatusBadRequest)
			return
		}
		hash, err := auth.HashPassword(req.NewPassword)
		if err != nil {
			http.Error(w, "failed to hash password", http.StatusInternalServerError)
			return
		}
		user.Password = hash
		changedPassword = true
	}
	if err := h.store.SaveUser(*user); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if changedPassword {
		h.audit(r, "password.change", user.Username, "")
	} else {
		h.audit(r, "profile.update", user.Username, "")
	}
	writeJSON(w, userView(*user))
}

// Users (admin only)

func (h *Handler) getUsers(w http.ResponseWriter, r *http.Request) {
	users := h.store.GetUsers()
	safe := make([]map[string]any, 0, len(users))
	for _, u := range users {
		safe = append(safe, userView(u))
	}
	writeJSON(w, safe)
}

func (h *Handler) createUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string     `json:"username"`
		Password string     `json:"password"`
		Role     model.Role `json:"role"`
		Email    string     `json:"email"`
	}
	if !decode(w, r, &req) {
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		http.Error(w, "username and password required", http.StatusBadRequest)
		return
	}
	if req.Role == "" {
		req.Role = model.RoleUser
	}
	if existing := h.store.GetUserByUsername(req.Username); existing != nil {
		http.Error(w, "username already exists", http.StatusConflict)
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		http.Error(w, "failed to hash password", http.StatusInternalServerError)
		return
	}
	user := model.User{ID: uuid.NewString(), Username: req.Username, Password: hash, Role: req.Role, Email: strings.TrimSpace(req.Email)}
	if err := h.store.SaveUser(user); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.audit(r, "user.create", user.Username, string(user.Role))
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, userView(user))
}

func (h *Handler) updateUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Role     model.Role `json:"role"`
		Password string     `json:"password"`
		Email    *string    `json:"email"`
	}
	if !decode(w, r, &req) {
		return
	}
	user := h.store.GetUser(id)
	if user == nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}
	details := []string{}
	if req.Role != "" && req.Role != user.Role {
		user.Role = req.Role
		details = append(details, "role="+string(req.Role))
	}
	if req.Password != "" {
		hash, err := auth.HashPassword(req.Password)
		if err != nil {
			http.Error(w, "failed to hash password", http.StatusInternalServerError)
			return
		}
		user.Password = hash
		details = append(details, "password")
	}
	if req.Email != nil {
		user.Email = strings.TrimSpace(*req.Email)
	}
	if err := h.store.SaveUser(*user); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.audit(r, "user.update", user.Username, strings.Join(details, ","))
	writeJSON(w, userView(*user))
}

func (h *Handler) deleteUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	u := auth.GetUser(r)
	if u != nil && u.ID == id {
		http.Error(w, "cannot delete yourself", http.StatusBadRequest)
		return
	}
	target := h.store.GetUser(id)
	if target == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	h.store.DeleteUser(id)
	h.audit(r, "user.delete", target.Username, "")
	w.WriteHeader(http.StatusNoContent)
}

// exportChunkMB — config qiymati (faqat ko'rsatish uchun, sozlamalar sahifasida).
func (h *Handler) SetExportChunkMB(mb int) { h.exportChunkMB = mb }
