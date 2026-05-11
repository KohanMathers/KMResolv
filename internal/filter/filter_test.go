package filter

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kohanmathers/kmresolv/internal/config"
)

func newFilter(mode string, inline []string) *Filter {
	return NewFilter(&config.Config{
		Filtering: config.FilterConfig{
			Mode:   mode,
			Inline: inline,
		},
	})
}

func TestFilterOff(t *testing.T) {
	f := newFilter("off", nil)
	if f.Blocked("anything.com") {
		t.Error("mode off should never block")
	}
}

func TestBlacklistBlocked(t *testing.T) {
	f := newFilter("blacklist", []string{"ads.com"})

	if !f.Blocked("ads.com") {
		t.Error("ads.com should be blocked")
	}
	if f.Blocked("safe.com") {
		t.Error("safe.com should not be blocked")
	}
}

func TestBlacklistSubdomain(t *testing.T) {
	f := newFilter("blacklist", []string{"ads.com"})

	if !f.Blocked("tracker.ads.com") {
		t.Error("subdomain of blocked domain should be blocked")
	}
}

func TestBlacklistCaseInsensitive(t *testing.T) {
	f := newFilter("blacklist", []string{"Ads.Com"})

	if !f.Blocked("ADS.COM") {
		t.Error("blocking should be case-insensitive")
	}
	if !f.Blocked("ads.com") {
		t.Error("lowercase variant should also be blocked")
	}
}

func TestBlacklistTrailingDot(t *testing.T) {
	f := newFilter("blacklist", []string{"ads.com"})

	if !f.Blocked("ads.com.") {
		t.Error("trailing dot should be stripped for comparison")
	}
}

func TestWhitelistAllowsListed(t *testing.T) {
	f := newFilter("whitelist", []string{"allowed.com"})

	if f.Blocked("allowed.com") {
		t.Error("listed domain should not be blocked in whitelist mode")
	}
	if f.Blocked("sub.allowed.com") {
		t.Error("subdomain of whitelisted domain should not be blocked")
	}
}

func TestWhitelistBlocksUnlisted(t *testing.T) {
	f := newFilter("whitelist", []string{"allowed.com"})

	if !f.Blocked("other.com") {
		t.Error("unlisted domain should be blocked in whitelist mode")
	}
}

func TestAddRemove(t *testing.T) {
	f := newFilter("blacklist", nil)

	f.Add("block-me.com")
	if !f.Blocked("block-me.com") {
		t.Error("added domain should be blocked")
	}

	f.Remove("block-me.com")
	if f.Blocked("block-me.com") {
		t.Error("removed domain should not be blocked")
	}
}

func TestAddNormalizesCase(t *testing.T) {
	f := newFilter("blacklist", nil)
	f.Add("UPPER.COM")

	if !f.Blocked("upper.com") {
		t.Error("Add should normalize domain to lowercase")
	}
}

func TestInlineDomainsSorted(t *testing.T) {
	f := newFilter("blacklist", []string{"b.com", "a.com", "c.com"})
	inline := f.InlineDomains()

	if len(inline) != 3 {
		t.Fatalf("expected 3 inline domains, got %d", len(inline))
	}
	if inline[0] != "a.com" || inline[1] != "b.com" || inline[2] != "c.com" {
		t.Errorf("inline domains should be sorted: got %v", inline)
	}
}

func TestInlineOnlyTracksInlineAdds(t *testing.T) {
	f := newFilter("blacklist", nil)
	f.Add("via-add.com")

	inline := f.InlineDomains()
	if len(inline) != 1 || inline[0] != "via-add.com" {
		t.Errorf("Add should register domain in inline list: %v", inline)
	}
}

func TestRemoveFromInline(t *testing.T) {
	f := newFilter("blacklist", []string{"a.com", "b.com"})
	f.Remove("a.com")

	inline := f.InlineDomains()
	if len(inline) != 1 || inline[0] != "b.com" {
		t.Errorf("Remove should delete from inline list: %v", inline)
	}
}

func TestSetMode(t *testing.T) {
	f := newFilter("off", nil)
	f.Add("example.com")

	if f.Blocked("example.com") {
		t.Error("should not block in off mode")
	}

	f.SetMode("blacklist")
	if !f.Blocked("example.com") {
		t.Error("should block after switching to blacklist mode")
	}

	f.SetMode("off")
	if f.Blocked("example.com") {
		t.Error("should not block after switching back to off mode")
	}
}

func TestSize(t *testing.T) {
	f := newFilter("blacklist", []string{"a.com", "b.com"})

	if f.Size() != 2 {
		t.Errorf("expected size 2, got %d", f.Size())
	}

	f.Add("c.com")
	if f.Size() != 3 {
		t.Errorf("expected size 3 after Add, got %d", f.Size())
	}

	f.Remove("a.com")
	if f.Size() != 2 {
		t.Errorf("expected size 2 after Remove, got %d", f.Size())
	}
}

func TestParseListSingleField(t *testing.T) {
	domains := make(map[string]bool)
	path := writeList(t, `# comment
ads.com
tracker.net
# another comment
malware.org
`)
	if err := loadFromFile(path, domains); err != nil {
		t.Fatalf("loadFromFile: %v", err)
	}
	for _, d := range []string{"ads.com", "tracker.net", "malware.org"} {
		if !domains[d] {
			t.Errorf("domain %q should be loaded from single-field list", d)
		}
	}
}

func TestParseListTwoField(t *testing.T) {
	domains := make(map[string]bool)
	path := writeList(t, `0.0.0.0 ads.com
127.0.0.1 localhost
0.0.0.0 tracker.net
0.0.0.0 localhost.localdomain
`)
	if err := loadFromFile(path, domains); err != nil {
		t.Fatalf("loadFromFile: %v", err)
	}
	if !domains["ads.com"] {
		t.Error("ads.com should be loaded from hosts-style list")
	}
	if !domains["tracker.net"] {
		t.Error("tracker.net should be loaded from hosts-style list")
	}
	if domains["localhost"] {
		t.Error("localhost should be filtered out")
	}
	if domains["localhost.localdomain"] {
		t.Error("localhost.localdomain should be filtered out")
	}
}

func TestParseListInlineComment(t *testing.T) {
	domains := make(map[string]bool)
	path := writeList(t, "ads.com # this is a comment\n")
	if err := loadFromFile(path, domains); err != nil {
		t.Fatalf("loadFromFile: %v", err)
	}
	if !domains["ads.com"] {
		t.Error("inline comment should be stripped, domain should be loaded")
	}
}

func TestParseListEmptyLines(t *testing.T) {
	domains := make(map[string]bool)
	path := writeList(t, "\n\n   \nads.com\n\n")
	if err := loadFromFile(path, domains); err != nil {
		t.Fatalf("loadFromFile: %v", err)
	}
	if !domains["ads.com"] {
		t.Error("empty lines should be skipped and valid domains loaded")
	}
	if len(domains) != 1 {
		t.Errorf("expected 1 domain, got %d", len(domains))
	}
}

func writeList(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "list.txt")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}
