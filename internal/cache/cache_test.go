package cache

import (
	"testing"
	"time"

	"github.com/kohanmathers/kmresolv/internal/config"
	"github.com/kohanmathers/kmresolv/internal/dns"
)

func cfgWithCache(prefetch bool) *config.Config {
	return &config.Config{
		Resolver: config.ResolverConfig{
			Cache: config.CacheConfig{
				Enabled:  true,
				Prefetch: prefetch,
			},
		},
	}
}

func makeMsg(ttl uint32) *dns.Message {
	return &dns.Message{
		Answers: []dns.RR{
			{Name: "example.com", Type: dns.TypeA, Class: dns.ClassIN, TTL: ttl, Data: []byte{1, 2, 3, 4}},
		},
	}
}

func TestCacheKey(t *testing.T) {
	if cacheKey("a.com", 1) == cacheKey("a.com", 28) {
		t.Error("different qtypes must produce different keys")
	}
	if cacheKey("a.com", 1) == cacheKey("b.com", 1) {
		t.Error("different names must produce different keys")
	}
	if cacheKey("a.com", 1) != cacheKey("a.com", 1) {
		t.Error("same inputs must produce same key")
	}
}

func TestSetGet(t *testing.T) {
	c := NewCache()
	cfg := cfgWithCache(false)

	c.Set("example.com", dns.TypeA, makeMsg(300))

	got := c.Get("example.com", dns.TypeA, cfg)
	if got == nil {
		t.Fatal("expected cached message, got nil")
	}
	if len(got.Answers) != 1 {
		t.Fatalf("expected 1 answer, got %d", len(got.Answers))
	}
}

func TestGetMiss(t *testing.T) {
	c := NewCache()
	cfg := cfgWithCache(false)

	if c.Get("missing.com", dns.TypeA, cfg) != nil {
		t.Error("missing entry should return nil")
	}
}

func TestSetZeroTTLNotCached(t *testing.T) {
	c := NewCache()
	cfg := cfgWithCache(false)

	c.Set("example.com", dns.TypeA, makeMsg(0))

	if c.Get("example.com", dns.TypeA, cfg) != nil {
		t.Error("message with zero TTL should not be cached")
	}
}

func TestTTLDecrement(t *testing.T) {
	c := NewCache()
	cfg := cfgWithCache(false)

	const original uint32 = 300
	key := cacheKey("example.com", dns.TypeA)
	elapsed := 10 * time.Second

	c.mu.Lock()
	c.entries[key] = &cacheEntry{
		msg:     makeMsg(original),
		expires: time.Now().Add(time.Duration(original)*time.Second - elapsed),
		cached:  time.Now().Add(-elapsed),
	}
	c.mu.Unlock()

	got := c.Get("example.com", dns.TypeA, cfg)
	if got == nil {
		t.Fatal("expected cached entry")
	}
	if got.Answers[0].TTL >= original {
		t.Errorf("TTL should be decremented below %d, got %d", original, got.Answers[0].TTL)
	}
}

func TestSize(t *testing.T) {
	c := NewCache()

	if c.Size() != 0 {
		t.Errorf("initial size should be 0, got %d", c.Size())
	}
	c.Set("a.com", dns.TypeA, makeMsg(60))
	c.Set("b.com", dns.TypeA, makeMsg(60))
	if c.Size() != 2 {
		t.Errorf("expected size 2, got %d", c.Size())
	}
}

func TestNegativeCacheRoundTrip(t *testing.T) {
	c := NewCache()

	if c.IsNegative("nxd.com", dns.TypeA) {
		t.Fatal("should not be negative before setting")
	}
	c.SetNegative("nxd.com", dns.TypeA, 300)
	if !c.IsNegative("nxd.com", dns.TypeA) {
		t.Fatal("should be negative after setting")
	}
}

func TestNegativeCacheSeparateTypes(t *testing.T) {
	c := NewCache()

	c.SetNegative("a.com", dns.TypeA, 300)
	c.SetNegative("a.com", dns.TypeAAAA, 300)

	if c.NegativeSize() != 2 {
		t.Errorf("expected 2 negative entries, got %d", c.NegativeSize())
	}
}

func TestNegativeCacheDefaultTTL(t *testing.T) {
	c := NewCache()
	c.SetNegative("x.com", dns.TypeA, 0)
	if !c.IsNegative("x.com", dns.TypeA) {
		t.Error("default TTL entry should still be in negative cache")
	}
}

func TestNegativeCacheExpiredEntry(t *testing.T) {
	c := NewCache()

	c.mu.Lock()
	c.negative[cacheKey("old.com", dns.TypeA)] = negEntry{expires: time.Now().Add(-1 * time.Second)}
	c.mu.Unlock()

	if c.IsNegative("old.com", dns.TypeA) {
		t.Error("expired negative entry should not be returned as negative")
	}
}

func TestFlushAll(t *testing.T) {
	c := NewCache()
	c.Set("a.com", dns.TypeA, makeMsg(60))
	c.SetNegative("b.com", dns.TypeA, 300)

	c.Flush("")

	if c.Size() != 0 {
		t.Errorf("positive cache should be empty after full flush, got %d", c.Size())
	}
	if c.NegativeSize() != 0 {
		t.Errorf("negative cache should be empty after full flush, got %d", c.NegativeSize())
	}
}

func TestFlushNegative(t *testing.T) {
	c := NewCache()
	c.Set("a.com", dns.TypeA, makeMsg(60))
	c.SetNegative("b.com", dns.TypeA, 300)

	c.Flush("negative")

	if c.Size() != 1 {
		t.Errorf("positive entries should survive negative flush, got %d", c.Size())
	}
	if c.NegativeSize() != 0 {
		t.Errorf("negative cache should be empty, got %d", c.NegativeSize())
	}
}

func TestFlushExpired(t *testing.T) {
	c := NewCache()
	c.Set("live.com", dns.TypeA, makeMsg(3600))

	expKey := cacheKey("expired.com", dns.TypeA)
	c.mu.Lock()
	c.entries[expKey] = &cacheEntry{
		msg:     makeMsg(1),
		expires: time.Now().Add(-1 * time.Second),
		cached:  time.Now().Add(-2 * time.Second),
	}
	c.mu.Unlock()

	if c.Size() != 2 {
		t.Fatalf("expected 2 entries before flush, got %d", c.Size())
	}

	c.Flush("expired")

	if c.Size() != 1 {
		t.Errorf("live entry should survive expired flush, got %d", c.Size())
	}
}

func TestPrefetchTrigger(t *testing.T) {
	c := NewCache()
	cfg := cfgWithCache(true)

	fetched := make(chan string, 1)
	c.SetPrefetchFn(func(name string, qtype uint16) (*dns.Message, error) {
		fetched <- name
		return makeMsg(300), nil
	})

	key := cacheKey("prefetch.com", dns.TypeA)
	now := time.Now()
	c.mu.Lock()
	c.entries[key] = &cacheEntry{
		msg:     makeMsg(100),
		expires: now.Add(4 * time.Second),
		cached:  now.Add(-96 * time.Second),
	}
	c.mu.Unlock()

	got := c.Get("prefetch.com", dns.TypeA, cfg)
	if got == nil {
		t.Fatal("near-expired entry should still be returned")
	}

	select {
	case name := <-fetched:
		if name != "prefetch.com" {
			t.Errorf("prefetch called with %q, want prefetch.com", name)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("prefetch function not called within 500ms")
	}
}

func TestDecrementRRs(t *testing.T) {
	rrs := []dns.RR{{TTL: 100}, {TTL: 50}, {TTL: 5}}
	out := decrementRRs(rrs, 10)

	if out[0].TTL != 90 {
		t.Errorf("expected 90, got %d", out[0].TTL)
	}
	if out[1].TTL != 40 {
		t.Errorf("expected 40, got %d", out[1].TTL)
	}
	if out[2].TTL != 0 {
		t.Errorf("TTL below elapsed should floor to 0, got %d", out[2].TTL)
	}
}

func TestLowestTTL(t *testing.T) {
	msg := &dns.Message{
		Answers: []dns.RR{{TTL: 300}, {TTL: 60}, {TTL: 120}},
	}
	if lowestTTL(msg) != 60 {
		t.Errorf("expected 60, got %d", lowestTTL(msg))
	}
}

func TestLowestTTLNoAnswers(t *testing.T) {
	if lowestTTL(&dns.Message{}) != 0 {
		t.Errorf("expected 0 for empty answers")
	}
}

func TestCloneMessagePreservesAllSections(t *testing.T) {
	msg := &dns.Message{
		Answers:    []dns.RR{{TTL: 100}},
		Authority:  []dns.RR{{TTL: 200}},
		Additional: []dns.RR{{TTL: 300}},
	}
	clone := cloneMessageWithDecrementedTTL(msg, 10)

	if clone.Answers[0].TTL != 90 {
		t.Errorf("Answers TTL = %d, want 90", clone.Answers[0].TTL)
	}
	if clone.Authority[0].TTL != 190 {
		t.Errorf("Authority TTL = %d, want 190", clone.Authority[0].TTL)
	}
	if clone.Additional[0].TTL != 290 {
		t.Errorf("Additional TTL = %d, want 290", clone.Additional[0].TTL)
	}
}
