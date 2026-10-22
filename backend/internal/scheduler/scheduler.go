package scheduler

import (
	"fmt"
	"log"
	"sync"
	"time"

	"logdownloader/internal/model"
	"logdownloader/internal/store"
	"logdownloader/internal/worker"
)

// Scheduler — har 30 soniyada schedules.json ni ko'rib, vaqti kelganlarini
// ishga tushiradi. Server o'chiq paytda o'tkazib yuborilgan ishga tushirish
// bitta marta qoplanadi (NextRunAt < now bo'lsa darhol).
type Scheduler struct {
	store  *store.Store
	worker *worker.Worker
	mu     sync.Mutex
}

func New(s *store.Store, w *worker.Worker) *Scheduler {
	return &Scheduler{store: s, worker: w}
}

func (sc *Scheduler) Start() {
	sc.refreshNextRuns()
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			sc.tick()
		}
	}()
}

// refreshNextRuns — NextRunAt yo'q yoki cron o'zgargan schedule'larga keyingi vaqtni hisoblash.
func (sc *Scheduler) refreshNextRuns() {
	now := time.Now()
	for _, s := range sc.store.AllSchedules() {
		if !s.Enabled || s.NextRunAt != nil {
			continue
		}
		c, err := Parse(s.Cron)
		if err != nil {
			continue
		}
		next := c.Next(now)
		sc.store.UpdateSchedule(s.ID, func(x *model.Schedule) { x.NextRunAt = &next })
	}
}

func (sc *Scheduler) tick() {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	now := time.Now()
	for _, s := range sc.store.AllSchedules() {
		if !s.Enabled {
			continue
		}
		if s.NextRunAt == nil {
			sc.refreshNextRuns()
			continue
		}
		if s.NextRunAt.After(now) {
			continue
		}
		sc.run(&s, now, false)
	}
}

// RunNow — "Hozir ishga tushirish" (UI). Keyingi rejaga ta'sir qilmaydi.
func (sc *Scheduler) RunNow(id string) (string, error) {
	s := sc.store.GetSchedule(id)
	if s == nil {
		return "", fmt.Errorf("schedule not found")
	}
	return sc.run(s, time.Now(), true), nil
}

// ComputeNext — handler yaratish/tahrirlashda chaqiradi.
func ComputeNext(cronExpr string, from time.Time) (*time.Time, error) {
	c, err := Parse(cronExpr)
	if err != nil {
		return nil, err
	}
	n := c.Next(from)
	if n.IsZero() {
		return nil, fmt.Errorf("cron: mos vaqt topilmadi")
	}
	return &n, nil
}

// Range — [start, end]: end = now (Align bo'lsa lookback'ka tekislangan),
// start = end - lookback. Masalan daily 00:05 + lookback 24h + align → kechagi 00:00–24:00.
func Range(now time.Time, lookbackMinutes int, align bool) (time.Time, time.Time) {
	if lookbackMinutes <= 0 {
		lookbackMinutes = 60
	}
	lb := time.Duration(lookbackMinutes) * time.Minute
	end := now
	if align {
		end = now.Truncate(lb)
	}
	return end.Add(-lb), end
}

func (sc *Scheduler) run(s *model.Schedule, now time.Time, manual bool) string {
	start, end := Range(now, s.LookbackMinutes, s.Align)
	name := s.Name
	if name == "" {
		name = "schedule"
	}
	jobID, err := sc.worker.StartExport(worker.ExportParams{
		Query:          s.Query,
		DatasourceID:   s.DatasourceID,
		Start:          start.UTC().Format(time.RFC3339),
		End:            end.UTC().Format(time.RFC3339),
		SortOrder:      s.SortOrder,
		Name:           name,
		Format:         s.Format,
		Content:        s.Content,
		Fields:         s.Fields,
		ArchiveSplitMB: s.ArchiveSplitMB,
		Notify:         s.Notify,
		AutoShare:      s.AutoShare,
		OwnerID:        s.OwnerID,
		OwnerName:      s.OwnerName,
		ScheduleID:     s.ID,
	})
	var next *time.Time
	if !manual {
		if c, perr := Parse(s.Cron); perr == nil {
			n := c.Next(now)
			next = &n
		}
	}
	sc.store.UpdateSchedule(s.ID, func(x *model.Schedule) {
		if !manual {
			x.LastRunAt = &now
			x.NextRunAt = next
		}
		if err != nil {
			x.LastError = err.Error()
		} else {
			x.LastError = ""
			x.LastJobID = jobID
		}
	})
	if err != nil {
		log.Printf("schedule %s (%s): start failed: %v", s.ID, s.Name, err)
		return ""
	}
	log.Printf("schedule %s (%s): started job %s [%s — %s]", s.ID, s.Name, jobID, start.Format(time.RFC3339), end.Format(time.RFC3339))
	return jobID
}
