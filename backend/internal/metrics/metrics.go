// Package metrics — Prometheus text formatidagi oddiy metrikalar, kutubxonasiz.
package metrics

import (
	"fmt"
	"net/http"
	"runtime"
	"sync/atomic"
	"time"
)

var (
	startedAt = time.Now()

	ExportsStarted atomic.Int64
	ExportsDone    atomic.Int64
	ExportsFailed  atomic.Int64
	ExportLines    atomic.Int64
	ExportBytes    atomic.Int64
	ExportRetries  atomic.Int64

	http2xx atomic.Int64
	http3xx atomic.Int64
	http4xx atomic.Int64
	http5xx atomic.Int64

	NotifySent   atomic.Int64
	NotifyFailed atomic.Int64
	LoginFailed  atomic.Int64
	LoginBlocked atomic.Int64
)

// Snapshot — handler har so'rovda store/worker'dan oladigan joriy qiymatlar.
type Snapshot struct {
	Version        string
	JobsQueued     int
	JobsRunning    int
	JobsDone       int
	JobsFailed     int
	WorkerRunning  int
	WorkerQueued   int
	WorkerMax      int
	ExportsDirByte int64
	ExportsDirFile int
	Schedules      int
	AuditLines     int
}

// Handler — /metrics. snap har chaqiruvda joriy holatni beradi.
func Handler(snap func() Snapshot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s := snap()
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		p := func(format string, a ...any) { fmt.Fprintf(w, format+"\n", a...) }

		p("# HELP logdownloader_build_info Build information.")
		p("# TYPE logdownloader_build_info gauge")
		p("logdownloader_build_info{version=%q,go=%q} 1", s.Version, runtime.Version())
		p("# HELP logdownloader_uptime_seconds Seconds since process start.")
		p("# TYPE logdownloader_uptime_seconds gauge")
		p("logdownloader_uptime_seconds %d", int64(time.Since(startedAt).Seconds()))

		p("# HELP logdownloader_exports_total Export jobs by outcome since start.")
		p("# TYPE logdownloader_exports_total counter")
		p("logdownloader_exports_total{status=\"started\"} %d", ExportsStarted.Load())
		p("logdownloader_exports_total{status=\"done\"} %d", ExportsDone.Load())
		p("logdownloader_exports_total{status=\"failed\"} %d", ExportsFailed.Load())
		p("# HELP logdownloader_export_lines_total Log lines exported since start.")
		p("# TYPE logdownloader_export_lines_total counter")
		p("logdownloader_export_lines_total %d", ExportLines.Load())
		p("# HELP logdownloader_export_bytes_total Bytes written to export files since start.")
		p("# TYPE logdownloader_export_bytes_total counter")
		p("logdownloader_export_bytes_total %d", ExportBytes.Load())
		p("# HELP logdownloader_export_window_retries_total Retried time windows since start.")
		p("# TYPE logdownloader_export_window_retries_total counter")
		p("logdownloader_export_window_retries_total %d", ExportRetries.Load())

		p("# HELP logdownloader_jobs Jobs currently stored, by status.")
		p("# TYPE logdownloader_jobs gauge")
		p("logdownloader_jobs{status=\"queued\"} %d", s.JobsQueued)
		p("logdownloader_jobs{status=\"running\"} %d", s.JobsRunning)
		p("logdownloader_jobs{status=\"done\"} %d", s.JobsDone)
		p("logdownloader_jobs{status=\"failed\"} %d", s.JobsFailed)
		p("# HELP logdownloader_worker_running Exports currently executing.")
		p("# TYPE logdownloader_worker_running gauge")
		p("logdownloader_worker_running %d", s.WorkerRunning)
		p("# HELP logdownloader_worker_queued Exports waiting in the queue.")
		p("# TYPE logdownloader_worker_queued gauge")
		p("logdownloader_worker_queued %d", s.WorkerQueued)
		p("# HELP logdownloader_worker_max_concurrent Configured concurrency limit.")
		p("# TYPE logdownloader_worker_max_concurrent gauge")
		p("logdownloader_worker_max_concurrent %d", s.WorkerMax)

		p("# HELP logdownloader_exports_dir_bytes Size of the exports directory.")
		p("# TYPE logdownloader_exports_dir_bytes gauge")
		p("logdownloader_exports_dir_bytes %d", s.ExportsDirByte)
		p("# HELP logdownloader_exports_dir_files Files in the exports directory.")
		p("# TYPE logdownloader_exports_dir_files gauge")
		p("logdownloader_exports_dir_files %d", s.ExportsDirFile)
		p("# HELP logdownloader_schedules Configured scheduled exports.")
		p("# TYPE logdownloader_schedules gauge")
		p("logdownloader_schedules %d", s.Schedules)
		p("# HELP logdownloader_audit_entries Entries in the current audit log file.")
		p("# TYPE logdownloader_audit_entries gauge")
		p("logdownloader_audit_entries %d", s.AuditLines)

		p("# HELP logdownloader_http_requests_total HTTP responses by status class.")
		p("# TYPE logdownloader_http_requests_total counter")
		p("logdownloader_http_requests_total{class=\"2xx\"} %d", http2xx.Load())
		p("logdownloader_http_requests_total{class=\"3xx\"} %d", http3xx.Load())
		p("logdownloader_http_requests_total{class=\"4xx\"} %d", http4xx.Load())
		p("logdownloader_http_requests_total{class=\"5xx\"} %d", http5xx.Load())
		p("# HELP logdownloader_notifications_total Notifications by outcome.")
		p("# TYPE logdownloader_notifications_total counter")
		p("logdownloader_notifications_total{status=\"sent\"} %d", NotifySent.Load())
		p("logdownloader_notifications_total{status=\"failed\"} %d", NotifyFailed.Load())
		p("# HELP logdownloader_login_failures_total Failed login attempts.")
		p("# TYPE logdownloader_login_failures_total counter")
		p("logdownloader_login_failures_total %d", LoginFailed.Load())
		p("# HELP logdownloader_login_blocked_total Login attempts rejected by rate limiting.")
		p("# TYPE logdownloader_login_blocked_total counter")
		p("logdownloader_login_blocked_total %d", LoginBlocked.Load())

		p("# HELP go_goroutines Number of goroutines.")
		p("# TYPE go_goroutines gauge")
		p("go_goroutines %d", runtime.NumGoroutine())
	}
}

// statusWriter — javob kodini ushlab qoladi; Flusher'ni (tail/SSE) ham uzatadi.
type statusWriter struct {
	http.ResponseWriter
	code int
}

func (s *statusWriter) WriteHeader(code int) {
	s.code = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusWriter) Write(b []byte) (int, error) {
	if s.code == 0 {
		s.code = http.StatusOK
	}
	return s.ResponseWriter.Write(b)
}

func (s *statusWriter) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// HTTPMiddleware — javob kodlari bo'yicha hisoblagich.
func HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		code := sw.code
		if code == 0 {
			code = http.StatusOK
		}
		switch {
		case code >= 500:
			http5xx.Add(1)
		case code >= 400:
			http4xx.Add(1)
		case code >= 300:
			http3xx.Add(1)
		default:
			http2xx.Add(1)
		}
	})
}
