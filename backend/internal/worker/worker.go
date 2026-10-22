// Package worker — background export dvigateli.
//
// Oqim: StartExport → job (queued) → dispatcher (MaxConcurrent ta parallel) →
// runExport: vaqt oynalari bo'yicha VictoriaLogs'dan oqim → xom .tmp (tarkib
// transformatsiyasi + qism chegaralari) → paketlash (tar.gz qismlar/volumelar,
// log.gz, log) → done → auto-share → bildirishnoma.
package worker

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"logdownloader/internal/metrics"
	"logdownloader/internal/model"
	"logdownloader/internal/store"
)

// Supported export formats. Noma'lum yoki bo'sh qiymat FormatTarGz'ga
// normalizatsiya qilinadi (UI default'i ham .tar.gz).
const (
	FormatLog   = "log"
	FormatLogGz = "log.gz"
	FormatTarGz = "tar.gz"
)

// ExportParams — StartExport kirish parametrlari.
type ExportParams struct {
	Query          string
	DatasourceID   string
	Start          string
	End            string
	SortOrder      string
	Name           string
	Format         string
	Content        string
	Fields         []string
	ArchiveSplitMB int
	Notify         bool
	AutoShare      bool
	OwnerID        string
	OwnerName      string
	ScheduleID     string
}

// Notifier — job tugaganda chaqiriladi (notify paketi).
type Notifier interface {
	JobFinished(job *model.ExportJob)
}

type Worker struct {
	store *store.Store
	// chunkBytes — avtomatik bo'laklash: .tar.gz ichidagi bitta qismning
	// maksimal xom hajmi. 0 = bo'laklash yo'q. Config `export_chunk_mb`.
	chunkBytes int64
	notifier   Notifier

	mu      sync.Mutex
	cancels map[string]context.CancelFunc // running jobID -> cancel
	queue   []string                      // queued jobID'lar tartibda
	queued  map[string]bool
	running int
	wake    chan struct{}
}

func New(s *store.Store, chunkBytes int64, n Notifier) *Worker {
	if chunkBytes < 0 {
		chunkBytes = 0
	}
	return &Worker{
		store:      s,
		chunkBytes: chunkBytes,
		notifier:   n,
		cancels:    make(map[string]context.CancelFunc),
		queued:     make(map[string]bool),
		wake:       make(chan struct{}, 1),
	}
}

// Start — restart'dan qolgan queued joblarni navbatga oladi va dispatcher'ni ishga tushiradi.
func (w *Worker) Start() {
	for _, id := range w.store.QueuedJobIDs() {
		w.enqueue(id)
	}
	go w.dispatch()
}

func (w *Worker) enqueue(id string) {
	w.mu.Lock()
	if !w.queued[id] {
		w.queued[id] = true
		w.queue = append(w.queue, id)
	}
	w.mu.Unlock()
	w.signal()
}

func (w *Worker) signal() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

// Stats — metrics uchun.
func (w *Worker) Stats() (running, queued int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.running, len(w.queue)
}

// Signal — sozlamalar (MaxConcurrent) o'zgarganda dispatcher'ni uyg'otish.
func (w *Worker) Signal() { w.signal() }

func (w *Worker) dispatch() {
	for {
		w.mu.Lock()
		max := w.store.MaxConcurrent()
		for w.running < max && len(w.queue) > 0 {
			id := w.queue[0]
			w.queue = w.queue[1:]
			delete(w.queued, id)
			job := w.store.GetJob(id)
			if job == nil || job.Status != model.JobQueued {
				continue
			}
			ctx, cancel := context.WithCancel(context.Background())
			w.cancels[id] = cancel
			w.running++
			now := time.Now()
			w.store.UpdateJob(id, true, func(j *model.ExportJob) {
				j.Status = model.JobRunning
				j.StartedAt = &now
			})
			job.Status = model.JobRunning
			metrics.ExportsStarted.Add(1)
			go func(id string, job *model.ExportJob, ctx context.Context, cancel context.CancelFunc) {
				defer func() {
					w.mu.Lock()
					w.running--
					delete(w.cancels, id)
					w.mu.Unlock()
					cancel()
					w.signal()
				}()
				w.runExport(ctx, job)
			}(id, job, ctx, cancel)
		}
		w.mu.Unlock()
		select {
		case <-w.wake:
		case <-time.After(5 * time.Second):
		}
	}
}

// StartExport — job yaratadi (queued) va navbatga qo'yadi. Datasource
// topilmasa xato.
func (w *Worker) StartExport(p ExportParams) (string, error) {
	ds := w.store.GetDatasource(p.DatasourceID)
	if ds == nil {
		return "", fmt.Errorf("datasource not found")
	}
	if strings.TrimSpace(p.Query) == "" {
		return "", fmt.Errorf("query is required")
	}
	createdAt := time.Now()
	jobID := fmt.Sprintf("job_%d", createdAt.UnixNano())
	format := normalizeFormat(p.Format)
	content := normalizeContent(p.Content)
	fields := normalizeFields(content, p.Fields)
	fileName := sanitizeFileName(p.Name, createdAt, fileSuffix(format, content))

	job := &model.ExportJob{
		ID:             jobID,
		OwnerID:        p.OwnerID,
		OwnerName:      p.OwnerName,
		Query:          p.Query,
		DatasourceID:   p.DatasourceID,
		DatasourceName: ds.Name,
		Status:         model.JobQueued,
		FileName:       fileName,
		CreatedAt:      createdAt,
		Start:          p.Start,
		End:            p.End,
		SortOrder:      p.SortOrder,
		Format:         format,
		Content:        content,
		Fields:         fields,
		ArchiveSplitMB: p.ArchiveSplitMB,
		Notify:         p.Notify,
		AutoShare:      p.AutoShare,
		ScheduleID:     p.ScheduleID,
	}
	if job.ArchiveSplitMB < 0 || format != FormatTarGz {
		job.ArchiveSplitMB = 0
	}
	if err := w.store.AddJob(job); err != nil {
		return "", fmt.Errorf("persist job: %w", err)
	}
	w.enqueue(jobID)
	return jobID, nil
}

// Cancel — navbatdagi jobni navbatdan oladi, running jobning context'ini
// bekor qiladi (job o'chirilganda chaqiriladi). Noma'lum/tugagan job — no-op.
func (w *Worker) Cancel(jobID string) {
	w.mu.Lock()
	if w.queued[jobID] {
		delete(w.queued, jobID)
		for i, id := range w.queue {
			if id == jobID {
				w.queue = append(w.queue[:i], w.queue[i+1:]...)
				break
			}
		}
	}
	cancel, ok := w.cancels[jobID]
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

func normalizeContent(c string) string {
	switch c {
	case model.ContentMsg, model.ContentCSV, model.ContentNDJSON:
		return c
	default:
		return model.ContentNDJSON
	}
}

// normalizeFields — bo'sh/bo'shliqli nomlar tashlanadi; CSV uchun bo'sh
// ro'yxat default ustunlarga aylanadi; msg uchun maydonlar ahamiyatsiz.
func normalizeFields(content string, fields []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f == "" || seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
	}
	switch content {
	case model.ContentCSV:
		if len(out) == 0 {
			out = []string{"_time", "_stream", "_msg"}
		}
	case model.ContentMsg:
		out = nil
	}
	return out
}

func fileSuffix(format, content string) string {
	ext := ".log"
	if content == model.ContentCSV {
		ext = ".csv"
	}
	switch format {
	case FormatLogGz:
		return ext + ".gz"
	case FormatTarGz:
		return ".tar.gz"
	default:
		return ext
	}
}

// sanitizeFileName fayl nomini xavfsiz qiladi va doim oxiriga UTC timestamp +
// suffix qo'shadi. Foydalanuvchi yozgan ma'lum suffix'lar kesiladi.
func sanitizeFileName(name string, createdAt time.Time, suffix string) string {
	ts := createdAt.UTC().Format("20060102-150405")
	ms := createdAt.Nanosecond() / 1e6
	timestamp := fmt.Sprintf("%s-%03d", ts, ms)

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
	lower := strings.ToLower(name)
	for _, ext := range []string{".tar.gz", ".log.gz", ".csv.gz", ".log", ".csv"} {
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
