package auth

import (
	"sync"
	"time"
)

// Limiter — xotiradagi oddiy sliding-window hisoblagich (login brute-force
// himoyasi). Kalit bo'yicha hodisa vaqtlarini saqlaydi; Count oynadagi sonni
// qaytaradi. Eski kalitlar davriy tozalanadi.
type Limiter struct {
	mu     sync.Mutex
	events map[string][]time.Time
	last   time.Time
}

func NewLimiter() *Limiter {
	return &Limiter{events: make(map[string][]time.Time), last: time.Now()}
}

func (l *Limiter) prune(key string, window time.Duration, now time.Time) []time.Time {
	list := l.events[key]
	cut := now.Add(-window)
	i := 0
	for i < len(list) && list[i].Before(cut) {
		i++
	}
	list = list[i:]
	if len(list) == 0 {
		delete(l.events, key)
	} else {
		l.events[key] = list
	}
	return list
}

// Hit — hodisani qayd etadi va oynadagi jami sonini qaytaradi.
func (l *Limiter) Hit(key string, window time.Duration) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	l.gc(now)
	list := l.prune(key, window, now)
	list = append(list, now)
	l.events[key] = list
	return len(list)
}

// Count — qayd etmasdan oynadagi sonni qaytaradi.
func (l *Limiter) Count(key string, window time.Duration) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.prune(key, window, time.Now()))
}

func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.events, key)
}

// gc — har 5 daqiqada bir marta barcha kalitlarni 1 soatlik oynaga qarab tozalaydi.
func (l *Limiter) gc(now time.Time) {
	if now.Sub(l.last) < 5*time.Minute {
		return
	}
	l.last = now
	for k := range l.events {
		l.prune(k, time.Hour, now)
	}
}
