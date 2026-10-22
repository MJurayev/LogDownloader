package model

import "time"

type Role string

const (
	RoleAdmin Role = "admin"
	RoleUser  Role = "user"
)

// Auth manbai: bo'sh yoki "local" — lokal parol; "ldap" — LDAP/AD (parol hash yo'q).
const (
	AuthLocal = "local"
	AuthLDAP  = "ldap"
)

type User struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Password string `json:"password,omitempty"` // bcrypt hash, omit in JSON responses
	Role     Role   `json:"role"`
	// AuthSource — "ldap" bo'lsa parol LDAP'da tekshiriladi; DN — LDAP yozuvi.
	AuthSource string `json:"auth_source,omitempty"`
	DN         string `json:"dn,omitempty"`
	// Bildirishnoma manzillari (profil). Bo'sh bo'lsa shu kanal ishlatilmaydi.
	Email          string `json:"email,omitempty"`
	TelegramChatID string `json:"telegram_chat_id,omitempty"`
	NotifyOnDone   bool   `json:"notify_on_done"`
}

type Datasource struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	URL      string            `json:"url"`
	Username string            `json:"username,omitempty"`
	Password string            `json:"password,omitempty"`
	Headers  map[string]string `json:"headers,omitempty"`
	// VictoriaLogs multitenancy: ikkalasi ham ixtiyoriy. Bo'sh bo'lmasa
	// `AccountID` / `ProjectID` HTTP header sifatida yuboriladi.
	AccountID string `json:"account_id,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	Global    bool   `json:"global"`
	OwnerID   string `json:"owner_id"`
}

// RetentionSettings — exportlarni avtomatik tozalash. 0 = o'chiq.
type RetentionSettings struct {
	Days    int     `json:"days"`     // done/failed joblar shuncha kundan keyin o'chadi
	QuotaGB float64 `json:"quota_gb"` // exports/ hajmi shundan oshsa eng eski done joblar o'chadi
}

// WorkerSettings — bir vaqtda ishlaydigan exportlar soni (qolgani navbatda).
type WorkerSettings struct {
	MaxConcurrent int `json:"max_concurrent"`
}

type SMTPSettings struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	From     string `json:"from"`
	StartTLS bool   `json:"starttls"`
}

// NotifySettings — bildirishnoma kanallari (admin sozlaydi). Qabul qiluvchi
// manzillar har bir userning profilida (Email, TelegramChatID).
type NotifySettings struct {
	TelegramBotToken string       `json:"telegram_bot_token,omitempty"`
	SMTP             SMTPSettings `json:"smtp"`
	WebhookURL       string       `json:"webhook_url,omitempty"`
}

// LDAPSettings — LDAP / Active Directory autentifikatsiyasi (admin sozlaydi).
// Kirish tartibi: lokal paroli bor user → lokal; aks holda Enabled bo'lsa LDAP.
type LDAPSettings struct {
	Enabled            bool   `json:"enabled"`
	URL                string `json:"url"`                  // ldap://dc:389 | ldaps://dc:636
	StartTLS           bool   `json:"starttls"`             // faqat ldap:// uchun
	InsecureSkipVerify bool   `json:"insecure_skip_verify"` // TLS sertifikatini tekshirmaslik
	BindDN             string `json:"bind_dn,omitempty"`    // service account; bo'sh = anonim
	BindPassword       string `json:"bind_password,omitempty"`
	BaseDN             string `json:"base_dn"`                    // DC=ucell,DC=uz
	UserFilter         string `json:"user_filter,omitempty"`      // default (&(objectClass=user)(sAMAccountName=%s))
	UsernameAttr       string `json:"username_attr,omitempty"`    // sAMAccountName
	EmailAttr          string `json:"email_attr,omitempty"`       // mail
	AdminGroupDN       string `json:"admin_group_dn,omitempty"`   // a'zolari admin (nested ham)
	AllowedGroupDN     string `json:"allowed_group_dn,omitempty"` // bo'sh = hamma kira oladi
	DefaultRole        Role   `json:"default_role,omitempty"`     // user
	TimeoutSec         int    `json:"timeout_sec,omitempty"`      // 10
}

type Settings struct {
	Datasources []Datasource      `json:"datasources"`
	Retention   RetentionSettings `json:"retention"`
	Worker      WorkerSettings    `json:"worker"`
	Notify      NotifySettings    `json:"notify"`
	LDAP        LDAPSettings      `json:"ldap"`
	// PublicURL — tashqaridan ko'rinadigan manzil (share/notification linklari uchun),
	// masalan https://logs.example.com. Bo'sh bo'lsa so'rovdagi Host ishlatiladi.
	PublicURL string `json:"public_url,omitempty"`
}

type SavedQuery struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Query        string `json:"query"`
	DatasourceID string `json:"datasource_id,omitempty"`
	Global       bool   `json:"global"`
	OwnerID      string `json:"owner_id"`
}

type JobStatus string

const (
	JobQueued  JobStatus = "queued"
	JobRunning JobStatus = "running"
	JobDone    JobStatus = "done"
	JobFailed  JobStatus = "failed"
)

// Export tarkibi (content): nima yoziladi.
const (
	ContentNDJSON = "ndjson" // har qator to'liq JSON (default); Fields berilsa faqat shu maydonlar
	ContentMsg    = "msg"    // faqat _msg matni, har qator bitta xabar
	ContentCSV    = "csv"    // Fields ustunlari bilan CSV (har qismda sarlavha qatori)
)

// ExportFile — job natijasidagi bitta fayl (arxiv hajmi bo'yicha bo'laklanganda
// bir nechta bo'ladi: <nom>_vol001.tar.gz, _vol002, ...).
type ExportFile struct {
	Name  string `json:"name"`
	Size  int64  `json:"size"`
	Parts int    `json:"parts"` // shu arxiv ichidagi qismlar soni
}

// ExportJob — `<DataDir>/jobs.json` da saqlanadi (restart'da yo'qolmaydi).
// Running holatida uzilgan job server qayta ishga tushganda failed bo'ladi,
// queued qolgan job qayta navbatga qo'yiladi.
type ExportJob struct {
	ID             string    `json:"id"`
	OwnerID        string    `json:"owner_id"`
	OwnerName      string    `json:"owner_name,omitempty"`
	Query          string    `json:"query"`
	DatasourceID   string    `json:"datasource_id"`
	DatasourceName string    `json:"datasource_name"`
	Status         JobStatus `json:"status"`
	// FileName — birinchi (asosiy) fayl; to'liq ro'yxat Files da.
	FileName  string    `json:"file_name"`
	CreatedAt time.Time `json:"created_at"`
	Error     string    `json:"error,omitempty"`
	Lines     int64     `json:"lines"`
	Start     string    `json:"start,omitempty"`
	End       string    `json:"end,omitempty"`
	SortOrder string    `json:"sort_order,omitempty"`
	// Format: "tar.gz" (default) | "log.gz" | "log". StartExport'da normalizatsiya qilinadi.
	Format string `json:"format,omitempty"`
	// Content: ndjson | msg | csv; Fields — ndjson/csv uchun maydonlar (ixtiyoriy).
	Content string   `json:"content,omitempty"`
	Fields  []string `json:"fields,omitempty"`
	// ArchiveSplitMB — bitta .tar.gz arxivning (siqilgan) maksimal hajmi; 0 = cheksiz.
	ArchiveSplitMB int `json:"archive_split_mb,omitempty"`
	// Size — barcha fayllarning jami hajmi; Parts — jami qismlar soni.
	Size  int64        `json:"size"`
	Parts int          `json:"parts,omitempty"`
	Files []ExportFile `json:"files,omitempty"`
	// Progress: TotalEstimate — VictoriaLogs hits bo'yicha taxminiy qator soni (0 = noma'lum);
	// WindowsTotal/WindowsDone — vaqt oynalari bo'yicha bo'lingan so'rov taraqqiyoti.
	TotalEstimate int64      `json:"total_estimate,omitempty"`
	WindowsTotal  int        `json:"windows_total,omitempty"`
	WindowsDone   int        `json:"windows_done,omitempty"`
	Retries       int        `json:"retries,omitempty"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
	// ScheduleID — rejali export tomonidan yaratilgan bo'lsa.
	ScheduleID string `json:"schedule_id,omitempty"`
	// Notify — tugaganda egasiga xabar; AutoShare — tugaganda share link yaratish (ShareURL).
	Notify    bool   `json:"notify,omitempty"`
	AutoShare bool   `json:"auto_share,omitempty"`
	ShareURL  string `json:"share_url,omitempty"`
}

// ShareLink — export faylga ommaviy URL beradi (auth talab qilmaydi).
// ExpiresAt nil bo'lsa muddati cheksiz, MaxDownloads 0 bo'lsa cheklov yo'q.
// Limit/expiry hodisalaridan keyin fayl + job + ushbu share o'chadi.
type ShareLink struct {
	Token         string     `json:"token"` // URL'dagi token (URL-safe base64)
	JobID         string     `json:"job_id"`
	CreatedBy     string     `json:"created_by"` // user ID
	CreatedAt     time.Time  `json:"created_at"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	MaxDownloads  int        `json:"max_downloads"` // 0 = cheksiz
	DownloadCount int        `json:"download_count"`
}

// Schedule — rejali (cron) export. Har ishga tushganda [now-Lookback, now]
// (Align bo'lsa Lookback ga tekislangan davr) oralig'i export qilinadi.
type Schedule struct {
	ID              string     `json:"id"`
	OwnerID         string     `json:"owner_id"`
	OwnerName       string     `json:"owner_name,omitempty"`
	Name            string     `json:"name"`
	Query           string     `json:"query"`
	DatasourceID    string     `json:"datasource_id"`
	Cron            string     `json:"cron"` // 5 maydon: min hour dom month dow
	LookbackMinutes int        `json:"lookback_minutes"`
	Align           bool       `json:"align"`
	SortOrder       string     `json:"sort_order,omitempty"`
	Format          string     `json:"format,omitempty"`
	Content         string     `json:"content,omitempty"`
	Fields          []string   `json:"fields,omitempty"`
	ArchiveSplitMB  int        `json:"archive_split_mb,omitempty"`
	Notify          bool       `json:"notify"`
	AutoShare       bool       `json:"auto_share"`
	Enabled         bool       `json:"enabled"`
	CreatedAt       time.Time  `json:"created_at"`
	LastRunAt       *time.Time `json:"last_run_at,omitempty"`
	LastJobID       string     `json:"last_job_id,omitempty"`
	LastError       string     `json:"last_error,omitempty"`
	NextRunAt       *time.Time `json:"next_run_at,omitempty"`
}

// AuditEntry — `audit.jsonl` ga bitta qator.
type AuditEntry struct {
	Time    time.Time `json:"time"`
	UserID  string    `json:"user_id,omitempty"`
	User    string    `json:"user,omitempty"`
	Action  string    `json:"action"`
	Target  string    `json:"target,omitempty"`
	IP      string    `json:"ip,omitempty"`
	Details string    `json:"details,omitempty"`
}

// HistoryEntry — foydalanuvchining so'nggi querylari (Log Viewer tarixi).
type HistoryEntry struct {
	Query        string    `json:"query"`
	DatasourceID string    `json:"datasource_id,omitempty"`
	At           time.Time `json:"at"`
}
