package cache

import (
	"sync"
	"time"

	"github.com/kohanmathers/kmresolv/internal/config"
	"github.com/kohanmathers/kmresolv/internal/dns"
	"github.com/kohanmathers/kmresolv/internal/logger"
)

type PrefetchFn func(name string, qtype uint16) (*dns.Message, error)

type cacheEntry struct {
	msg     *dns.Message
	expires time.Time
	cached  time.Time
}

type negEntry struct {
	expires time.Time
}

type Cache struct {
	mu         sync.RWMutex
	entries    map[string]*cacheEntry
	negative   map[string]negEntry
	prefetchFn PrefetchFn
}

func NewCache() *Cache {
	c := &Cache{
		entries:  make(map[string]*cacheEntry),
		negative: make(map[string]negEntry),
	}
	go c.evictLoop()
	return c
}

func (c *Cache) SetPrefetchFn(fn PrefetchFn) {
	c.prefetchFn = fn
}

func (c *Cache) SetNegative(name string, qtype uint16, ttl int) {
	if ttl == 0 {
		ttl = 300
	}
	c.mu.Lock()
	c.negative[cacheKey(name, qtype)] = negEntry{
		expires: time.Now().Add(time.Duration(ttl) * time.Second),
	}
	c.mu.Unlock()
	logger.LogDebug("cache negative: %s TTL=%ds", name, ttl)
}

func (c *Cache) IsNegative(name string, qtype uint16) bool {
	c.mu.RLock()
	e, ok := c.negative[cacheKey(name, qtype)]
	c.mu.RUnlock()
	return ok && time.Now().Before(e.expires)
}

func cacheKey(name string, qtype uint16) string {
	return name + ":" + string(rune(qtype))
}

func (c *Cache) Get(name string, qtype uint16, cfg *config.Config) *dns.Message {
	c.mu.RLock()
	e, ok := c.entries[cacheKey(name, qtype)]
	c.mu.RUnlock()
	if !ok || time.Now().After(e.expires) {
		return nil
	}

	if cfg.Resolver.Cache.Prefetch && c.prefetchFn != nil {
		total := e.expires.Sub(e.cached)
		remaining := time.Until(e.expires)
		if remaining < total/10 {
			pf := c.prefetchFn
			go func() {
				logger.LogDebug("prefetching: %s", name)
				if msg, err := pf(name, qtype); err == nil {
					c.Set(name, qtype, msg)
				}
			}()
		}
	}

	elapsed := uint32(time.Since(e.cached).Seconds())
	return cloneMessageWithDecrementedTTL(e.msg, elapsed)
}

func cloneMessageWithDecrementedTTL(msg *dns.Message, elapsed uint32) *dns.Message {
	clone := *msg
	clone.Answers = decrementRRs(msg.Answers, elapsed)
	clone.Authority = decrementRRs(msg.Authority, elapsed)
	clone.Additional = decrementRRs(msg.Additional, elapsed)
	return &clone
}

func decrementRRs(rrs []dns.RR, elapsed uint32) []dns.RR {
	out := make([]dns.RR, len(rrs))
	for i, rr := range rrs {
		if rr.TTL > elapsed {
			rr.TTL -= elapsed
		} else {
			rr.TTL = 0
		}
		out[i] = rr
	}
	return out
}

func (c *Cache) Set(name string, qtype uint16, msg *dns.Message) {
	ttl := lowestTTL(msg)
	if ttl == 0 {
		return
	}
	c.mu.Lock()
	c.entries[cacheKey(name, qtype)] = &cacheEntry{
		msg:     msg,
		expires: time.Now().Add(time.Duration(ttl) * time.Second),
		cached:  time.Now(),
	}
	c.mu.Unlock()
}

func lowestTTL(msg *dns.Message) uint32 {
	var min uint32 = ^uint32(0)
	for _, rr := range msg.Answers {
		if rr.TTL < min {
			min = rr.TTL
		}
	}
	if min == ^uint32(0) {
		return 0
	}
	return min
}

func (c *Cache) evictLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		c.mu.Lock()
		for k, e := range c.entries {
			if now.After(e.expires) {
				delete(c.entries, k)
			}
		}
		for k, e := range c.negative {
			if now.After(e.expires) {
				delete(c.negative, k)
			}
		}
		c.mu.Unlock()
	}
}

func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

func (c *Cache) NegativeSize() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.negative)
}

func (c *Cache) Flush(mode string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch mode {
	case "negative":
		c.negative = make(map[string]negEntry)
	case "expired":
		now := time.Now()
		for k, e := range c.entries {
			if now.After(e.expires) {
				delete(c.entries, k)
			}
		}
		for k, e := range c.negative {
			if now.After(e.expires) {
				delete(c.negative, k)
			}
		}
	default:
		c.entries = make(map[string]*cacheEntry)
		c.negative = make(map[string]negEntry)
	}
	logger.LogInfo("cache flushed: mode=%s", mode)
}
