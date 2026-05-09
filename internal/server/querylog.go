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
	max     int
}

var queryLog = &QueryLog{max: 500}

func (l *QueryLog) Add(e QueryEntry) {
	l.mu.Lock()
	l.entries = append(l.entries, e)
	if len(l.entries) > l.max {
		l.entries = l.entries[len(l.entries)-l.max:]
	}
	l.mu.Unlock()
}

func (l *QueryLog) Recent(n int) []QueryEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if len(l.entries) <= n {
		out := make([]QueryEntry, len(l.entries))
		copy(out, l.entries)
		return out
	}
	out := make([]QueryEntry, n)
	copy(out, l.entries[len(l.entries)-n:])
	return out
}
