package server

import (
	"sync"
	"time"
)

type QueryEntry struct {
	Time    time.Time `json:"time"`
	Domain  string    `json:"domain"`
	Type    string    `json:"type"`
	Client  string    `json:"client"`
	Status  string    `json:"status"`
	Latency int64     `json:"latency_ms"`
}

type QueryLog struct {
	mu      sync.RWMutex
	entries []QueryEntry
	head    int
	count   int
	max     int
}

func newQueryLog(max int) *QueryLog {
	return &QueryLog{max: max, entries: make([]QueryEntry, max)}
}

func (l *QueryLog) Add(e QueryEntry) {
	l.mu.Lock()
	l.entries[l.head] = e
	l.head = (l.head + 1) % l.max
	if l.count < l.max {
		l.count++
	}
	l.mu.Unlock()
}

func (l *QueryLog) Recent(n int) []QueryEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if n > l.count {
		n = l.count
	}
	out := make([]QueryEntry, n)
	for i := range n {
		out[i] = l.entries[(l.head-n+i+l.max)%l.max]
	}
	return out
}
