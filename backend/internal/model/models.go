package model

import "time"

type Datasource struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	URL      string            `json:"url"`
	Username string            `json:"username,omitempty"`
	Password string            `json:"password,omitempty"`
	Headers  map[string]string `json:"headers,omitempty"`
}

type Settings struct {
	Datasources []Datasource `json:"datasources"`
}

type SavedQuery struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Query        string `json:"query"`
	DatasourceID string `json:"datasource_id,omitempty"`
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
}
