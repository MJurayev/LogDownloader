package worker

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"logdownloader/internal/model"
	"logdownloader/internal/store"
	"logdownloader/internal/vlclient"
)

type Worker struct {
	store *store.Store
}

func New(s *store.Store) *Worker {
	return &Worker{store: s}
}

func (w *Worker) StartExport(query, datasourceID string) string {
	jobID := fmt.Sprintf("job_%d", time.Now().UnixNano())
	fileName := fmt.Sprintf("export_%s.log", jobID)

	dsName := datasourceID
	if ds := w.store.GetDatasource(datasourceID); ds != nil {
		dsName = ds.Name
	}

	job := &model.ExportJob{
		ID:             jobID,
		Query:          query,
		DatasourceID:   datasourceID,
		DatasourceName: dsName,
		Status:         model.JobRunning,
		FileName:       fileName,
		CreatedAt:      time.Now(),
	}

	w.store.AddJob(job)
	go w.runExport(job)

	return jobID
}

func (w *Worker) runExport(job *model.ExportJob) {
	ds := w.store.GetDatasource(job.DatasourceID)
	if ds == nil {
		w.store.UpdateJobStatus(job.ID, model.JobFailed, "Datasource not found", 0)
		return
	}
	if ds.URL == "" {
		w.store.UpdateJobStatus(job.ID, model.JobFailed, "Datasource URL not configured", 0)
		return
	}

	baseURL := strings.TrimRight(ds.URL, "/")
	exportURL := fmt.Sprintf("%s/select/logsql/query", baseURL)

	params := url.Values{}
	params.Set("query", job.Query)

	resp, err := vlclient.Do("GET", exportURL+"?"+params.Encode(), *ds)
	if err != nil {
		w.store.UpdateJobStatus(job.ID, model.JobFailed, fmt.Sprintf("request failed: %v", err), 0)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		w.store.UpdateJobStatus(job.ID, model.JobFailed, fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body)), 0)
		return
	}

	outPath := filepath.Join(w.store.ExportsDir(), job.FileName)
	f, err := os.Create(outPath)
	if err != nil {
		w.store.UpdateJobStatus(job.ID, model.JobFailed, fmt.Sprintf("file create failed: %v", err), 0)
		return
	}
	defer f.Close()

	writer := bufio.NewWriter(f)
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	var lines int64
	for scanner.Scan() {
		writer.WriteString(scanner.Text())
		writer.WriteByte('\n')
		lines++
		if lines%1000 == 0 {
			w.store.UpdateJobStatus(job.ID, model.JobRunning, "", lines)
		}
	}
	writer.Flush()

	if err := scanner.Err(); err != nil {
		w.store.UpdateJobStatus(job.ID, model.JobFailed, fmt.Sprintf("read error: %v", err), lines)
		return
	}

	w.store.UpdateJobStatus(job.ID, model.JobDone, "", lines)
}
