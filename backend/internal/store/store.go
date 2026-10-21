package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"logdownloader/internal/model"
)

type Store struct {
	mu       sync.RWMutex
	dataDir  string
	settings model.Settings
	users    map[string]model.User
	queries  map[string]model.SavedQuery
	jobs     map[string]*model.ExportJob
	shares   map[string]*model.ShareLink // key: token
}

func New(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "exports"), 0755); err != nil {
		return nil, err
	}

	s := &Store{
		dataDir: dataDir,
		users:   make(map[string]model.User),
		queries: make(map[string]model.SavedQuery),
		jobs:    make(map[string]*model.ExportJob),
		shares:  make(map[string]*model.ShareLink),
	}

	s.loadSettings()
	s.loadQueries()
	s.loadUsers()
	s.loadJobs()
	s.loadShares()
	return s, nil
}

func (s *Store) ExportsDir() string {
	return filepath.Join(s.dataDir, "exports")
}

// Settings

// cloneDatasource chuqur nusxa qaytaradi — caller store'ning ichki
// map/slice'iga tegmasligi uchun (mutex'siz yozuv = data race).
func cloneDatasource(ds model.Datasource) model.Datasource {
	c := ds
	if ds.Headers != nil {
		c.Headers = make(map[string]string, len(ds.Headers))
		for k, v := range ds.Headers {
			c.Headers[k] = v
		}
	}
	return c
}

func cloneSettings(st model.Settings) model.Settings {
	c := model.Settings{}
	if st.Datasources != nil {
		c.Datasources = make([]model.Datasource, len(st.Datasources))
		for i, ds := range st.Datasources {
			c.Datasources[i] = cloneDatasource(ds)
		}
	}
	return c
}

// GetSettings chuqur nusxa qaytaradi. Caller uni erkin o'zgartirib
// SaveSettings'ga qaytarishi mumkin.
func (s *Store) GetSettings() model.Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneSettings(s.settings)
}

func (s *Store) SaveSettings(settings model.Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings = cloneSettings(settings)
	return s.saveFile("settings.json", s.settings)
}

// GetDatasource nusxa qaytaradi (store'ning ichki slice'iga pointer emas).
func (s *Store) GetDatasource(id string) *model.Datasource {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := range s.settings.Datasources {
		if s.settings.Datasources[i].ID == id {
			c := cloneDatasource(s.settings.Datasources[i])
			return &c
		}
	}
	return nil
}

func (s *Store) loadSettings() {
	data, err := os.ReadFile(filepath.Join(s.dataDir, "settings.json"))
	if err != nil {
		return
	}
	json.Unmarshal(data, &s.settings)
}

// Queries

func (s *Store) GetQueries() []model.SavedQuery {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]model.SavedQuery, 0, len(s.queries))
	for _, q := range s.queries {
		result = append(result, q)
	}
	return result
}

func (s *Store) SaveQuery(q model.SavedQuery) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queries[q.ID] = q
	return s.saveFile("queries.json", s.queries)
}

func (s *Store) DeleteQuery(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.queries, id)
	return s.saveFile("queries.json", s.queries)
}

func (s *Store) loadQueries() {
	data, err := os.ReadFile(filepath.Join(s.dataDir, "queries.json"))
	if err != nil {
		return
	}
	json.Unmarshal(data, &s.queries)
}

// Users

func (s *Store) GetUsers() []model.User {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]model.User, 0, len(s.users))
	for _, u := range s.users {
		result = append(result, u)
	}
	return result
}

func (s *Store) GetUser(id string) *model.User {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if u, ok := s.users[id]; ok {
		return &u
	}
	return nil
}

func (s *Store) GetUserByUsername(username string) *model.User {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, u := range s.users {
		if u.Username == username {
			return &u
		}
	}
	return nil
}

func (s *Store) SaveUser(u model.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.users[u.ID] = u
	return s.saveFile("users.json", s.users)
}

func (s *Store) DeleteUser(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.users, id)
	return s.saveFile("users.json", s.users)
}

func (s *Store) loadUsers() {
	data, err := os.ReadFile(filepath.Join(s.dataDir, "users.json"))
	if err != nil {
		return
	}
	json.Unmarshal(data, &s.users)
}

// Datasources filtered by user

func (s *Store) GetDatasourcesForUser(userID string) []model.Datasource {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []model.Datasource
	for _, ds := range s.settings.Datasources {
		if ds.Global || ds.OwnerID == userID {
			result = append(result, cloneDatasource(ds))
		}
	}
	return result
}

// Queries filtered by user

func (s *Store) GetQueriesForUser(userID string) []model.SavedQuery {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []model.SavedQuery
	for _, q := range s.queries {
		if q.Global || q.OwnerID == userID {
			result = append(result, q)
		}
	}
	return result
}

// Jobs — `jobs.json` da saqlanadi. Progress (har 1000 qator) faqat xotirada,
// diskka faqat yakuniy holat (done/failed) va qo'shish/o'chirish yoziladi.

func (s *Store) loadJobs() {
	data, err := os.ReadFile(filepath.Join(s.dataDir, "jobs.json"))
	if err != nil {
		return
	}
	json.Unmarshal(data, &s.jobs)
}

func (s *Store) saveJobsLocked() error {
	return s.saveFile("jobs.json", s.jobs)
}

// RecoverJobs server ishga tushganda chaqiriladi:
//   - running holatida qolgan joblar (uzilgan export) failed deb belgilanadi, chala fayl o'chadi;
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
		if job.Status == model.JobRunning {
			job.Status = model.JobFailed
			job.Error = "server qayta ishga tushdi, export uzildi"
			os.Remove(filepath.Join(s.ExportsDir(), job.FileName))
			os.Remove(filepath.Join(s.ExportsDir(), job.FileName+".tmp"))
			interrupted++
			jobsChanged = true
			continue
		}
		known[job.FileName] = true
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

func (s *Store) AddJob(job *model.ExportJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[job.ID] = job
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

func (s *Store) UpdateJobStatus(id string, status model.JobStatus, errMsg string, lines int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	if !ok {
		return
	}
	job.Status = status
	job.Error = errMsg
	job.Lines = lines
	if status != model.JobRunning {
		s.saveJobsLocked()
	}
}

// UpdateJobSize diskka yozmaydi — har doim undan keyin keladigan yakuniy
// UpdateJobStatus(done) persist qiladi.
func (s *Store) UpdateJobSize(id string, size int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if job, ok := s.jobs[id]; ok {
		job.Size = size
	}
}

func (s *Store) DeleteJob(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	if !ok {
		return
	}
	os.Remove(filepath.Join(s.ExportsDir(), job.FileName))
	os.Remove(filepath.Join(s.ExportsDir(), job.FileName+".tmp"))
	delete(s.jobs, id)
	s.saveJobsLocked()
	// Birga shu job uchun yaratilgan share linklarni ham olib tashlash
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

// Shares

func (s *Store) loadShares() {
	data, err := os.ReadFile(filepath.Join(s.dataDir, "shares.json"))
	if err != nil {
		return
	}
	json.Unmarshal(data, &s.shares)
}

func (s *Store) GetShare(token string) *model.ShareLink {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if sh, ok := s.shares[token]; ok {
		c := *sh
		return &c
	}
	return nil
}

func (s *Store) GetShareByJob(jobID string) *model.ShareLink {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, sh := range s.shares {
		if sh.JobID == jobID {
			c := *sh
			return &c
		}
	}
	return nil
}

// SaveShare yangi share saqlaydi. Agar shu job uchun avvalgi share bo'lsa, u
// olib tashlanadi (1 share per job qoidasi).
func (s *Store) SaveShare(share *model.ShareLink) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for token, sh := range s.shares {
		if sh.JobID == share.JobID {
			delete(s.shares, token)
		}
	}
	c := *share
	s.shares[share.Token] = &c
	return s.saveFile("shares.json", s.shares)
}

func (s *Store) DeleteShare(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.shares, token)
	return s.saveFile("shares.json", s.shares)
}

// ConsumeShare atomik tarzda: tekshiradi → countni oshiradi → agar limitga
// yetgan bo'lsa share, job va faylni o'chiradi. Qaytaradi:
//   share — ishlatilayotgan share nusxasi (mavjud bo'lsa)
//   filePath — yuklab olish uchun fayl yo'li (mavjud bo'lsa)
//   exhaustedAfter — true bo'lsa fayl serve qilingandan keyin tozalanishi kerak
//   err — invalidlanish sababi ("not found", "expired", "exhausted")
func (s *Store) ConsumeShare(token string) (share *model.ShareLink, filePath string, exhaustedAfter bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sh, ok := s.shares[token]
	if !ok {
		return nil, "", false, ErrShareNotFound
	}
	now := time.Now()
	if sh.ExpiresAt != nil && !sh.ExpiresAt.After(now) {
		s.purgeShareLocked(sh)
		return nil, "", false, ErrShareExpired
	}
	if sh.MaxDownloads > 0 && sh.DownloadCount >= sh.MaxDownloads {
		s.purgeShareLocked(sh)
		return nil, "", false, ErrShareExhausted
	}

	job, jobOK := s.jobs[sh.JobID]
	if !jobOK || job.Status != model.JobDone {
		s.purgeShareLocked(sh)
		return nil, "", false, ErrShareNotFound
	}

	sh.DownloadCount++
	exhaustedAfter = sh.MaxDownloads > 0 && sh.DownloadCount >= sh.MaxDownloads
	s.saveFile("shares.json", s.shares)

	c := *sh
	return &c, filepath.Join(s.ExportsDir(), job.FileName), exhaustedAfter, nil
}

// PurgeShare share, ushbu job va diskdagi faylni o'chiradi.
// Token bo'yicha yo'q bo'lsa hech narsa qilmaydi.
func (s *Store) PurgeShare(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sh, ok := s.shares[token]; ok {
		s.purgeShareLocked(sh)
	}
}

func (s *Store) purgeShareLocked(sh *model.ShareLink) {
	if job, ok := s.jobs[sh.JobID]; ok {
		os.Remove(filepath.Join(s.ExportsDir(), job.FileName))
		delete(s.jobs, sh.JobID)
		s.saveJobsLocked()
	}
	delete(s.shares, sh.Token)
	s.saveFile("shares.json", s.shares)
}

// CleanupExpiredShares fon ravishda chaqiriladi (har daqiqada). Vaqti chiqqan
// share'larni, ularning job va fayllarini olib tashlaydi. Tozalangan sonni
// qaytaradi.
func (s *Store) CleanupExpiredShares() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	removed := 0
	for _, sh := range s.shares {
		if sh.ExpiresAt != nil && !sh.ExpiresAt.After(now) {
			s.purgeShareLocked(sh)
			removed++
		}
	}
	return removed
}

var (
	ErrShareNotFound  = shareErr("share not found")
	ErrShareExpired   = shareErr("share expired")
	ErrShareExhausted = shareErr("download limit reached")
)

type shareErr string

func (e shareErr) Error() string { return string(e) }

func (s *Store) saveFile(name string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.dataDir, name), data, 0644)
}
