package gateway

import (
	"net"
	"sync"
	"time"
)

const (
	defaultAuthFailures = 5
	defaultAuthWindow   = time.Minute
	defaultAuthLockout  = time.Minute
	defaultAuthEntries  = 4096
)

type authAttempt struct {
	failures     int
	windowStart  time.Time
	blockedUntil time.Time
	lastSeen     time.Time
}

type authLimiter struct {
	mu          sync.Mutex
	attempts    map[string]authAttempt
	now         func() time.Time
	maxFailures int
	window      time.Duration
	lockout     time.Duration
	maxEntries  int
}

func newAuthLimiter() *authLimiter {
	return &authLimiter{
		attempts:    make(map[string]authAttempt),
		now:         time.Now,
		maxFailures: defaultAuthFailures,
		window:      defaultAuthWindow,
		lockout:     defaultAuthLockout,
		maxEntries:  defaultAuthEntries,
	}
}

func (l *authLimiter) allowed(remoteAddress string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	key := authKey(remoteAddress)
	attempt, ok := l.attempts[key]
	if !ok {
		return true
	}
	if !attempt.blockedUntil.IsZero() && now.Before(attempt.blockedUntil) {
		attempt.lastSeen = now
		l.attempts[key] = attempt
		return false
	}
	if now.Sub(attempt.windowStart) >= l.window {
		delete(l.attempts, key)
	}
	return true
}

func (l *authLimiter) failure(remoteAddress string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	key := authKey(remoteAddress)
	attempt, ok := l.attempts[key]
	if !ok && len(l.attempts) >= l.maxEntries {
		l.evictOldest()
	}
	if !ok || now.Sub(attempt.windowStart) >= l.window {
		attempt = authAttempt{windowStart: now}
	}
	attempt.failures++
	attempt.lastSeen = now
	if attempt.failures >= l.maxFailures {
		attempt.blockedUntil = now.Add(l.lockout)
	}
	l.attempts[key] = attempt
}

func (l *authLimiter) success(remoteAddress string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, authKey(remoteAddress))
}

func (l *authLimiter) evictOldest() {
	var oldestKey string
	var oldest time.Time
	for key, attempt := range l.attempts {
		if oldestKey == "" || attempt.lastSeen.Before(oldest) {
			oldestKey = key
			oldest = attempt.lastSeen
		}
	}
	delete(l.attempts, oldestKey)
}

func authKey(remoteAddress string) string {
	host, _, err := net.SplitHostPort(remoteAddress)
	if err == nil && host != "" {
		return host
	}
	return remoteAddress
}
