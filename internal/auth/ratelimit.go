package auth

import (
	"net"
	"net/http"
	"sync"
	"time"
)

type attemptWindow struct {
	failures  int
	resetTime time.Time
}

type LoginLimiter struct {
	mu          sync.Mutex
	attempts    map[string]attemptWindow
	maxFailures int
	window      time.Duration
}

func NewLoginLimiter(maxFailures int, window time.Duration) *LoginLimiter {
	return &LoginLimiter{
		attempts:    make(map[string]attemptWindow),
		maxFailures: maxFailures,
		window:      window,
	}
}

func clientIPKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (l *LoginLimiter) Allow(r *http.Request) (bool, time.Duration) {
	key := clientIPKey(r)
	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	entry, ok := l.attempts[key]
	if !ok || now.After(entry.resetTime) {
		return true, 0
	}
	if entry.failures >= l.maxFailures {
		return false, time.Until(entry.resetTime)
	}
	return true, 0
}

func (l *LoginLimiter) RecordFailure(r *http.Request) {
	key := clientIPKey(r)
	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	entry, ok := l.attempts[key]
	if !ok || now.After(entry.resetTime) {
		l.attempts[key] = attemptWindow{
			failures:  1,
			resetTime: now.Add(l.window),
		}
		return
	}
	entry.failures++
	l.attempts[key] = entry
}

func (l *LoginLimiter) Reset(r *http.Request) {
	key := clientIPKey(r)
	l.mu.Lock()
	delete(l.attempts, key)
	l.mu.Unlock()
}
