package store

import (
	"sort"

	"logdownloader/internal/model"
)

// Schedules — `schedules.json`.

func cloneSchedule(sc *model.Schedule) *model.Schedule {
	c := *sc
	if sc.Fields != nil {
		c.Fields = append([]string(nil), sc.Fields...)
	}
	return &c
}

func (s *Store) GetSchedule(id string) *model.Schedule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if sc, ok := s.schedules[id]; ok {
		return cloneSchedule(sc)
	}
	return nil
}

// GetSchedulesForUser — all=true (admin) hammasi, aks holda faqat o'ziniki. Nom bo'yicha.
func (s *Store) GetSchedulesForUser(ownerID string, all bool) []model.Schedule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]model.Schedule, 0, len(s.schedules))
	for _, sc := range s.schedules {
		if all || sc.OwnerID == ownerID {
			result = append(result, *cloneSchedule(sc))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result
}

// AllSchedules — scheduler uchun (barchasi).
func (s *Store) AllSchedules() []model.Schedule {
	return s.GetSchedulesForUser("", true)
}

func (s *Store) SaveSchedule(sc *model.Schedule) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.schedules[sc.ID] = cloneSchedule(sc)
	return s.saveFile("schedules.json", s.schedules)
}

// UpdateSchedule — mutex ostida qisman yangilash (scheduler run natijalari).
func (s *Store) UpdateSchedule(id string, fn func(sc *model.Schedule)) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	sc, ok := s.schedules[id]
	if !ok {
		return false
	}
	fn(sc)
	s.saveFile("schedules.json", s.schedules)
	return true
}

func (s *Store) DeleteSchedule(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.schedules, id)
	return s.saveFile("schedules.json", s.schedules)
}
