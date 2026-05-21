package handler

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"logdownloader/internal/auth"
	"logdownloader/internal/model"
	"logdownloader/internal/store"
	"logdownloader/internal/vlclient"
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
	// Auth
	mux.HandleFunc("POST /api/login", h.login)
	mux.HandleFunc("GET /api/me", h.me)

	// Settings / Datasources
	mux.HandleFunc("GET /api/datasources", h.getDatasources)
	mux.HandleFunc("POST /api/datasources", h.createDatasource)
	mux.HandleFunc("PUT /api/datasources/{id}", h.updateDatasource)
	mux.HandleFunc("DELETE /api/datasources/{id}", h.deleteDatasource)

	// Queries
	mux.HandleFunc("GET /api/queries", h.getQueries)
	mux.HandleFunc("POST /api/queries", h.createQuery)
	mux.HandleFunc("PUT /api/queries/{id}", h.updateQuery)
	mux.HandleFunc("DELETE /api/queries/{id}", h.deleteQuery)

	// Log query & export
	mux.HandleFunc("GET /api/query", h.queryLogs)
	mux.HandleFunc("POST /api/export", h.startExport)
	mux.HandleFunc("GET /api/jobs", h.getJobs)
	mux.HandleFunc("DELETE /api/jobs/{id}", h.deleteJob)
	mux.HandleFunc("GET /api/jobs/{id}/download", h.downloadJob)

	// Share links
	mux.HandleFunc("POST /api/jobs/{id}/share", h.createShare)
	mux.HandleFunc("GET /api/jobs/{id}/share", h.getShare)
	mux.HandleFunc("DELETE /api/jobs/{id}/share", h.revokeShare)
	// Public download — `/dl/...` /api/ prefiksiga kirmaydi, shu sababli auth
	// middleware uni avtomatik o'tkazib yuboradi.
	mux.HandleFunc("GET /dl/{token}", h.downloadShare)

	// User management (admin only)
	mux.HandleFunc("GET /api/users", auth.RequireAdmin(h.getUsers))
	mux.HandleFunc("POST /api/users", auth.RequireAdmin(h.createUser))
	mux.HandleFunc("PUT /api/users/{id}", auth.RequireAdmin(h.updateUser))
	mux.HandleFunc("DELETE /api/users/{id}", auth.RequireAdmin(h.deleteUser))
}

// Auth

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	user := h.store.GetUserByUsername(req.Username)
	if user == nil || !auth.CheckPassword(user.Password, req.Password) {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	token, err := auth.GenerateToken(*user)
	if err != nil {
		http.Error(w, "token generation failed", http.StatusInternalServerError)
		return
	}

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
	writeJSON(w, map[string]any{
		"user_id":  u.ID,
		"username": u.Username,
		"role":     u.Role,
	})
}

// Users (admin only)

func (h *Handler) getUsers(w http.ResponseWriter, r *http.Request) {
	users := h.store.GetUsers()
	// Strip passwords
	safe := make([]map[string]any, len(users))
	for i, u := range users {
		safe[i] = map[string]any{"id": u.ID, "username": u.Username, "role": u.Role}
	}
	writeJSON(w, safe)
}

func (h *Handler) createUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string     `json:"username"`
		Password string     `json:"password"`
		Role     model.Role `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
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

	user := model.User{
		ID:       uuid.NewString(),
		Username: req.Username,
		Password: hash,
		Role:     req.Role,
	}
	if err := h.store.SaveUser(user); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]any{"id": user.ID, "username": user.Username, "role": user.Role})
}

func (h *Handler) updateUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Role     model.Role `json:"role"`
		Password string     `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	user := h.store.GetUser(id)
	if user == nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	if req.Role != "" {
		user.Role = req.Role
	}
	if req.Password != "" {
		hash, err := auth.HashPassword(req.Password)
		if err != nil {
			http.Error(w, "failed to hash password", http.StatusInternalServerError)
			return
		}
		user.Password = hash
	}

	if err := h.store.SaveUser(*user); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"id": user.ID, "username": user.Username, "role": user.Role})
}

func (h *Handler) deleteUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	u := auth.GetUser(r)
	if u != nil && u.ID == id {
		http.Error(w, "cannot delete yourself", http.StatusBadRequest)
		return
	}
	h.store.DeleteUser(id)
	w.WriteHeader(http.StatusNoContent)
}

// Datasources

func (h *Handler) getDatasources(w http.ResponseWriter, r *http.Request) {
	u := auth.GetUser(r)
	writeJSON(w, h.store.GetDatasourcesForUser(u.ID))
}

func (h *Handler) createDatasource(w http.ResponseWriter, r *http.Request) {
	u := auth.GetUser(r)
	var ds model.Datasource
	if err := json.NewDecoder(r.Body).Decode(&ds); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ds.ID = uuid.NewString()
	ds.OwnerID = u.ID

	// Only admin can create global datasources
	if ds.Global && u.Role != model.RoleAdmin {
		ds.Global = false
	}

	settings := h.store.GetSettings()
	settings.Datasources = append(settings.Datasources, ds)
	if err := h.store.SaveSettings(settings); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, ds)
}

func (h *Handler) updateDatasource(w http.ResponseWriter, r *http.Request) {
	u := auth.GetUser(r)
	id := r.PathValue("id")

	var input model.Datasource
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	settings := h.store.GetSettings()
	found := false
	for i := range settings.Datasources {
		if settings.Datasources[i].ID != id {
			continue
		}
		existing := settings.Datasources[i]
		if existing.OwnerID != u.ID && u.Role != model.RoleAdmin {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		// Preserve immutable fields; restrict global flag to admins.
		input.ID = existing.ID
		input.OwnerID = existing.OwnerID
		if input.Global && u.Role != model.RoleAdmin {
			input.Global = existing.Global
		}
		settings.Datasources[i] = input
		found = true
		break
	}
	if !found {
		http.Error(w, "datasource not found", http.StatusNotFound)
		return
	}
	if err := h.store.SaveSettings(settings); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, input)
}

func (h *Handler) deleteDatasource(w http.ResponseWriter, r *http.Request) {
	u := auth.GetUser(r)
	id := r.PathValue("id")

	settings := h.store.GetSettings()
	var filtered []model.Datasource
	for _, ds := range settings.Datasources {
		if ds.ID == id {
			// Only owner or admin can delete
			if ds.OwnerID != u.ID && u.Role != model.RoleAdmin {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			continue
		}
		filtered = append(filtered, ds)
	}
	settings.Datasources = filtered
	h.store.SaveSettings(settings)
	w.WriteHeader(http.StatusNoContent)
}

// Queries

func (h *Handler) getQueries(w http.ResponseWriter, r *http.Request) {
	u := auth.GetUser(r)
	writeJSON(w, h.store.GetQueriesForUser(u.ID))
}

func (h *Handler) createQuery(w http.ResponseWriter, r *http.Request) {
	u := auth.GetUser(r)
	var q model.SavedQuery
	if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	q.ID = uuid.NewString()
	q.OwnerID = u.ID

	// Only admin can create global queries
	if q.Global && u.Role != model.RoleAdmin {
		q.Global = false
	}

	if err := h.store.SaveQuery(q); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, q)
}

func (h *Handler) updateQuery(w http.ResponseWriter, r *http.Request) {
	u := auth.GetUser(r)
	id := r.PathValue("id")

	var input model.SavedQuery
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var existing *model.SavedQuery
	for _, q := range h.store.GetQueries() {
		if q.ID == id {
			qq := q
			existing = &qq
			break
		}
	}
	if existing == nil {
		http.Error(w, "query not found", http.StatusNotFound)
		return
	}
	if existing.OwnerID != u.ID && u.Role != model.RoleAdmin {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	input.ID = existing.ID
	input.OwnerID = existing.OwnerID
	if input.Global && u.Role != model.RoleAdmin {
		input.Global = existing.Global
	}

	if err := h.store.SaveQuery(input); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, input)
}

func (h *Handler) deleteQuery(w http.ResponseWriter, r *http.Request) {
	u := auth.GetUser(r)
	id := r.PathValue("id")

	queries := h.store.GetQueries()
	for _, q := range queries {
		if q.ID == id {
			if q.OwnerID != u.ID && u.Role != model.RoleAdmin {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
		}
	}

	if err := h.store.DeleteQuery(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Export & Jobs

func (h *Handler) startExport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Query        string `json:"query"`
		DatasourceID string `json:"datasource_id"`
		Start        string `json:"start"`
		End          string `json:"end"`
		SortOrder    string `json:"sort_order"`
		Name         string `json:"name"`
		Format       string `json:"format"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Query == "" || req.DatasourceID == "" {
		http.Error(w, "query and datasource_id are required", http.StatusBadRequest)
		return
	}

	jobID := h.worker.StartExport(req.Query, req.DatasourceID, req.Start, req.End, req.SortOrder, req.Name, req.Format)
	writeJSON(w, map[string]string{"job_id": jobID})
}

// contentTypeForFile fayl kengaytmasiga qarab Content-Type qaytaradi.
func contentTypeForFile(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return "application/gzip"
	case strings.HasSuffix(lower, ".gz"):
		return "application/gzip"
	default:
		return "application/octet-stream"
	}
}

func (h *Handler) getJobs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, h.store.GetJobs())
}

func (h *Handler) deleteJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	h.store.DeleteJob(id)
	w.WriteHeader(http.StatusNoContent)
}

// Share links

func (h *Handler) createShare(w http.ResponseWriter, r *http.Request) {
	u := auth.GetUser(r)
	jobID := r.PathValue("id")

	job := h.store.GetJob(jobID)
	if job == nil {
		http.Error(w, "job not found", http.StatusNotFound)
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
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

	token, err := generateShareToken()
	if err != nil {
		http.Error(w, "failed to generate token", http.StatusInternalServerError)
		return
	}

	share := &model.ShareLink{
		Token:         token,
		JobID:         jobID,
		CreatedBy:     u.ID,
		CreatedAt:     time.Now(),
		ExpiresAt:     expiresAt,
		MaxDownloads:  req.MaxDownloads,
		DownloadCount: 0,
	}
	if err := h.store.SaveShare(share); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, shareResponse(r, share))
}

func (h *Handler) getShare(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("id")
	share := h.store.GetShareByJob(jobID)
	if share == nil {
		http.Error(w, "no share for this job", http.StatusNotFound)
		return
	}
	writeJSON(w, shareResponse(r, share))
}

func (h *Handler) revokeShare(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("id")
	share := h.store.GetShareByJob(jobID)
	if share == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := h.store.DeleteShare(share.Token); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) downloadShare(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")

	share, filePath, err, exhaustedAfter := h.store.ConsumeShare(token)
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

	job := h.store.GetJob(share.JobID)
	fileName := "export.log"
	if job != nil {
		fileName = job.FileName
	}

	w.Header().Set("Content-Disposition", "attachment; filename="+fileName)
	w.Header().Set("Content-Type", contentTypeForFile(fileName))
	http.ServeFile(w, r, filePath)

	if exhaustedAfter {
		h.store.PurgeShare(token)
	}
}

func generateShareToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// shareResponse — share modelni response uchun tayyorlaydi, full URL ham qo'shadi.
func shareResponse(r *http.Request, sh *model.ShareLink) map[string]any {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	host := r.Host
	if h := r.Header.Get("X-Forwarded-Host"); h != "" {
		host = h
	}
	return map[string]any{
		"token":          sh.Token,
		"job_id":         sh.JobID,
		"created_by":     sh.CreatedBy,
		"created_at":     sh.CreatedAt,
		"expires_at":     sh.ExpiresAt,
		"max_downloads":  sh.MaxDownloads,
		"download_count": sh.DownloadCount,
		"url":            fmt.Sprintf("%s://%s/dl/%s", scheme, host, sh.Token),
	}
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
	w.Header().Set("Content-Type", contentTypeForFile(job.FileName))
	http.ServeFile(w, r, filePath)
}

// Query logs

func (h *Handler) queryLogs(w http.ResponseWriter, r *http.Request) {
	dsID := r.URL.Query().Get("datasource_id")
	if dsID == "" {
		http.Error(w, "datasource_id is required", http.StatusBadRequest)
		return
	}

	ds := h.store.GetDatasource(dsID)
	if ds == nil {
		http.Error(w, "datasource not found", http.StatusNotFound)
		return
	}

	query := r.URL.Query().Get("query")
	if query == "" {
		http.Error(w, "query parameter is required", http.StatusBadRequest)
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}

	fetchLimit := limit + offset

	baseURL := strings.TrimRight(ds.URL, "/")
	vlURL := fmt.Sprintf("%s/select/logsql/query", baseURL)

	sortOrder := r.URL.Query().Get("sort")
	finalQuery := vlclient.WithTimeSort(query, sortOrder)

	params := url.Values{}
	params.Set("query", finalQuery)
	params.Set("limit", strconv.Itoa(fetchLimit))

	if start := r.URL.Query().Get("start"); start != "" {
		params.Set("start", start)
	}
	if end := r.URL.Query().Get("end"); end != "" {
		params.Set("end", end)
	}

	resp, err := vlclient.Do("GET", vlURL+"?"+params.Encode(), *ds)
	if err != nil {
		http.Error(w, fmt.Sprintf("VictoriaLogs request failed: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		http.Error(w, fmt.Sprintf("VictoriaLogs error: %s", string(body)), resp.StatusCode)
		return
	}

	var allLogs []json.RawMessage
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		allLogs = append(allLogs, json.RawMessage(append([]byte{}, line...)))
	}

	total := len(allLogs)
	hasMore := total == fetchLimit

	if offset >= len(allLogs) {
		allLogs = nil
	} else {
		allLogs = allLogs[offset:]
	}
	if len(allLogs) > limit {
		allLogs = allLogs[:limit]
	}

	result := map[string]any{
		"logs":     allLogs,
		"total":    total,
		"has_more": hasMore,
		"limit":    limit,
		"offset":   offset,
	}
	writeJSON(w, result)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

