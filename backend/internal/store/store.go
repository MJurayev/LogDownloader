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
	s.loadShares()
	return s, nil
}

func (s *Store) ExportsDir() string {
	return filepath.Join(s.dataDir, "exports")
}

// Settings

func (s *Store) GetSettings() model.Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings
}

func (s *Store) SaveSettings(settings model.Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings = settings
	return s.saveFile("settings.json", settings)
}

func (s *Store) GetDatasource(id string) *model.Datasource {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := range s.settings.Datasources {
		if s.settings.Datasources[i].ID == id {
			return &s.settings.Datasources[i]
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
			result = append(result, ds)
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

// Jobs

func (s *Store) AddJob(job *model.ExportJob) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[job.ID] = job
}

func (s *Store) GetJob(id string) *model.ExportJob {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.jobs[id]
}

func (s *Store) GetJobs() []*model.ExportJob {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*model.ExportJob, 0, len(s.jobs))
	for _, j := range s.jobs {
		result = append(result, j)
	}
	// Eng yangi joblar tepada (CreatedAt DESC).
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result
}

func (s *Store) UpdateJobStatus(id string, status model.JobStatus, errMsg string, lines int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if job, ok := s.jobs[id]; ok {
		job.Status = status
		job.Error = errMsg
		job.Lines = lines
	}
}

func (s *Store) DeleteJob(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if job, ok := s.jobs[id]; ok {
		os.Remove(filepath.Join(s.ExportsDir(), job.FileName))
		delete(s.jobs, id)
		// Birga shu job uchun yaratilgan share linklarni ham olib tashlash
		for token, sh := range s.shares {
			if sh.JobID == id {
				delete(s.shares, token)
			}
		}
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
	return s.shares[token]
}

func (s *Store) GetShareByJob(jobID string) *model.ShareLink {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, sh := range s.shares {
		if sh.JobID == jobID {
			return sh
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
	s.shares[share.Token] = share
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
//   share — ishlatilayotgan share (mavjud bo'lsa)
//   filePath — yuklab olish uchun fayl yo'li (mavjud bo'lsa)
//   err — invalidlanish sababi ("not found", "expired", "exhausted")
//   exhaustedAfter — true bo'lsa fayl serve qilingandan keyin tozalanishi kerak
func (s *Store) ConsumeShare(token string) (share *model.ShareLink, filePath string, err error, exhaustedAfter bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sh, ok := s.shares[token]
	if !ok {
		return nil, "", ErrShareNotFound, false
	}
	now := time.Now()
	if sh.ExpiresAt != nil && !sh.ExpiresAt.After(now) {
		s.purgeShareLocked(sh)
		return nil, "", ErrShareExpired, false
	}
	if sh.MaxDownloads > 0 && sh.DownloadCount >= sh.MaxDownloads {
		s.purgeShareLocked(sh)
		return nil, "", ErrShareExhausted, false
	}

	job, jobOK := s.jobs[sh.JobID]
	if !jobOK || job.Status != model.JobDone {
		s.purgeShareLocked(sh)
		return nil, "", ErrShareNotFound, false
	}

	sh.DownloadCount++
	exhaustedAfter = sh.MaxDownloads > 0 && sh.DownloadCount >= sh.MaxDownloads
	s.saveFile("shares.json", s.shares)

	return sh, filepath.Join(s.ExportsDir(), job.FileName), nil, exhaustedAfter
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
