package model

import "time"

type Role string

const (
	RoleAdmin Role = "admin"
	RoleUser  Role = "user"
)

type User struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Password string `json:"password,omitempty"` // bcrypt hash, omit in JSON responses
	Role     Role   `json:"role"`
}

type Datasource struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	URL      string            `json:"url"`
	Username string            `json:"username,omitempty"`
	Password string            `json:"password,omitempty"`
	Headers  map[string]string `json:"headers,omitempty"`
	Global   bool              `json:"global"`
	OwnerID  string            `json:"owner_id"`
}

type Settings struct {
	Datasources []Datasource `json:"datasources"`
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
	JobRunning JobStatus = "running"
	JobDone    JobStatus = "done"
	JobFailed  JobStatus = "failed"
)

type ExportJob struct {
	ID             string    `json:"id"`
	Query          string    `json:"query"`
	DatasourceID   string    `json:"datasource_id"`
	DatasourceName string    `json:"datasource_name"`
	Status         JobStatus `json:"status"`
	FileName       string    `json:"file_name"`
	CreatedAt      time.Time `json:"created_at"`
	Error          string    `json:"error,omitempty"`
	Lines          int64     `json:"lines"`
	Start          string    `json:"start,omitempty"`
	End            string    `json:"end,omitempty"`
	SortOrder      string    `json:"sort_order,omitempty"`
	// Format: "log" (xom NDJSON) | "log.gz" (gzipped) | "tar.gz" (tar+gzip).
	// Bo'sh bo'lsa "log" deb interpretatsiya qilinadi (eski joblar uchun).
	Format string `json:"format,omitempty"`
	// Size — diskdagi yakuniy fayl hajmi (baytlar).
	Size int64 `json:"size"`
}

// ShareLink — export faylga ommaviy URL beradi (auth talab qilmaydi).
// ExpiresAt nil bo'lsa muddati cheksiz, MaxDownloads 0 bo'lsa cheklov yo'q.
// Limit/expiry hodisalaridan keyin fayl + job + ushbu share o'chadi.
type ShareLink struct {
	Token         string     `json:"token"`            // URL'dagi token (URL-safe base64)
	JobID         string     `json:"job_id"`
	CreatedBy     string     `json:"created_by"`       // user ID
	CreatedAt     time.Time  `json:"created_at"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	MaxDownloads  int        `json:"max_downloads"`    // 0 = cheksiz
	DownloadCount int        `json:"download_count"`
}
