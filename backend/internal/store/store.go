package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"logdownloader/internal/model"
)

type Store struct {
	mu       sync.RWMutex
	dataDir  string
	settings model.Settings
	users    map[string]model.User
	queries  map[string]model.SavedQuery
	jobs     map[string]*model.ExportJob
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
	}

	s.loadSettings()
	s.loadQueries()
	s.loadUsers()
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
	}
}

func (s *Store) saveFile(name string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.dataDir, name), data, 0644)
}
