package store

import (
	"strings"
	"time"

	"logdownloader/internal/model"
)

// Query history — `history.json`, har user uchun oxirgi HistoryLimit ta query
// (eng yangisi birinchi, dublikatlar yuqoriga ko'chadi).

const HistoryLimit = 50

func (s *Store) AddHistory(userID, query, datasourceID string) {
	query = strings.TrimSpace(query)
	if query == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	list := s.history[userID]
	filtered := make([]model.HistoryEntry, 0, len(list)+1)
	filtered = append(filtered, model.HistoryEntry{Query: query, DatasourceID: datasourceID, At: time.Now()})
	for _, h := range list {
		if h.Query == query {
			continue
		}
		filtered = append(filtered, h)
		if len(filtered) >= HistoryLimit {
			break
		}
	}
	s.history[userID] = filtered
	s.saveFile("history.json", s.history)
}

func (s *Store) GetHistory(userID string) []model.HistoryEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]model.HistoryEntry(nil), s.history[userID]...)
}

func (s *Store) ClearHistory(userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.history, userID)
	s.saveFile("history.json", s.history)
}
