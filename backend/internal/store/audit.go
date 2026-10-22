package store

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"logdownloader/internal/model"
)

// Audit log — `audit.jsonl` (har qator bitta JSON). Faqat qo'shiladi.
// Fayl AuditRotateBytes dan oshsa `audit-<ts>.jsonl` ga aylantiriladi,
// eng ko'pi AuditKeepFiles ta eski fayl saqlanadi.

const (
	AuditRotateBytes = 20 * 1024 * 1024
	AuditKeepFiles   = 3
)

func (s *Store) AuditFilePath() string {
	return filepath.Join(s.dataDir, "audit.jsonl")
}

// Audit — yozuvni qo'shadi. Xato bo'lsa jim o'tadi (audit asosiy ishni to'xtatmasin).
func (s *Store) Audit(e model.AuditEntry) {
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	data, err := json.Marshal(e)
	if err != nil {
		return
	}
	s.auditMu.Lock()
	defer s.auditMu.Unlock()
	path := s.AuditFilePath()
	if info, err := os.Stat(path); err == nil && info.Size() >= AuditRotateBytes {
		s.rotateAuditLocked()
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	f.Write(append(data, '\n'))
}

func (s *Store) rotateAuditLocked() {
	ts := time.Now().UTC().Format("20060102-150405")
	os.Rename(s.AuditFilePath(), filepath.Join(s.dataDir, fmt.Sprintf("audit-%s.jsonl", ts)))
	matches, _ := filepath.Glob(filepath.Join(s.dataDir, "audit-*.jsonl"))
	if len(matches) > AuditKeepFiles {
		// Glob natijasi nom bo'yicha tartiblangan → eng eskisi birinchi.
		for _, old := range matches[:len(matches)-AuditKeepFiles] {
			os.Remove(old)
		}
	}
}

// ReadAudit — joriy fayldan yozuvlar, eng yangisi birinchi. action/user bo'sh
// bo'lmasa filtr (substring, case-insensitive). total — filtrdan o'tgan jami.
func (s *Store) ReadAudit(limit, offset int, action, user string) (entries []model.AuditEntry, total int) {
	s.auditMu.Lock()
	defer s.auditMu.Unlock()
	f, err := os.Open(s.AuditFilePath())
	if err != nil {
		return nil, 0
	}
	defer f.Close()
	var all []model.AuditEntry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	action = strings.ToLower(action)
	user = strings.ToLower(user)
	for sc.Scan() {
		var e model.AuditEntry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			continue
		}
		if action != "" && !strings.Contains(strings.ToLower(e.Action), action) {
			continue
		}
		if user != "" && !strings.Contains(strings.ToLower(e.User), user) {
			continue
		}
		all = append(all, e)
	}
	// teskari tartib
	for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
		all[i], all[j] = all[j], all[i]
	}
	total = len(all)
	if offset >= len(all) {
		return []model.AuditEntry{}, total
	}
	all = all[offset:]
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all, total
}

// AuditLineCount — metrics uchun (taxminan, faqat joriy fayl).
func (s *Store) AuditLineCount() int {
	s.auditMu.Lock()
	defer s.auditMu.Unlock()
	f, err := os.Open(s.AuditFilePath())
	if err != nil {
		return 0
	}
	defer f.Close()
	n := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		n++
	}
	return n
}
