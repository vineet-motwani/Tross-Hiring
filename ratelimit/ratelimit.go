package ratelimit

import (
	"sync"
	"time"
)

type InMemoryRateLimiter struct {
	limit      int
	maxClients int
	requests   map[string][]float64
	mu         sync.Mutex
}

func NewInMemoryRateLimiter(requestsPerMinute int, maxClients int) *InMemoryRateLimiter {
	if maxClients == 0 {
		maxClients = 2048
	}
	return &InMemoryRateLimiter{
		limit:      requestsPerMinute,
		maxClients: maxClients,
		requests:   make(map[string][]float64),
	}
}

func (l *InMemoryRateLimiter) Allow(clientKey string) bool {
	now := float64(time.Now().UnixNano()) / 1e9
	cutoff := now - 60.0

	l.mu.Lock()
	defer l.mu.Unlock()

	l.prune(cutoff)

	timestamps, exists := l.requests[clientKey]
	if !exists {
		if len(l.requests) >= l.maxClients {
			return false
		}
		timestamps = make([]float64, 0)
	}

	// Filter timestamps greater than cutoff
	var active []float64
	for _, t := range timestamps {
		if t > cutoff {
			active = append(active, t)
		}
	}
	timestamps = active
	l.requests[clientKey] = timestamps

	if len(timestamps) >= l.limit {
		return false
	}

	l.requests[clientKey] = append(l.requests[clientKey], now)
	return true
}

func (l *InMemoryRateLimiter) prune(cutoff float64) {
	for key, values := range l.requests {
		if len(values) == 0 || values[len(values)-1] <= cutoff {
			delete(l.requests, key)
		}
	}
}
