package ratelimit

import (
	"sync"
	"time"
)

type Policy struct {
	MaxAttempts int
	Window      time.Duration
}

type entry struct {
	Count int
	Reset time.Time
}

type Limiter struct {
	mu      sync.Mutex
	entries map[string]entry
	now     func() time.Time
}

func New() *Limiter {
	return &Limiter{entries: make(map[string]entry), now: time.Now}
}

func (l *Limiter) Allow(key string, policy Policy) (bool, time.Duration) {
	if policy.MaxAttempts <= 0 || policy.Window <= 0 {
		return true, 0
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	item, ok := l.entries[key]
	if !ok || !now.Before(item.Reset) {
		l.entries[key] = entry{Count: 1, Reset: now.Add(policy.Window)}
		return true, 0
	}
	if item.Count >= policy.MaxAttempts {
		return false, item.Reset.Sub(now)
	}
	item.Count++
	l.entries[key] = item
	return true, 0
}

func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	delete(l.entries, key)
	l.mu.Unlock()
}

func (l *Limiter) Cleanup() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	removed := 0
	for key, item := range l.entries {
		if !now.Before(item.Reset) {
			delete(l.entries, key)
			removed++
		}
	}
	return removed
}
