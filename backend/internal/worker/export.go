package worker

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"logdownloader/internal/metrics"
	"logdownloader/internal/model"
	"logdownloader/internal/vlclient"
)

const (
	maxWindowRetries = 3
)

var retryBackoff = []time.Duration{2 * time.Second, 5 * time.Second, 15 * time.Second}

// exportError — retryable (tarmoq/5xx) yoki doimiy (4xx, disk) xato.
type exportError struct {
	msg       string
	retryable bool
}

func (e *exportError) Error() string { return e.msg }

func permanent(format string, a ...any) error {
	return &exportError{msg: fmt.Sprintf(format, a...), retryable: false}
}

func transient(format string, a ...any) error {
	return &exportError{msg: fmt.Sprintf(format, a...), retryable: true}
}

func isRetryable(err error) bool {
	if e, ok := err.(*exportError); ok {
		return e.retryable
	}
	return false
}

func (w *Worker) fail(job *model.ExportJob, lines int64, msg string) {
	w.store.UpdateJobStatus(job.ID, model.JobFailed, msg, lines)
	metrics.ExportsFailed.Add(1)
	job.Status = model.JobFailed
	job.Error = msg
	job.Lines = lines
	if w.notifier != nil {
		go w.notifier.JobFinished(job)
	}
}

func (w *Worker) runExport(ctx context.Context, job *model.ExportJob) {
	outPath := filepath.Join(w.store.ExportsDir(), job.FileName)
	tmpPath := outPath + ".tmp"
	// Job bekor qilingan (o'chirilgan) bo'lsa chala fayllar qolmasin —
	// DeleteJob fayl yaratilishidan oldin kelgan bo'lishi mumkin.
	defer func() {
		os.Remove(tmpPath)
		if ctx.Err() != nil {
			os.Remove(outPath)
			base := strings.TrimSuffix(outPath, ".tar.gz")
			if vols, _ := filepath.Glob(base + "_vol*.tar.gz"); vols != nil {
				for _, v := range vols {
					os.Remove(v)
				}
			}
		}
	}()

	ds := w.store.GetDatasource(job.DatasourceID)
	if ds == nil {
		w.fail(job, 0, "Datasource not found")
		return
	}
	if ds.URL == "" {
		w.fail(job, 0, "Datasource URL not configured")
		return
	}

	// 1) Oynalar va taxminiy jami.
	windows, windowed := planWindows(ctx, *ds, job)
	if windowed {
		w.store.UpdateJob(job.ID, false, func(j *model.ExportJob) { j.WindowsTotal = len(windows) })
	}
	if !vlclient.HasPipes(job.Query) {
		estCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		if total, err := vlclient.CountHits(estCtx, *ds, job.Query, job.Start, job.End, rangeDuration(job)); err == nil && total > 0 {
			w.store.UpdateJob(job.ID, false, func(j *model.ExportJob) { j.TotalEstimate = total })
		}
		cancel()
	}
	if ctx.Err() != nil {
		return
	}

	// 2) Xom oqim → .tmp (tarkib transformatsiyasi + qism chegaralari).
	sink, err := newRawSink(tmpPath, w.chunkBytes, job.Content, job.Fields)
	if err != nil {
		w.fail(job, 0, fmt.Sprintf("temp file failed: %v", err))
		return
	}
	closed := false
	defer func() {
		if !closed {
			sink.close()
		}
	}()

	order := windows
	if job.SortOrder == "desc" && windowed {
		order = make([]window, len(windows))
		for i, win := range windows {
			order[len(windows)-1-i] = win
		}
	}

	for i, win := range order {
		attempt := 0
		for {
			m, merr := sink.mark()
			if merr != nil {
				w.fail(job, sink.lines, fmt.Sprintf("flush error: %v", merr))
				return
			}
			err := w.fetchWindow(ctx, *ds, job, win, sink)
			if err == nil {
				break
			}
			if ctx.Err() != nil {
				return // bekor qilingan — job allaqachon o'chirilgan
			}
			if !isRetryable(err) || attempt >= maxWindowRetries {
				w.fail(job, sink.lines, err.Error())
				return
			}
			if rbErr := sink.rollback(m); rbErr != nil {
				w.fail(job, sink.lines, fmt.Sprintf("rollback failed after %v: %v", err, rbErr))
				return
			}
			attempt++
			metrics.ExportRetries.Add(1)
			w.store.UpdateJob(job.ID, false, func(j *model.ExportJob) { j.Retries++; j.Lines = sink.lines })
			log.Printf("job %s: window %d/%d retry %d after error: %v", job.ID, i+1, len(order), attempt, err)
			backoff := retryBackoff[min(attempt-1, len(retryBackoff)-1)]
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return
			}
		}
		done := i + 1
		w.store.UpdateJob(job.ID, false, func(j *model.ExportJob) {
			if windowed {
				j.WindowsDone = done
			}
			j.Lines = sink.lines
		})
	}

	if err := sink.close(); err != nil {
		w.fail(job, sink.lines, fmt.Sprintf("flush error: %v", err))
		return
	}
	closed = true
	if ctx.Err() != nil {
		return
	}

	// 3) Paketlash.
	files, err := w.packageOutput(job, sink, outPath)
	if err != nil {
		w.fail(job, sink.lines, err.Error())
		return
	}
	if ctx.Err() != nil {
		for _, f := range files {
			os.Remove(filepath.Join(w.store.ExportsDir(), f.Name))
		}
		return
	}

	var size int64
	parts := 0
	for _, f := range files {
		size += f.Size
		parts += f.Parts
	}
	now := time.Now()
	lines := sink.lines
	w.store.UpdateJob(job.ID, true, func(j *model.ExportJob) {
		j.Status = model.JobDone
		j.Error = ""
		j.Lines = lines
		j.Files = files
		j.FileName = files[0].Name
		j.Size = size
		j.Parts = parts
		j.FinishedAt = &now
		if windowed {
			j.WindowsDone = j.WindowsTotal
		}
	})
	metrics.ExportsDone.Add(1)
	metrics.ExportLines.Add(lines)
	metrics.ExportBytes.Add(size)

	// 4) Auto-share va bildirishnoma.
	final := w.store.GetJob(job.ID)
	if final == nil {
		return
	}
	if final.AutoShare {
		if url, err := w.createAutoShare(final); err == nil {
			w.store.UpdateJob(job.ID, true, func(j *model.ExportJob) { j.ShareURL = url })
			final.ShareURL = url
		} else {
			log.Printf("job %s: auto-share failed: %v", job.ID, err)
		}
	}
	if w.notifier != nil {
		go w.notifier.JobFinished(final)
	}
}

// fetchWindow — bitta oyna uchun VictoriaLogs so'rovi va qatorlarni sink'ka oqim.
func (w *Worker) fetchWindow(ctx context.Context, ds model.Datasource, job *model.ExportJob, win window, sink *rawSink) error {
	params := url.Values{}
	params.Set("query", vlclient.WithTimeSort(job.Query, job.SortOrder))
	if win.start != "" {
		params.Set("start", win.start)
	}
	if win.end != "" {
		params.Set("end", win.end)
	}

	resp, err := vlclient.DoLongRunning(ctx, "GET", vlclient.Endpoint(ds, "query", params), ds)
	if err != nil {
		return transient("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		msg := fmt.Sprintf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests {
			return transient("%s", msg)
		}
		return permanent("%s", msg)
	}

	reader := bufio.NewReaderSize(resp.Body, 64*1024)
	var sinceUpdate int64
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			if werr := sink.writeLine([]byte(line)); werr != nil {
				return permanent("write error: %v", werr)
			}
			sinceUpdate++
			if sinceUpdate >= 1000 {
				sinceUpdate = 0
				lines := sink.lines
				w.store.UpdateJob(job.ID, false, func(j *model.ExportJob) { j.Lines = lines })
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			if ctx.Err() != nil {
				return permanent("canceled")
			}
			return transient("read error: %v", err)
		}
	}
}

// createAutoShare — 7 kunlik, cheksiz yuklab olishli share link.
func (w *Worker) createAutoShare(job *model.ExportJob) (string, error) {
	token, err := newShareToken()
	if err != nil {
		return "", err
	}
	exp := time.Now().Add(7 * 24 * time.Hour)
	share := &model.ShareLink{
		Token:     token,
		JobID:     job.ID,
		CreatedBy: job.OwnerID,
		CreatedAt: time.Now(),
		ExpiresAt: &exp,
	}
	if err := w.store.SaveShare(share); err != nil {
		return "", err
	}
	base := strings.TrimRight(w.store.PublicURL(), "/")
	return base + "/dl/" + token, nil
}
