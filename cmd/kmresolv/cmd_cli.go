package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/kohanmathers/kmresolv/internal/config"
)

type cliConfig struct {
	host string
	port int
}

func (c *cliConfig) baseURL() string {
	return fmt.Sprintf("http://%s:%d", c.host, c.port)
}

func resolveCLIConfig(args []string) (*cliConfig, []string) {
	fs := flag.NewFlagSet("cli", flag.ExitOnError)
	configPath := fs.String("config", "config.yml", "path to config file")
	host := fs.String("host", "", "dashboard host")
	port := fs.Int("port", 0, "dashboard port")
	fs.Parse(args)

	cc := &cliConfig{host: "127.0.0.1", port: 8080}

	if cfg, err := config.LoadConfig(*configPath); err == nil {
		cc.host = cfg.Dashboard.Listen
		cc.port = cfg.Dashboard.Port
		if cc.host == "0.0.0.0" {
			cc.host = "127.0.0.1"
		}
	}

	if *host != "" {
		cc.host = *host
	}
	if *port != 0 {
		cc.port = *port
	}

	return cc, fs.Args()
}

func apiGet(cc *cliConfig, path string, out any) error {
	resp, err := http.Get(cc.baseURL() + path)
	if err != nil {
		return fmt.Errorf("could not reach kmresolv dashboard at %s — is the server running?", cc.baseURL())
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(out)
}

func apiPost(cc *cliConfig, path string, body any, out any) error {
	data, _ := json.Marshal(body)
	resp, err := http.Post(cc.baseURL()+path, "application/json", strings.NewReader(string(data)))
	if err != nil {
		return fmt.Errorf("could not reach kmresolv dashboard at %s — is the server running?", cc.baseURL())
	}
	defer resp.Body.Close()
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func cmdStatus(args []string) {
	cc, _ := resolveCLIConfig(args)

	var stats struct {
		TotalQueries  uint64  `json:"total_queries"`
		CacheHits     uint64  `json:"cache_hits"`
		Blocked       uint64  `json:"blocked"`
		HitRate       float64 `json:"hit_rate"`
		AvgLatencyMs  float64 `json:"avg_latency_ms"`
		UptimeSeconds int     `json:"uptime_seconds"`
		CacheSize     int     `json:"cache_size"`
		CacheNegative int     `json:"cache_negative"`
	}

	if err := apiGet(cc, "/api/stats", &stats); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	uptime := time.Duration(stats.UptimeSeconds) * time.Second
	h := int(uptime.Hours())
	m := int(uptime.Minutes()) % 60
	s := int(uptime.Seconds()) % 60

	fmt.Printf("\n  kmresolv status\n\n")
	fmt.Printf("  %-20s %s\n", "uptime", fmt.Sprintf("%02d:%02d:%02d", h, m, s))
	fmt.Printf("  %-20s %d\n", "total queries", stats.TotalQueries)
	fmt.Printf("  %-20s %.1f%%\n", "cache hit rate", stats.HitRate)
	fmt.Printf("  %-20s %d (pos) + %d (neg)\n", "cached entries", stats.CacheSize, stats.CacheNegative)
	fmt.Printf("  %-20s %d\n", "blocked", stats.Blocked)
	fmt.Printf("  %-20s %.1fms\n", "avg latency", stats.AvgLatencyMs)
	fmt.Println()
}

func cmdFlush(args []string) {
	fs := flag.NewFlagSet("flush", flag.ExitOnError)
	expired := fs.Bool("expired", false, "flush only expired entries")
	negative := fs.Bool("negative", false, "flush only negative (NXDOMAIN) entries")
	fs.Parse(args)

	cc, _ := resolveCLIConfig(fs.Args())

	mode := "all"
	if *expired {
		mode = "expired"
	}
	if *negative {
		mode = "negative"
	}

	var result map[string]any
	if err := apiPost(cc, "/api/cache/flush?mode="+mode, nil, &result); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("cache flushed (%s)\n", mode)
}

func cmdBlock(args []string) {
	cc, remaining := resolveCLIConfig(args)

	if len(remaining) == 0 {
		fmt.Fprintln(os.Stderr, "usage: kmresolv block <domain>")
		os.Exit(1)
	}

	domain := remaining[0]
	var result map[string]any
	if err := apiPost(cc, "/api/filter/add", map[string]string{"domain": domain}, &result); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("blocked: %s\n", domain)
}

func cmdUnblock(args []string) {
	cc, remaining := resolveCLIConfig(args)

	if len(remaining) == 0 {
		fmt.Fprintln(os.Stderr, "usage: kmresolv unblock <domain>")
		os.Exit(1)
	}

	domain := remaining[0]
	var result map[string]any
	if err := apiPost(cc, "/api/filter/remove", map[string]string{"domain": domain}, &result); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("unblocked: %s\n", domain)
}

func cmdLog(args []string) {
	fs := flag.NewFlagSet("log", flag.ExitOnError)
	n := fs.Int("n", 20, "number of entries to show")
	fs.Parse(args)

	cc, _ := resolveCLIConfig(fs.Args())

	var entries []struct {
		Time      time.Time `json:"time"`
		Domain    string    `json:"domain"`
		Type      string    `json:"type"`
		Client    string    `json:"client"`
		Status    string    `json:"status"`
		LatencyMs int64     `json:"latency_ms"`
	}

	if err := apiGet(cc, "/api/querylog", &entries); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	start := len(entries) - *n
	if start < 0 {
		start = 0
	}
	entries = entries[start:]

	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}

	statusColor := map[string]string{
		"resolved": "\033[32m",
		"cached":   "\033[34m",
		"blocked":  "\033[31m",
		"error":    "\033[33m",
	}
	reset := "\033[0m"

	fmt.Println()
	for _, e := range entries {
		color := statusColor[e.Status]
		if color == "" {
			color = ""
		}
		fmt.Printf("  %s  %-40s  %-6s  %s%-9s%s  %dms\n",
			e.Time.Format("15:04:05"),
			e.Domain,
			e.Type,
			color, e.Status, reset,
			e.LatencyMs,
		)
	}
	fmt.Println()
}
