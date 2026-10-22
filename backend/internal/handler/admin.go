package handler

import (
	"net/http"
	"os"
	"strconv"
	"strings"

	"logdownloader/internal/auth"
	"logdownloader/internal/ldapauth"
	"logdownloader/internal/model"
)

// Audit (admin)

func (h *Handler) getAudit(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	offset, _ := strconv.Atoi(q.Get("offset"))
	if offset < 0 {
		offset = 0
	}
	entries, total := h.store.ReadAudit(limit, offset, q.Get("action"), q.Get("user"))
	if entries == nil {
		entries = []model.AuditEntry{}
	}
	writeJSON(w, map[string]any{"entries": entries, "total": total, "limit": limit, "offset": offset})
}

func (h *Handler) downloadAudit(w http.ResponseWriter, r *http.Request) {
	path := h.store.AuditFilePath()
	if _, err := os.Stat(path); err != nil {
		http.Error(w, "audit log is empty", http.StatusNotFound)
		return
	}
	h.audit(r, "audit.download", "", "")
	w.Header().Set("Content-Disposition", "attachment; filename=audit.jsonl")
	w.Header().Set("Content-Type", "application/x-ndjson")
	http.ServeFile(w, r, path)
}

// Settings (admin): retention, worker, notify kanallari, public_url.
// Sirlar (telegram token, SMTP parol) javobda qaytmaydi — faqat has_* flag.

type settingsView struct {
	Retention model.RetentionSettings `json:"retention"`
	Worker    model.WorkerSettings    `json:"worker"`
	PublicURL string                  `json:"public_url"`
	Notify    struct {
		HasTelegramToken bool   `json:"has_telegram_token"`
		WebhookURL       string `json:"webhook_url"`
		SMTP             struct {
			Host        string `json:"host"`
			Port        int    `json:"port"`
			Username    string `json:"username"`
			HasPassword bool   `json:"has_password"`
			From        string `json:"from"`
			StartTLS    bool   `json:"starttls"`
		} `json:"smtp"`
	} `json:"notify"`
	ExportChunkMB int      `json:"export_chunk_mb"`
	LDAP          ldapView `json:"ldap"`
}

// ldapView — LDAP sozlamalari (bind paroli yashirin).
type ldapView struct {
	Enabled            bool       `json:"enabled"`
	URL                string     `json:"url"`
	StartTLS           bool       `json:"starttls"`
	InsecureSkipVerify bool       `json:"insecure_skip_verify"`
	BindDN             string     `json:"bind_dn"`
	HasBindPassword    bool       `json:"has_bind_password"`
	BaseDN             string     `json:"base_dn"`
	UserFilter         string     `json:"user_filter"`
	UsernameAttr       string     `json:"username_attr"`
	EmailAttr          string     `json:"email_attr"`
	AdminGroupDN       string     `json:"admin_group_dn"`
	AllowedGroupDN     string     `json:"allowed_group_dn"`
	DefaultRole        model.Role `json:"default_role"`
	TimeoutSec         int        `json:"timeout_sec"`
}

func (h *Handler) settingsView() settingsView {
	st := h.store.GetSettings()
	var v settingsView
	v.Retention = st.Retention
	v.Worker = st.Worker
	v.PublicURL = st.PublicURL
	v.Notify.HasTelegramToken = st.Notify.TelegramBotToken != ""
	v.Notify.WebhookURL = st.Notify.WebhookURL
	v.Notify.SMTP.Host = st.Notify.SMTP.Host
	v.Notify.SMTP.Port = st.Notify.SMTP.Port
	v.Notify.SMTP.Username = st.Notify.SMTP.Username
	v.Notify.SMTP.HasPassword = st.Notify.SMTP.Password != ""
	v.Notify.SMTP.From = st.Notify.SMTP.From
	v.Notify.SMTP.StartTLS = st.Notify.SMTP.StartTLS
	v.ExportChunkMB = h.exportChunkMB
	l := st.LDAP
	v.LDAP = ldapView{
		Enabled: l.Enabled, URL: l.URL, StartTLS: l.StartTLS, InsecureSkipVerify: l.InsecureSkipVerify,
		BindDN: l.BindDN, HasBindPassword: l.BindPassword != "", BaseDN: l.BaseDN, UserFilter: l.UserFilter,
		UsernameAttr: l.UsernameAttr, EmailAttr: l.EmailAttr, AdminGroupDN: l.AdminGroupDN, AllowedGroupDN: l.AllowedGroupDN,
		DefaultRole: l.DefaultRole, TimeoutSec: l.TimeoutSec,
	}
	return v
}

// ldapInput — PUT /api/settings `ldap` bo'limi; bind_password bo'sh = o'zgarmaydi.
type ldapInput struct {
	Enabled            bool       `json:"enabled"`
	URL                string     `json:"url"`
	StartTLS           bool       `json:"starttls"`
	InsecureSkipVerify bool       `json:"insecure_skip_verify"`
	BindDN             string     `json:"bind_dn"`
	BindPassword       string     `json:"bind_password"`
	ClearBindPassword  bool       `json:"clear_bind_password"`
	BaseDN             string     `json:"base_dn"`
	UserFilter         string     `json:"user_filter"`
	UsernameAttr       string     `json:"username_attr"`
	EmailAttr          string     `json:"email_attr"`
	AdminGroupDN       string     `json:"admin_group_dn"`
	AllowedGroupDN     string     `json:"allowed_group_dn"`
	DefaultRole        model.Role `json:"default_role"`
	TimeoutSec         int        `json:"timeout_sec"`
}

func applyLDAP(dst *model.LDAPSettings, in *ldapInput) {
	dst.Enabled = in.Enabled
	dst.URL = strings.TrimSpace(in.URL)
	dst.StartTLS = in.StartTLS
	dst.InsecureSkipVerify = in.InsecureSkipVerify
	dst.BindDN = strings.TrimSpace(in.BindDN)
	if in.ClearBindPassword {
		dst.BindPassword = ""
	} else if in.BindPassword != "" {
		dst.BindPassword = in.BindPassword
	}
	dst.BaseDN = strings.TrimSpace(in.BaseDN)
	dst.UserFilter = strings.TrimSpace(in.UserFilter)
	dst.UsernameAttr = strings.TrimSpace(in.UsernameAttr)
	dst.EmailAttr = strings.TrimSpace(in.EmailAttr)
	dst.AdminGroupDN = strings.TrimSpace(in.AdminGroupDN)
	dst.AllowedGroupDN = strings.TrimSpace(in.AllowedGroupDN)
	dst.DefaultRole = in.DefaultRole
	if dst.DefaultRole != model.RoleAdmin {
		dst.DefaultRole = model.RoleUser
	}
	dst.TimeoutSec = in.TimeoutSec
	if dst.TimeoutSec < 0 {
		dst.TimeoutSec = 0
	}
}

// testLDAP — admin "Test": ulanish + service bind + base DN; username/parol
// berilsa to'liq login sinovi (DN, guruhlar, admin bo'ladimi). Saqlanmagan
// forma qiymatlari bilan ishlaydi; bind paroli bo'sh bo'lsa saqlangani olinadi.
func (h *Handler) testLDAP(w http.ResponseWriter, r *http.Request) {
	var req struct {
		LDAP     ldapInput `json:"ldap"`
		Username string    `json:"username"`
		Password string    `json:"password"`
	}
	if !decode(w, r, &req) {
		return
	}
	cfg := h.store.GetSettings().LDAP
	if req.LDAP.BindPassword == "" && !req.LDAP.ClearBindPassword {
		req.LDAP.BindPassword = cfg.BindPassword
	}
	applyLDAP(&cfg, &req.LDAP)
	res, err := ldapauth.TestConnection(r.Context(), cfg, strings.TrimSpace(req.Username), req.Password)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "result": res})
}

func (h *Handler) getSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, h.settingsView())
}

func (h *Handler) updateSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Retention *model.RetentionSettings `json:"retention"`
		Worker    *model.WorkerSettings    `json:"worker"`
		PublicURL *string                  `json:"public_url"`
		LDAP      *ldapInput               `json:"ldap"`
		Notify    *struct {
			TelegramBotToken   string  `json:"telegram_bot_token"` // bo'sh = o'zgarmasin
			ClearTelegramToken bool    `json:"clear_telegram_token"`
			WebhookURL         *string `json:"webhook_url"`
			SMTP               *struct {
				Host          string `json:"host"`
				Port          int    `json:"port"`
				Username      string `json:"username"`
				Password      string `json:"password"` // bo'sh = o'zgarmasin
				ClearPassword bool   `json:"clear_password"`
				From          string `json:"from"`
				StartTLS      bool   `json:"starttls"`
			} `json:"smtp"`
		} `json:"notify"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.Retention != nil && (req.Retention.Days < 0 || req.Retention.QuotaGB < 0) {
		http.Error(w, "retention values must be >= 0", http.StatusBadRequest)
		return
	}
	if req.Worker != nil && (req.Worker.MaxConcurrent < 1 || req.Worker.MaxConcurrent > 64) {
		http.Error(w, "max_concurrent must be 1..64", http.StatusBadRequest)
		return
	}
	err := h.store.UpdateSettings(func(st *model.Settings) {
		if req.Retention != nil {
			st.Retention = *req.Retention
		}
		if req.Worker != nil {
			st.Worker = *req.Worker
		}
		if req.PublicURL != nil {
			st.PublicURL = strings.TrimRight(strings.TrimSpace(*req.PublicURL), "/")
		}
		if req.LDAP != nil {
			applyLDAP(&st.LDAP, req.LDAP)
		}
		if req.Notify != nil {
			if req.Notify.ClearTelegramToken {
				st.Notify.TelegramBotToken = ""
			} else if t := strings.TrimSpace(req.Notify.TelegramBotToken); t != "" {
				st.Notify.TelegramBotToken = t
			}
			if req.Notify.WebhookURL != nil {
				st.Notify.WebhookURL = strings.TrimSpace(*req.Notify.WebhookURL)
			}
			if s := req.Notify.SMTP; s != nil {
				st.Notify.SMTP.Host = strings.TrimSpace(s.Host)
				st.Notify.SMTP.Port = s.Port
				st.Notify.SMTP.Username = strings.TrimSpace(s.Username)
				st.Notify.SMTP.From = strings.TrimSpace(s.From)
				st.Notify.SMTP.StartTLS = s.StartTLS
				if s.ClearPassword {
					st.Notify.SMTP.Password = ""
				} else if s.Password != "" {
					st.Notify.SMTP.Password = s.Password
				}
			}
		}
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.worker.Signal()
	h.audit(r, "settings.update", "", "")
	writeJSON(w, h.settingsView())
}

func (h *Handler) testNotify(w http.ResponseWriter, r *http.Request) {
	u := auth.GetUser(r)
	var req struct {
		Channel string `json:"channel"`
	}
	if !decode(w, r, &req) {
		return
	}
	if err := h.notifier.SendTest(req.Channel, h.store.GetUser(u.ID)); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}
