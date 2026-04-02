package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"logdownloader/internal/model"
)

type Store struct {
	mu       sync.RWMutex
	dataDir  string
	settings model.Settings
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
		queries: make(map[string]model.SavedQuery),
		jobs:    make(map[string]*model.ExportJob),
	}

	s.loadSettings()
	s.loadQueries()
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
