package store

import (
	"os"
	"path/filepath"
	"sort"
	"time"

	"logdownloader/internal/model"
)

// Jobs — `jobs.json` da saqlanadi. Progress (qatorlar, oynalar) faqat xotirada,
// diskka faqat qo'shish/o'chirish/holat o'zgarishi (queued→running→done/failed) yoziladi.

func (s *Store) saveJobsLocked() error {
	return s.saveFile("jobs.json", s.jobs)
}

// jobFileNames — jobga tegishli barcha disk fayllari (Files + FileName + .tmp).
func jobFileNames(job *model.ExportJob) []string {
	seen := map[string]bool{}
	var names []string
	add := func(n string) {
		if n != "" && !seen[n] {
			seen[n] = true
			names = append(names, n)
		}
	}
	add(job.FileName)
	for _, f := range job.Files {
		add(f.Name)
	}
	add(job.FileName + ".tmp")
	return names
}

func (s *Store) removeJobFilesLocked(job *model.ExportJob) {
	for _, n := range jobFileNames(job) {
		os.Remove(filepath.Join(s.ExportsDir(), n))
	}
}

// RecoverJobs server ishga tushganda chaqiriladi:
//   - running holatida qolgan joblar (uzilgan export) failed deb belgilanadi, chala fayl o'chadi;
//   - queued joblar qoladi (worker ularni qayta navbatga oladi — QueuedJobIDs);
//   - hech qaysi jobga tegishli bo'lmagan `exports/` fayllar (orphan) o'chadi;
//   - mavjud bo'lmagan yoki done bo'lmagan jobga ishora qiluvchi share'lar o'chadi.
//
// Qaytaradi: uzilgan joblar soni, o'chirilgan orphan fayllar soni.
func (s *Store) RecoverJobs() (interrupted, orphans int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	known := make(map[string]bool, len(s.jobs))
	jobsChanged := false
	for _, job := range s.jobs {
		switch job.Status {
		case model.JobRunning:
			job.Status = model.JobFailed
			job.Error = "server qayta ishga tushdi, export uzildi"
			now := time.Now()
			job.FinishedAt = &now
			s.removeJobFilesLocked(job)
			interrupted++
			jobsChanged = true
			continue
		case model.JobQueued:
			continue
		}
		for _, n := range jobFileNames(job) {
			known[n] = true
		}
	}

	if entries, err := os.ReadDir(s.ExportsDir()); err == nil {
		for _, e := range entries {
			if e.IsDir() || known[e.Name()] {
				continue
			}
			if err := os.Remove(filepath.Join(s.ExportsDir(), e.Name())); err == nil {
				orphans++
			}
		}
	}

	sharesChanged := false
	for token, sh := range s.shares {
		if job, ok := s.jobs[sh.JobID]; !ok || job.Status != model.JobDone {
			delete(s.shares, token)
			sharesChanged = true
		}
	}

	if jobsChanged {
		s.saveJobsLocked()
	}
	if sharesChanged {
		s.saveFile("shares.json", s.shares)
	}
	return interrupted, orphans
}

// QueuedJobIDs — queued holatidagi joblar, CreatedAt bo'yicha (restart'dan keyin
// worker navbatini tiklash uchun).
func (s *Store) QueuedJobIDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var list []*model.ExportJob
	for _, j := range s.jobs {
		if j.Status == model.JobQueued {
			list = append(list, j)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].CreatedAt.Before(list[j].CreatedAt) })
	ids := make([]string, 0, len(list))
	for _, j := range list {
		ids = append(ids, j.ID)
	}
	return ids
}

func (s *Store) AddJob(job *model.ExportJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := *job
	s.jobs[job.ID] = &c
	return s.saveJobsLocked()
}

// GetJob nusxa qaytaradi — worker bir vaqtda Status/Lines'ni yangilayotgan
// bo'lishi mumkin.
func (s *Store) GetJob(id string) *model.ExportJob {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if job, ok := s.jobs[id]; ok {
		c := *job
		return &c
	}
	return nil
}

// GetJobsForUser — all=true bo'lsa (admin) barcha joblar, aks holda faqat
// ownerID'ga tegishli. Eng yangi joblar tepada (CreatedAt DESC).
func (s *Store) GetJobsForUser(ownerID string, all bool) []model.ExportJob {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]model.ExportJob, 0, len(s.jobs))
	for _, j := range s.jobs {
		if all || j.OwnerID == ownerID {
			result = append(result, *j)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result
}

// JobCounts — status bo'yicha joblar soni (metrics).
func (s *Store) JobCounts() map[model.JobStatus]int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m := map[model.JobStatus]int{model.JobQueued: 0, model.JobRunning: 0, model.JobDone: 0, model.JobFailed: 0}
	for _, j := range s.jobs {
		m[j.Status]++
	}
	return m
}

// UpdateJob — jobni mutex ostida o'zgartiradi. persist=true bo'lsa jobs.json
// ga yoziladi (holat o'zgarishi, yakun); progress yangilanishlari uchun false.
// Job mavjud bo'lmasa (o'chirilgan) hech narsa qilmaydi, false qaytaradi.
func (s *Store) UpdateJob(id string, persist bool, fn func(j *model.ExportJob)) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	if !ok {
		return false
	}
	fn(job)
	if persist {
		s.saveJobsLocked()
	}
	return true
}

func (s *Store) UpdateJobStatus(id string, status model.JobStatus, errMsg string, lines int64) {
	now := time.Now()
	s.UpdateJob(id, status != model.JobRunning, func(j *model.ExportJob) {
		j.Status = status
		j.Error = errMsg
		j.Lines = lines
		if status == model.JobDone || status == model.JobFailed {
			j.FinishedAt = &now
		}
	})
}

func (s *Store) DeleteJob(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	if !ok {
		return
	}
	s.removeJobFilesLocked(job)
	delete(s.jobs, id)
	s.saveJobsLocked()
	sharesChanged := false
	for token, sh := range s.shares {
		if sh.JobID == id {
			delete(s.shares, token)
			sharesChanged = true
		}
	}
	if sharesChanged {
		s.saveFile("shares.json", s.shares)
	}
}

// ExportsUsage — exports/ dagi fayllar jami hajmi va soni (diskdan).
func (s *Store) ExportsUsage() (bytes int64, files int) {
	entries, err := os.ReadDir(s.ExportsDir())
	if err != nil {
		return 0, 0
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if info, err := e.Info(); err == nil {
			bytes += info.Size()
			files++
		}
	}
	return bytes, files
}

// PruneExports — retention siyosati: (1) done/failed joblar `days` kundan eski
// bo'lsa o'chadi; (2) exports/ hajmi quotaBytes dan oshsa eng eski done/failed
// joblar o'chadi (running/queued tegilmaydi). 0 = tegishli qoida o'chiq.
// Qaytaradi: o'chirilgan joblar soni.
func (s *Store) PruneExports(days int, quotaBytes int64) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	removed := 0
	deleteLocked := func(job *model.ExportJob) {
		s.removeJobFilesLocked(job)
		delete(s.jobs, job.ID)
		for token, sh := range s.shares {
			if sh.JobID == job.ID {
				delete(s.shares, token)
			}
		}
		removed++
	}
	finished := func(j *model.ExportJob) bool {
		return j.Status == model.JobDone || j.Status == model.JobFailed
	}

	if days > 0 {
		cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
		for _, job := range s.jobs {
			if finished(job) && job.CreatedAt.Before(cutoff) {
				deleteLocked(job)
			}
		}
	}
	if quotaBytes > 0 {
		total := int64(0)
		for _, job := range s.jobs {
			total += job.Size
		}
		if total > quotaBytes {
			var candidates []*model.ExportJob
			for _, job := range s.jobs {
				if finished(job) {
					candidates = append(candidates, job)
				}
			}
			sort.Slice(candidates, func(i, j int) bool { return candidates[i].CreatedAt.Before(candidates[j].CreatedAt) })
			for _, job := range candidates {
				if total <= quotaBytes {
					break
				}
				total -= job.Size
				deleteLocked(job)
			}
		}
	}
	if removed > 0 {
		s.saveJobsLocked()
		s.saveFile("shares.json", s.shares)
	}
	return removed
}
