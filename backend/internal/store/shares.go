package store

import (
	"crypto/rand"
	"encoding/base64"
	"path/filepath"
	"time"

	"logdownloader/internal/model"
)

var (
	ErrShareNotFound  = shareErr("share not found")
	ErrShareExpired   = shareErr("share expired")
	ErrShareExhausted = shareErr("download limit reached")
)

type shareErr string

func (e shareErr) Error() string { return string(e) }

// NewShareToken — 16 bayt random, URL-safe base64.
func NewShareToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
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
// yetgan bo'lsa share, job va faylni o'chiradi. fileIndex — job.Files ichidagi
// 0-asosli indeks (ko'p arxivli export); chegaradan tashqarida bo'lsa 0.
// Qaytaradi: share nusxasi, fayl yo'li, fayl nomi, "serve'dan keyin tozalash" belgisi, xato.
func (s *Store) ConsumeShare(token string, fileIndex int) (share *model.ShareLink, filePath, fileName string, exhaustedAfter bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sh, ok := s.shares[token]
	if !ok {
		return nil, "", "", false, ErrShareNotFound
	}
	now := time.Now()
	if sh.ExpiresAt != nil && !sh.ExpiresAt.After(now) {
		s.purgeShareLocked(sh)
		return nil, "", "", false, ErrShareExpired
	}
	if sh.MaxDownloads > 0 && sh.DownloadCount >= sh.MaxDownloads {
		s.purgeShareLocked(sh)
		return nil, "", "", false, ErrShareExhausted
	}

	job, jobOK := s.jobs[sh.JobID]
	if !jobOK || job.Status != model.JobDone {
		s.purgeShareLocked(sh)
		return nil, "", "", false, ErrShareNotFound
	}

	name := job.FileName
	if fileIndex >= 0 && fileIndex < len(job.Files) {
		name = job.Files[fileIndex].Name
	}

	sh.DownloadCount++
	exhaustedAfter = sh.MaxDownloads > 0 && sh.DownloadCount >= sh.MaxDownloads
	s.saveFile("shares.json", s.shares)

	c := *sh
	return &c, filepath.Join(s.ExportsDir(), name), name, exhaustedAfter, nil
}

// PurgeShare share, ushbu job va diskdagi fayllarni o'chiradi.
func (s *Store) PurgeShare(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sh, ok := s.shares[token]; ok {
		s.purgeShareLocked(sh)
	}
}

func (s *Store) purgeShareLocked(sh *model.ShareLink) {
	if job, ok := s.jobs[sh.JobID]; ok {
		s.removeJobFilesLocked(job)
		delete(s.jobs, sh.JobID)
		s.saveJobsLocked()
	}
	delete(s.shares, sh.Token)
	s.saveFile("shares.json", s.shares)
}

// CleanupExpiredShares fon ravishda chaqiriladi (har daqiqada). Vaqti chiqqan
// share'larni, ularning job va fayllarini olib tashlaydi.
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
