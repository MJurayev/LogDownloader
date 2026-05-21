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

func (w *Worker) StartExport(query, datasourceID, start, end, sortOrder, name string) string {
	jobID := fmt.Sprintf("job_%d", time.Now().UnixNano())
	fileName := sanitizeFileName(name, jobID)

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
		Start:          start,
		End:            end,
		SortOrder:      sortOrder,
	}

	w.store.AddJob(job)
	go w.runExport(job)

	return jobID
}

// sanitizeFileName foydalanuvchidan kelgan nomni tozalaydi va xavfsiz qiladi:
// path separator olib tashlanadi, control chars yo'q qilinadi, bo'sh bo'lsa
// default nom qaytariladi, kerak bo'lsa `.log` qo'shiladi.
func sanitizeFileName(name, jobID string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Sprintf("export_%s.log", jobID)
	}
	// Backslash'larni ham slash bilan birga olib tashlash uchun
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	// Control chars va separatorlarni olib tashlash
	name = strings.Map(func(r rune) rune {
		if r < 32 || r == '/' || r == '\\' || r == 127 {
			return -1
		}
		return r
	}, name)
	if name == "" || name == "." || name == ".." {
		return fmt.Sprintf("export_%s.log", jobID)
	}
	if !strings.HasSuffix(strings.ToLower(name), ".log") {
		name = name + ".log"
	}
	return name
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
	params.Set("query", vlclient.WithTimeSort(job.Query, job.SortOrder))
	if job.Start != "" {
		params.Set("start", job.Start)
	}
	if job.End != "" {
		params.Set("end", job.End)
	}

	resp, err := vlclient.DoLongRunning("GET", exportURL+"?"+params.Encode(), *ds)
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
	reader := bufio.NewReaderSize(resp.Body, 64*1024)

	var lines int64
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			if _, werr := writer.WriteString(line); werr != nil {
				w.store.UpdateJobStatus(job.ID, model.JobFailed, fmt.Sprintf("write error: %v", werr), lines)
				return
			}
			lines++
			if lines%1000 == 0 {
				w.store.UpdateJobStatus(job.ID, model.JobRunning, "", lines)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			writer.Flush()
			w.store.UpdateJobStatus(job.ID, model.JobFailed, fmt.Sprintf("read error: %v", err), lines)
			return
		}
	}
	if err := writer.Flush(); err != nil {
		w.store.UpdateJobStatus(job.ID, model.JobFailed, fmt.Sprintf("flush error: %v", err), lines)
		return
	}

	w.store.UpdateJobStatus(job.ID, model.JobDone, "", lines)
}
