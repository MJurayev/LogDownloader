package worker

import (
	"context"
	"time"

	"logdownloader/internal/model"
	"logdownloader/internal/vlclient"
)

// window — bitta VictoriaLogs so'rovining start/end qiymatlari (bo'sh =
// parametr yuborilmaydi). Windowed rejimda RFC3339Nano UTC.
type window struct {
	start, end string
}

const maxWindows = 2000

// windowStep — oraliq uzunligiga qarab oyna hajmi ("Avtomatik").
func windowStep(d time.Duration) time.Duration {
	switch {
	case d <= 2*time.Hour:
		return 0 // bitta so'rov
	case d <= 48*time.Hour:
		return time.Hour
	case d <= 14*24*time.Hour:
		return 6 * time.Hour
	default:
		return 24 * time.Hour
	}
}

// planWindows — job oralig'ini vaqt oynalariga bo'ladi. Oraliq aniqlanmasa
// (parse xatosi, pipe'li query'da ochiq boshlanish) bitta oyna, asl
// start/end satrlari bilan. Qaytaradi: oynalar (vaqt o'sish tartibida) va
// windowed flag.
func planWindows(ctx context.Context, ds model.Datasource, job *model.ExportJob) ([]window, bool) {
	single := []window{{start: job.Start, end: job.End}}

	var start, end time.Time
	var err error
	if job.End != "" {
		if end, err = time.Parse(time.RFC3339Nano, job.End); err != nil {
			return single, false
		}
	} else {
		end = time.Now()
	}
	if job.Start != "" {
		if start, err = time.Parse(time.RFC3339Nano, job.Start); err != nil {
			return single, false
		}
	} else {
		// Ochiq boshlanish: eng eski logni topamiz (pipe'siz query'da).
		probeCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		earliest, perr := vlclient.EarliestTime(probeCtx, ds, job.Query, job.End)
		if perr != nil {
			return single, false
		}
		start = earliest
	}
	if !end.After(start) {
		return single, false
	}

	step := windowStep(end.Sub(start))
	if step == 0 {
		return single, false
	}
	for end.Sub(start)/step > maxWindows {
		step *= 2
	}

	var out []window
	for cur := start; cur.Before(end); cur = cur.Add(step) {
		next := cur.Add(step)
		wEnd := end
		if next.Before(end) {
			wEnd = next.Add(-time.Nanosecond) // VictoriaLogs end inklyuziv — dublikat bo'lmasin
		}
		out = append(out, window{
			start: cur.UTC().Format(time.RFC3339Nano),
			end:   wEnd.UTC().Format(time.RFC3339Nano),
		})
	}
	if len(out) <= 1 {
		return single, false
	}
	return out, true
}

// rangeDuration — progress taxmini uchun oraliq uzunligi (0 = noma'lum).
func rangeDuration(job *model.ExportJob) time.Duration {
	if job.Start == "" {
		return 0
	}
	start, err := time.Parse(time.RFC3339Nano, job.Start)
	if err != nil {
		return 0
	}
	end := time.Now()
	if job.End != "" {
		if e, err := time.Parse(time.RFC3339Nano, job.End); err == nil {
			end = e
		}
	}
	if !end.After(start) {
		return 0
	}
	return end.Sub(start)
}
