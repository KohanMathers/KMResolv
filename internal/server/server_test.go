package server

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/kohanmathers/kmresolv/internal/config"
	"github.com/kohanmathers/kmresolv/internal/dns"
)

// fakeConn satisfies net.PacketConn for handleQuery tests.
type fakeConn struct {
	mu      sync.Mutex
	written []byte
}

func (f *fakeConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make([]byte, len(b))
	copy(cp, b)
	f.written = cp
	return len(b), nil
}

func (f *fakeConn) lastWritten() []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.written
}

func (f *fakeConn) ReadFrom(b []byte) (int, net.Addr, error) { return 0, nil, errors.New("not used") }
func (f *fakeConn) Close() error                             { return nil }
func (f *fakeConn) LocalAddr() net.Addr                      { return &net.UDPAddr{} }
func (f *fakeConn) SetDeadline(t time.Time) error            { return nil }
func (f *fakeConn) SetReadDeadline(t time.Time) error        { return nil }
func (f *fakeConn) SetWriteDeadline(t time.Time) error       { return nil }

func minCfg() *config.Config {
	return &config.Config{
		Resolver: config.ResolverConfig{
			Timeout:  3,
			MaxDepth: 10,
			Cache:    config.CacheConfig{NegativeTTL: 300},
		},
		Filtering: config.FilterConfig{Mode: "off"},
	}
}

func packQuery(t *testing.T, name string, qtype uint16) []byte {
	t.Helper()
	req := &dns.Message{}
	req.ID = 0x1234
	req.Questions = []dns.Question{{Name: name, Type: qtype, Class: dns.ClassIN}}
	raw, err := req.Pack()
	if err != nil {
		t.Fatalf("pack query: %v", err)
	}
	return raw
}

// ---- typeName ----

func TestTypeName(t *testing.T) {
	cases := []struct {
		t    uint16
		want string
	}{
		{dns.TypeA, "A"},
		{dns.TypeAAAA, "AAAA"},
		{dns.TypeNS, "NS"},
		{dns.TypeCNAME, "CNAME"},
		{dns.TypeMX, "MX"},
		{dns.TypeTXT, "TXT"},
		{dns.TypeSOA, "SOA"},
		{99, "99"},
	}
	for _, c := range cases {
		if got := typeName(c.t); got != c.want {
			t.Errorf("typeName(%d) = %q, want %q", c.t, got, c.want)
		}
	}
}

// ---- nxdomain / servfail ----

func TestNXDomain(t *testing.T) {
	req := &dns.Message{}
	req.ID = 0xBEEF
	req.Questions = []dns.Question{{Name: "nxd.com", Type: dns.TypeA, Class: dns.ClassIN}}

	packed := nxdomain(req)
	if len(packed) == 0 {
		t.Fatal("nxdomain returned empty bytes")
	}

	resp, err := dns.ParseMessage(packed)
	if err != nil {
		t.Fatalf("parse nxdomain response: %v", err)
	}
	if resp.ID != 0xBEEF {
		t.Errorf("ID = %#x, want 0xBEEF", resp.ID)
	}
	if !resp.QR() {
		t.Error("QR bit should be set")
	}
	if resp.Rcode() != dns.RcodeNXDomain {
		t.Errorf("rcode = %d, want NXDomain (%d)", resp.Rcode(), dns.RcodeNXDomain)
	}
}

func TestServFail(t *testing.T) {
	req := &dns.Message{}
	req.ID = 0xABCD
	req.Questions = []dns.Question{{Name: "fail.com", Type: dns.TypeA, Class: dns.ClassIN}}

	resp := servfail(req)
	if resp.ID != 0xABCD {
		t.Errorf("ID = %#x, want 0xABCD", resp.ID)
	}
	if !resp.QR() {
		t.Error("QR bit should be set")
	}
	if resp.Rcode() != dns.RcodeServFail {
		t.Errorf("rcode = %d, want ServFail (%d)", resp.Rcode(), dns.RcodeServFail)
	}
}

// ---- QueryLog ----

func TestQueryLogAddRecent(t *testing.T) {
	ql := &QueryLog{max: 10}

	for i := 0; i < 3; i++ {
		ql.Add(QueryEntry{Domain: fmt.Sprintf("d%d.com", i)})
	}

	recent := ql.Recent(10)
	if len(recent) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(recent))
	}
}

func TestQueryLogMaxCapacity(t *testing.T) {
	ql := &QueryLog{max: 3}

	for i := 0; i < 5; i++ {
		ql.Add(QueryEntry{Domain: fmt.Sprintf("d%d.com", i)})
	}

	all := ql.Recent(100)
	if len(all) != 3 {
		t.Fatalf("expected max 3 entries, got %d", len(all))
	}
	// Oldest kept should be d2 (0-indexed)
	if all[0].Domain != "d2.com" {
		t.Errorf("expected first kept entry d2.com, got %s", all[0].Domain)
	}
	if all[2].Domain != "d4.com" {
		t.Errorf("expected last entry d4.com, got %s", all[2].Domain)
	}
}

func TestQueryLogRecentN(t *testing.T) {
	ql := &QueryLog{max: 100}

	for i := 0; i < 10; i++ {
		ql.Add(QueryEntry{Domain: fmt.Sprintf("d%d.com", i)})
	}

	recent := ql.Recent(3)
	if len(recent) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(recent))
	}
	if recent[2].Domain != "d9.com" {
		t.Errorf("expected last entry d9.com, got %s", recent[2].Domain)
	}
}

func TestQueryLogEmpty(t *testing.T) {
	ql := &QueryLog{max: 10}
	if got := ql.Recent(5); len(got) != 0 {
		t.Errorf("expected 0 from empty log, got %d", len(got))
	}
}

// ---- Server construction & stats ----

func TestNewServer(t *testing.T) {
	s := New(minCfg())
	if s == nil {
		t.Fatal("New returned nil")
	}
	st := s.Stats()
	if st.TotalQueries != 0 || st.CacheHits != 0 || st.Blocked != 0 {
		t.Errorf("initial stats should be zero: %+v", st)
	}
	if st.UptimeSeconds < 0 {
		t.Error("uptime should be non-negative")
	}
}

func TestGetSettings(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{Listen: "127.0.0.1", Port: 5353, LogLevel: "debug"},
		Resolver: config.ResolverConfig{
			Timeout:     5,
			MaxDepth:    8,
			EDNS0:       true,
			TCPFallback: false,
			Cache:       config.CacheConfig{NegativeTTL: 60, Prefetch: false},
		},
		Filtering: config.FilterConfig{Mode: "off"},
		Updater:   config.UpdaterConfig{CheckEnabled: false},
	}

	s := New(cfg)
	ss := s.GetSettings()

	if ss.Timeout != 5 {
		t.Errorf("Timeout = %d, want 5", ss.Timeout)
	}
	if ss.MaxDepth != 8 {
		t.Errorf("MaxDepth = %d, want 8", ss.MaxDepth)
	}
	if !ss.EDNS0 {
		t.Error("EDNS0 should be true")
	}
	if ss.TCPFallback {
		t.Error("TCPFallback should be false")
	}
	if ss.NegativeTTL != 60 {
		t.Errorf("NegativeTTL = %d, want 60", ss.NegativeTTL)
	}
	if ss.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want debug", ss.LogLevel)
	}
	if ss.UpdateCheck {
		t.Error("UpdateCheck should be false")
	}
}

func TestFlushCacheClearsStats(t *testing.T) {
	s := New(minCfg())
	s.FlushCache("")
	st := s.Stats()
	if st.CacheSize != 0 {
		t.Errorf("cache size should be 0 after full flush, got %d", st.CacheSize)
	}
}

func TestFilterStatus(t *testing.T) {
	cfg := &config.Config{
		Filtering: config.FilterConfig{
			Mode:   "blacklist",
			Inline: []string{"a.com", "b.com"},
		},
	}
	s := New(cfg)
	fs := s.FilterStatus()

	if fs.Mode != "blacklist" {
		t.Errorf("mode = %q, want blacklist", fs.Mode)
	}
	if fs.Size != 2 {
		t.Errorf("size = %d, want 2", fs.Size)
	}
}

func TestConfigRecords(t *testing.T) {
	cfg := &config.Config{
		Filtering: config.FilterConfig{Mode: "off"},
		Records: []config.RecordConfig{
			{Name: "host.com", Type: "A", TTL: 60, Value: "1.2.3.4"},
		},
	}
	s := New(cfg)
	recs := s.ConfigRecords()
	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
	if recs[0].Name != "host.com" {
		t.Errorf("record name = %q, want host.com", recs[0].Name)
	}
}

func TestCfg(t *testing.T) {
	cfg := minCfg()
	s := New(cfg)
	if s.Cfg() != cfg {
		t.Error("Cfg() should return the same config pointer passed to New")
	}
}

// ---- handleQuery ----

func TestHandleQueryBlocked(t *testing.T) {
	cfg := &config.Config{
		Resolver: config.ResolverConfig{
			Timeout:  3,
			MaxDepth: 10,
			Cache:    config.CacheConfig{NegativeTTL: 300},
		},
		Filtering: config.FilterConfig{
			Mode:   "blacklist",
			Inline: []string{"blocked.com"},
		},
	}

	s := New(cfg)
	conn := &fakeConn{}
	src := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 9999}

	s.handleQuery(conn, src, packQuery(t, "blocked.com", dns.TypeA))

	written := conn.lastWritten()
	if len(written) == 0 {
		t.Fatal("no response written for blocked query")
	}

	resp, err := dns.ParseMessage(written)
	if err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if resp.Rcode() != dns.RcodeNXDomain {
		t.Errorf("blocked query should get NXDOMAIN, got rcode %d", resp.Rcode())
	}

	st := s.Stats()
	if st.TotalQueries != 1 {
		t.Errorf("TotalQueries = %d, want 1", st.TotalQueries)
	}
	if st.Blocked != 1 {
		t.Errorf("Blocked = %d, want 1", st.Blocked)
	}
}

func TestHandleQueryCustomRecord(t *testing.T) {
	cfg := &config.Config{
		Resolver:  config.ResolverConfig{Timeout: 3, MaxDepth: 10, Cache: config.CacheConfig{NegativeTTL: 300}},
		Filtering: config.FilterConfig{Mode: "off"},
		Records: []config.RecordConfig{
			{Name: "custom.test", Type: "A", TTL: 60, Value: "9.8.7.6"},
		},
	}

	s := New(cfg)
	conn := &fakeConn{}
	src := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 9999}

	s.handleQuery(conn, src, packQuery(t, "custom.test", dns.TypeA))

	written := conn.lastWritten()
	if len(written) == 0 {
		t.Fatal("no response written for custom record query")
	}

	resp, err := dns.ParseMessage(written)
	if err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if resp.Rcode() != dns.RcodeNoError {
		t.Errorf("custom record query should get NOERROR, got rcode %d", resp.Rcode())
	}
	if !resp.QR() {
		t.Error("response should have QR bit set")
	}
	if len(resp.Answers) != 1 {
		t.Fatalf("expected 1 answer, got %d", len(resp.Answers))
	}

	st := s.Stats()
	if st.TotalQueries != 1 {
		t.Errorf("TotalQueries = %d, want 1", st.TotalQueries)
	}
	if st.Blocked != 0 {
		t.Errorf("Blocked should be 0 for custom record, got %d", st.Blocked)
	}
}

func TestHandleQueryInvalidMessage(t *testing.T) {
	s := New(minCfg())
	conn := &fakeConn{}
	src := &net.UDPAddr{}

	s.handleQuery(conn, src, []byte{0x00, 0x01}) // too short to parse

	if len(conn.lastWritten()) != 0 {
		t.Error("invalid message should produce no response")
	}
	if s.Stats().TotalQueries != 0 {
		t.Error("parse error should not increment total queries")
	}
}

func TestHandleQueryNoQuestions(t *testing.T) {
	s := New(minCfg())
	conn := &fakeConn{}
	src := &net.UDPAddr{}

	req := &dns.Message{}
	req.ID = 0x1234
	raw, _ := req.Pack()

	s.handleQuery(conn, src, raw)

	if len(conn.lastWritten()) != 0 {
		t.Error("query with no questions should produce no response")
	}
	if s.Stats().TotalQueries != 0 {
		t.Error("empty query should not increment total queries")
	}
}

func TestHandleQueryResponseID(t *testing.T) {
	cfg := &config.Config{
		Resolver:  config.ResolverConfig{Timeout: 3, MaxDepth: 10, Cache: config.CacheConfig{NegativeTTL: 300}},
		Filtering: config.FilterConfig{Mode: "blacklist", Inline: []string{"x.com"}},
	}

	s := New(cfg)
	conn := &fakeConn{}
	src := &net.UDPAddr{}

	req := &dns.Message{}
	req.ID = 0xDEAD
	req.Questions = []dns.Question{{Name: "x.com", Type: dns.TypeA, Class: dns.ClassIN}}
	raw, _ := req.Pack()

	s.handleQuery(conn, src, raw)

	resp, err := dns.ParseMessage(conn.lastWritten())
	if err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if resp.ID != 0xDEAD {
		t.Errorf("response ID = %#x, want 0xDEAD", resp.ID)
	}
}
