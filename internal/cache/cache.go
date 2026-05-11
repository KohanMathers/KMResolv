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

const numShards = 64

type cacheShard struct {
	mu       sync.RWMutex
	entries  map[string]*cacheEntry
	negative map[string]negEntry
}

type Cache struct {
	shards     [numShards]cacheShard
	prefetchFn PrefetchFn
	minTTL     uint32
}

func (c *Cache) SetMinTTL(n uint32) {
	c.minTTL = n
}

func shardIdx(key string) uint32 {
	var h uint32 = 2166136261
	for i := 0; i < len(key); i++ {
		h ^= uint32(key[i])
		h *= 16777619
	}
	return h & (numShards - 1)
}

func (c *Cache) shard(key string) *cacheShard {
	return &c.shards[shardIdx(key)]
}

func NewCache() *Cache {
	c := &Cache{}
	for i := range c.shards {
		c.shards[i].entries = make(map[string]*cacheEntry)
		c.shards[i].negative = make(map[string]negEntry)
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
	key := cacheKey(name, qtype)
	s := c.shard(key)
	s.mu.Lock()
	s.negative[key] = negEntry{expires: time.Now().Add(time.Duration(ttl) * time.Second)}
	s.mu.Unlock()
	logger.LogDebug("cache negative: %s TTL=%ds", name, ttl)
}

func (c *Cache) IsNegative(name string, qtype uint16) bool {
	key := cacheKey(name, qtype)
	s := c.shard(key)
	s.mu.RLock()
	e, ok := s.negative[key]
	s.mu.RUnlock()
	return ok && time.Now().Before(e.expires)
}

func cacheKey(name string, qtype uint16) string {
	return name + ":" + string(rune(qtype))
}

func (c *Cache) Get(name string, qtype uint16, cfg *config.Config) *dns.Message {
	key := cacheKey(name, qtype)
	s := c.shard(key)
	s.mu.RLock()
	e, ok := s.entries[key]
	s.mu.RUnlock()
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
	if c.minTTL > 0 && ttl < c.minTTL {
		ttl = c.minTTL
	}
	now := time.Now()
	key := cacheKey(name, qtype)
	s := c.shard(key)
	s.mu.Lock()
	s.entries[key] = &cacheEntry{
		msg:     msg,
		expires: now.Add(time.Duration(ttl) * time.Second),
		cached:  now,
	}
	s.mu.Unlock()
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
		for i := range c.shards {
			s := &c.shards[i]
			s.mu.Lock()
			for k, e := range s.entries {
				if now.After(e.expires) {
					delete(s.entries, k)
				}
			}
			for k, e := range s.negative {
				if now.After(e.expires) {
					delete(s.negative, k)
				}
			}
			s.mu.Unlock()
		}
	}
}

func (c *Cache) Size() int {
	var total int
	for i := range c.shards {
		c.shards[i].mu.RLock()
		total += len(c.shards[i].entries)
		c.shards[i].mu.RUnlock()
	}
	return total
}

func (c *Cache) NegativeSize() int {
	var total int
	for i := range c.shards {
		c.shards[i].mu.RLock()
		total += len(c.shards[i].negative)
		c.shards[i].mu.RUnlock()
	}
	return total
}

func (c *Cache) Flush(mode string) {
	for i := range c.shards {
		s := &c.shards[i]
		s.mu.Lock()
		switch mode {
		case "negative":
			s.negative = make(map[string]negEntry)
		case "expired":
			now := time.Now()
			for k, e := range s.entries {
				if now.After(e.expires) {
					delete(s.entries, k)
				}
			}
			for k, e := range s.negative {
				if now.After(e.expires) {
					delete(s.negative, k)
				}
			}
		default:
			s.entries = make(map[string]*cacheEntry)
			s.negative = make(map[string]negEntry)
		}
		s.mu.Unlock()
	}
	logger.LogInfo("cache flushed: mode=%s", mode)
}
