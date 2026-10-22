package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"logdownloader/internal/model"
)

// Store — disk ustidagi JSON fayllar + xotiradagi map'lar, hammasi bitta
// RWMutex ostida. Getter'lar nusxa qaytaradi (ichki ma'lumotga pointer emas),
// shu sababli handler/worker mutex'siz o'zgartirib Save* ga qaytarishi mumkin.
//
// Fayllar: settings.json, users.json, queries.json, jobs.json, shares.json,
// schedules.json, history.json, audit.jsonl (alohida mutex), jwt_secret, exports/.
type Store struct {
	mu        sync.RWMutex
	dataDir   string
	settings  model.Settings
	users     map[string]model.User
	queries   map[string]model.SavedQuery
	jobs      map[string]*model.ExportJob
	shares    map[string]*model.ShareLink // key: token
	schedules map[string]*model.Schedule
	history   map[string][]model.HistoryEntry // key: user ID

	auditMu sync.Mutex
}

const (
	DefaultRetentionDays = 30
	DefaultMaxConcurrent = 2
)

func New(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "exports"), 0755); err != nil {
		return nil, err
	}

	s := &Store{
		dataDir:   dataDir,
		users:     make(map[string]model.User),
		queries:   make(map[string]model.SavedQuery),
		jobs:      make(map[string]*model.ExportJob),
		shares:    make(map[string]*model.ShareLink),
		schedules: make(map[string]*model.Schedule),
		history:   make(map[string][]model.HistoryEntry),
	}

	s.loadSettings()
	s.loadJSON("queries.json", &s.queries)
	s.loadJSON("users.json", &s.users)
	s.loadJSON("jobs.json", &s.jobs)
	s.loadJSON("shares.json", &s.shares)
	s.loadJSON("schedules.json", &s.schedules)
	s.loadJSON("history.json", &s.history)
	return s, nil
}

func (s *Store) DataDir() string    { return s.dataDir }
func (s *Store) ExportsDir() string { return filepath.Join(s.dataDir, "exports") }

func (s *Store) loadJSON(name string, v any) {
	data, err := os.ReadFile(filepath.Join(s.dataDir, name))
	if err != nil {
		return
	}
	json.Unmarshal(data, v)
}

func (s *Store) saveFile(name string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.dataDir, name), data, 0644)
}

// Settings

func (s *Store) loadSettings() {
	s.loadJSON("settings.json", &s.settings)
	changed := false
	if s.settings.Retention.Days == 0 && s.settings.Retention.QuotaGB == 0 {
		// Birinchi ishga tushish: oqilona default (admin UI'dan o'zgartiradi).
		if _, err := os.Stat(filepath.Join(s.dataDir, "settings.json")); err != nil {
			s.settings.Retention.Days = DefaultRetentionDays
			changed = true
		}
	}
	if s.settings.Worker.MaxConcurrent <= 0 {
		s.settings.Worker.MaxConcurrent = DefaultMaxConcurrent
		changed = true
	}
	if changed {
		s.saveFile("settings.json", s.settings)
	}
}

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
	c := st
	c.Datasources = nil
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
	if settings.Worker.MaxConcurrent <= 0 {
		settings.Worker.MaxConcurrent = DefaultMaxConcurrent
	}
	s.settings = cloneSettings(settings)
	return s.saveFile("settings.json", s.settings)
}

// UpdateSettings — datasource'larga tegmasdan boshqa sozlamalarni yangilash.
func (s *Store) UpdateSettings(fn func(st *model.Settings)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := cloneSettings(s.settings)
	fn(&c)
	if c.Worker.MaxConcurrent <= 0 {
		c.Worker.MaxConcurrent = DefaultMaxConcurrent
	}
	s.settings = c
	return s.saveFile("settings.json", s.settings)
}

func (s *Store) MaxConcurrent() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.settings.Worker.MaxConcurrent <= 0 {
		return DefaultMaxConcurrent
	}
	return s.settings.Worker.MaxConcurrent
}

func (s *Store) PublicURL() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings.PublicURL
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
