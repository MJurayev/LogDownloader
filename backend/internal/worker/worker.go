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
	createdAt := time.Now()
	jobID := fmt.Sprintf("job_%d", createdAt.UnixNano())
	fileName := sanitizeFileName(name, createdAt)

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
		CreatedAt:      createdAt,
		Start:          start,
		End:            end,
		SortOrder:      sortOrder,
	}

	w.store.AddJob(job)
	go w.runExport(job)

	return jobID
}

// sanitizeFileName fayl nomini xavfsiz qiladi va doim oxiriga UTC timestamp
// (`YYYYMMDD-HHMMSS-mmm`) qo'shadi — diskdagi fayl nomlari unique bo'lishi va
// foydalanuvchi qachon export qilganini ko'rishi uchun.
//
// Misol: "my-prod-logs"  -> "my-prod-logs_20260521-090655-123.log"
//        ""              -> "export_20260521-090655-123.log"
//        "../../etc/passwd" -> "passwd_20260521-090655-123.log"
func sanitizeFileName(name string, createdAt time.Time) string {
	ts := createdAt.UTC().Format("20060102-150405")
	ms := createdAt.Nanosecond() / 1e6
	timestamp := fmt.Sprintf("%s-%03d", ts, ms)

	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Sprintf("export_%s.log", timestamp)
	}
	// Path traversal himoyasi
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	// Control chars va separatorlar
	name = strings.Map(func(r rune) rune {
		if r < 32 || r == '/' || r == '\\' || r == 127 {
			return -1
		}
		return r
	}, name)
	if name == "" || name == "." || name == ".." {
		return fmt.Sprintf("export_%s.log", timestamp)
	}
	// Foydalanuvchi `.log` yozgan bo'lsa kesib tashlaymiz — aks holda
	// `my-logs.log_20260521-090655-123.log` chiqib qoladi.
	base := strings.TrimSuffix(name, ".log")
	base = strings.TrimSuffix(base, ".LOG")
	if base == "" {
		return fmt.Sprintf("export_%s.log", timestamp)
	}
	return fmt.Sprintf("%s_%s.log", base, timestamp)
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
