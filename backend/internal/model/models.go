package model

import "time"

type Settings struct {
	VLSelectURL string `json:"vlselect_url"`
}

type SavedQuery struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Query string `json:"query"`
}

type JobStatus string

const (
	JobRunning JobStatus = "running"
	JobDone    JobStatus = "done"
	JobFailed  JobStatus = "failed"
)

type ExportJob struct {
	ID        string    `json:"id"`
	Query     string    `json:"query"`
	Status    JobStatus `json:"status"`
	FileName  string    `json:"file_name"`
	CreatedAt time.Time `json:"created_at"`
	Error     string    `json:"error,omitempty"`
	Lines     int64     `json:"lines"`
}
