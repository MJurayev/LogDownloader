package worker

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"logdownloader/internal/model"
	"logdownloader/internal/store"
	"logdownloader/internal/vlclient"
)

// Supported export formats. Noma'lum yoki bo'sh qiymat FormatTarGz'ga
// normalizatsiya qilinadi (UI'da ham format qulflangan: faqat .tar.gz).
const (
	FormatLog   = "log"
	FormatLogGz = "log.gz"
	FormatTarGz = "tar.gz"
)

// ExportParams — StartExport kirish parametrlari.
type ExportParams struct {
	Query        string
	DatasourceID string
	Start        string
	End          string
	SortOrder    string
	Name         string
	Format       string
	OwnerID      string
	OwnerName    string
}

type Worker struct {
	store   *store.Store
	mu      sync.Mutex
	cancels map[string]context.CancelFunc // jobID -> running export'ni to'xtatish
}

func New(s *store.Store) *Worker {
	return &Worker{store: s, cancels: make(map[string]context.CancelFunc)}
}

func (w *Worker) StartExport(p ExportParams) string {
	createdAt := time.Now()
	jobID := fmt.Sprintf("job_%d", createdAt.UnixNano())
	format := normalizeFormat(p.Format)
	fileName := sanitizeFileName(p.Name, createdAt, format)

	dsName := p.DatasourceID
	if ds := w.store.GetDatasource(p.DatasourceID); ds != nil {
		dsName = ds.Name
	}

	job := &model.ExportJob{
		ID:             jobID,
		OwnerID:        p.OwnerID,
		OwnerName:      p.OwnerName,
		Query:          p.Query,
		DatasourceID:   p.DatasourceID,
		DatasourceName: dsName,
		Status:         model.JobRunning,
		FileName:       fileName,
		CreatedAt:      createdAt,
		Start:          p.Start,
		End:            p.End,
		SortOrder:      p.SortOrder,
		Format:         format,
	}

	ctx, cancel := context.WithCancel(context.Background())
	w.mu.Lock()
	w.cancels[jobID] = cancel
	w.mu.Unlock()

	if err := w.store.AddJob(job); err != nil {
		log.Printf("job %s: persist failed: %v", jobID, err)
	}
	go w.runExport(ctx, job)

	return jobID
}

// Cancel running export'ni to'xtatadi (job o'chirilganda chaqiriladi).
// Job tugagan yoki noma'lum bo'lsa — no-op.
func (w *Worker) Cancel(jobID string) {
	w.mu.Lock()
	cancel, ok := w.cancels[jobID]
	delete(w.cancels, jobID)
	w.mu.Unlock()
	if ok {
		cancel()
	}
}

func normalizeFormat(f string) string {
	switch f {
	case FormatLog, FormatLogGz, FormatTarGz:
		return f
	default:
		return FormatTarGz
	}
}

func formatSuffix(format string) string {
	switch format {
	case FormatLogGz:
		return ".log.gz"
	case FormatTarGz:
		return ".tar.gz"
	default:
		return ".log"
	}
}

// sanitizeFileName fayl nomini xavfsiz qiladi va doim oxiriga UTC timestamp +
// format suffix qo'shadi. Foydalanuvchi yozgan har qanday `.log`/`.log.gz`/
// `.tar.gz` suffix kesiladi (duplikatlar oldini olish uchun).
func sanitizeFileName(name string, createdAt time.Time, format string) string {
	ts := createdAt.UTC().Format("20060102-150405")
	ms := createdAt.Nanosecond() / 1e6
	timestamp := fmt.Sprintf("%s-%03d", ts, ms)
	suffix := formatSuffix(format)

	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Sprintf("export_%s%s", timestamp, suffix)
	}
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if r < 32 || r == '/' || r == '\\' || r == 127 {
			return -1
		}
		return r
	}, name)
	if name == "" || name == "." || name == ".." {
		return fmt.Sprintf("export_%s%s", timestamp, suffix)
	}
	// Ma'lum suffix'larni kesib tashlash (eng uzunidan boshlab — ".log.gz" ".log"'dan oldin)
	lower := strings.ToLower(name)
	for _, ext := range []string{".tar.gz", ".log.gz", ".log"} {
		if strings.HasSuffix(lower, ext) {
			name = name[:len(name)-len(ext)]
			break
		}
	}
	if name == "" {
		return fmt.Sprintf("export_%s%s", timestamp, suffix)
	}
	return fmt.Sprintf("%s_%s%s", name, timestamp, suffix)
}

type countingWriter struct {
	w io.Writer
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

func (w *Worker) runExport(ctx context.Context, job *model.ExportJob) {
	// Tugaganda cancel map'dan chiqadi va context resurslari bo'shaydi.
	defer w.Cancel(job.ID)

	outPath := filepath.Join(w.store.ExportsDir(), job.FileName)
	// Job bekor qilingan (o'chirilgan) bo'lsa chala fayl yoki .tmp qolmasin —
	// DeleteJob fayl yaratilishidan oldin kelgan bo'lishi mumkin.
	defer func() {
		if ctx.Err() != nil {
			os.Remove(outPath)
			os.Remove(outPath + ".tmp")
		}
	}()

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

	resp, err := vlclient.DoLongRunning(ctx, "GET", exportURL+"?"+params.Encode(), *ds)
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

	if ctx.Err() != nil {
		return
	}

	switch job.Format {
	case FormatLogGz:
		w.writeLogGz(job, resp.Body, outPath)
	case FormatTarGz:
		w.writeTarGz(job, resp.Body, outPath)
	default:
		w.writeRaw(job, resp.Body, outPath)
	}
}

// writeRaw — xom NDJSON faylga oqim qiladi.
func (w *Worker) writeRaw(job *model.ExportJob, body io.Reader, outPath string) {
	f, err := os.Create(outPath)
	if err != nil {
		w.store.UpdateJobStatus(job.ID, model.JobFailed, fmt.Sprintf("file create failed: %v", err), 0)
		return
	}
	defer f.Close()

	counter := &countingWriter{w: f}
	writer := bufio.NewWriter(counter)
	lines := w.streamLines(job, body, writer)
	if lines < 0 {
		return // streamLines already updated the job to failed
	}
	if err := writer.Flush(); err != nil {
		w.store.UpdateJobStatus(job.ID, model.JobFailed, fmt.Sprintf("flush error: %v", err), lines)
		return
	}
	w.store.UpdateJobSize(job.ID, counter.n)
	w.store.UpdateJobStatus(job.ID, model.JobDone, "", lines)
}

// writeLogGz — gzip qilingan NDJSON faylga oqim qiladi.
func (w *Worker) writeLogGz(job *model.ExportJob, body io.Reader, outPath string) {
	f, err := os.Create(outPath)
	if err != nil {
		w.store.UpdateJobStatus(job.ID, model.JobFailed, fmt.Sprintf("file create failed: %v", err), 0)
		return
	}
	defer f.Close()

	counter := &countingWriter{w: f}
	bufW := bufio.NewWriter(counter)
	gz := gzip.NewWriter(bufW)

	lines := w.streamLines(job, body, gz)
	if lines < 0 {
		return
	}
	if err := gz.Close(); err != nil {
		w.store.UpdateJobStatus(job.ID, model.JobFailed, fmt.Sprintf("gzip close failed: %v", err), lines)
		return
	}
	if err := bufW.Flush(); err != nil {
		w.store.UpdateJobStatus(job.ID, model.JobFailed, fmt.Sprintf("flush error: %v", err), lines)
		return
	}
	w.store.UpdateJobSize(job.ID, counter.n)
	w.store.UpdateJobStatus(job.ID, model.JobDone, "", lines)
}

// writeTarGz — ikki fazada: oldin xom NDJSON ni .tmp faylga oqim qilamiz (tar
// header'i fayl o'lchamini bilishi kerak), keyin .tar.gz quramiz va .tmp ni
// o'chiramiz.
func (w *Worker) writeTarGz(job *model.ExportJob, body io.Reader, outPath string) {
	tempPath := outPath + ".tmp"
	tempF, err := os.Create(tempPath)
	if err != nil {
		w.store.UpdateJobStatus(job.ID, model.JobFailed, fmt.Sprintf("temp file failed: %v", err), 0)
		return
	}
	defer os.Remove(tempPath)

	rawCounter := &countingWriter{w: tempF}
	bufW := bufio.NewWriter(rawCounter)
	lines := w.streamLines(job, body, bufW)
	if lines < 0 {
		tempF.Close()
		return
	}
	if err := bufW.Flush(); err != nil {
		tempF.Close()
		w.store.UpdateJobStatus(job.ID, model.JobFailed, fmt.Sprintf("temp flush failed: %v", err), lines)
		return
	}
	tempF.Close()

	// Endi yakuniy .tar.gz quramiz
	finalF, err := os.Create(outPath)
	if err != nil {
		w.store.UpdateJobStatus(job.ID, model.JobFailed, fmt.Sprintf("file create failed: %v", err), lines)
		return
	}
	defer finalF.Close()

	finalCounter := &countingWriter{w: finalF}
	gz := gzip.NewWriter(finalCounter)
	tw := tar.NewWriter(gz)

	innerName := strings.TrimSuffix(filepath.Base(outPath), ".tar.gz") + ".log"
	if err := tw.WriteHeader(&tar.Header{
		Name:    innerName,
		Mode:    0644,
		Size:    rawCounter.n,
		ModTime: time.Now(),
	}); err != nil {
		w.store.UpdateJobStatus(job.ID, model.JobFailed, fmt.Sprintf("tar header failed: %v", err), lines)
		return
	}

	src, err := os.Open(tempPath)
	if err != nil {
		w.store.UpdateJobStatus(job.ID, model.JobFailed, fmt.Sprintf("tar open temp failed: %v", err), lines)
		return
	}
	if _, err := io.Copy(tw, src); err != nil {
		src.Close()
		w.store.UpdateJobStatus(job.ID, model.JobFailed, fmt.Sprintf("tar copy failed: %v", err), lines)
		return
	}
	src.Close()

	if err := tw.Close(); err != nil {
		w.store.UpdateJobStatus(job.ID, model.JobFailed, fmt.Sprintf("tar close failed: %v", err), lines)
		return
	}
	if err := gz.Close(); err != nil {
		w.store.UpdateJobStatus(job.ID, model.JobFailed, fmt.Sprintf("gzip close failed: %v", err), lines)
		return
	}

	w.store.UpdateJobSize(job.ID, finalCounter.n)
	w.store.UpdateJobStatus(job.ID, model.JobDone, "", lines)
}

// streamLines VictoriaLogs response'idan qator-bo'yicha o'qib writer'ga yozadi.
// Muvaffaqiyatli tugaganda lines qaytaradi. Xato bo'lsa job'ni failed deb belgilab
// -1 qaytaradi (caller buni ko'rib qaytishi kerak). Context bekor qilinsa
// o'qish xato bilan uziladi; job allaqachon o'chirilgan bo'lsa UpdateJobStatus no-op.
func (w *Worker) streamLines(job *model.ExportJob, body io.Reader, writer io.Writer) int64 {
	reader := bufio.NewReaderSize(body, 64*1024)
	var lines int64
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			if _, werr := writer.Write([]byte(line)); werr != nil {
				w.store.UpdateJobStatus(job.ID, model.JobFailed, fmt.Sprintf("write error: %v", werr), lines)
				return -1
			}
			lines++
			if lines%1000 == 0 {
				w.store.UpdateJobStatus(job.ID, model.JobRunning, "", lines)
			}
		}
		if err == io.EOF {
			return lines
		}
		if err != nil {
			w.store.UpdateJobStatus(job.ID, model.JobFailed, fmt.Sprintf("read error: %v", err), lines)
			return -1
		}
	}
}
