package filter

import (
	"bufio"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/kohanmathers/kmresolv/internal/config"
	"github.com/kohanmathers/kmresolv/internal/logger"
)

type Filter struct {
	mu      sync.RWMutex
	domains map[string]bool
	inline  map[string]bool
	mode    string
}

func NewFilter(cfg *config.Config) *Filter {
	f := &Filter{
		domains: make(map[string]bool),
		inline:  make(map[string]bool),
		mode:    strings.ToLower(cfg.Filtering.Mode),
	}
	if f.mode == "off" {
		return f
	}

	for _, d := range cfg.Filtering.Inline {
		d = strings.ToLower(strings.TrimSpace(d))
		f.domains[d] = true
		f.inline[d] = true
	}

	for _, source := range cfg.Filtering.Lists {
		if err := f.loadList(source); err != nil {
			logger.LogWarn("failed to load filter list %s: %v", source, err)
		}
	}

	logger.LogInfo("filter loaded: mode=%s domains=%d", f.mode, len(f.domains))
	return f
}

func (f *Filter) Blocked(name string) bool {
	if f.mode == "off" {
		return false
	}
	name = strings.ToLower(strings.TrimSuffix(name, "."))

	f.mu.RLock()
	defer f.mu.RUnlock()

	for {
		if f.domains[name] {
			if f.mode == "blacklist" {
				return true
			}
			return false
		}
		idx := strings.Index(name, ".")
		if idx == -1 {
			break
		}
		name = name[idx+1:]
	}

	if f.mode == "whitelist" {
		return true
	}
	return false
}

func (f *Filter) Add(domain string) {
	d := strings.ToLower(strings.TrimSpace(domain))
	f.mu.Lock()
	f.domains[d] = true
	f.inline[d] = true
	f.mu.Unlock()
}

func (f *Filter) Remove(domain string) {
	d := strings.ToLower(strings.TrimSpace(domain))
	f.mu.Lock()
	delete(f.domains, d)
	delete(f.inline, d)
	f.mu.Unlock()
}

func (f *Filter) InlineDomains() []string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := make([]string, 0, len(f.inline))
	for d := range f.inline {
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

func (f *Filter) SetMode(mode string) {
	f.mu.Lock()
	f.mode = strings.ToLower(mode)
	f.mu.Unlock()
}

func (f *Filter) Size() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.domains)
}

func (f *Filter) loadList(source string) error {
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		return f.loadURL(source)
	}
	return f.loadFile(source)
}

func (f *Filter) loadURL(url string) error {
	logger.LogInfo("fetching filter list: %s", url)
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	f.parseList(bufio.NewScanner(resp.Body))
	return nil
}

func (f *Filter) loadFile(path string) error {
	logger.LogInfo("loading filter list: %s", path)
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	f.parseList(bufio.NewScanner(file))
	return nil
}

func (f *Filter) parseList(scanner *bufio.Scanner) {
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if idx := strings.Index(line, "#"); idx != -1 {
			line = strings.TrimSpace(line[:idx])
		}
		fields := strings.Fields(line)
		switch len(fields) {
		case 1:
			f.domains[strings.ToLower(fields[0])] = true
		case 2:
			domain := strings.ToLower(fields[1])
			if domain != "localhost" && domain != "localhost.localdomain" {
				f.domains[domain] = true
			}
		}
	}
}
