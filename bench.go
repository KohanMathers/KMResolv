//go:build ignore

// Run with: go run bench.go [flags]
//
// Phases:
//  1. Cold      — kmresolv with empty cache (also warms cache)
//  2. Upstream  — direct queries to upstream resolver
//  3. Warm      — kmresolv with populated cache
//  4. Burst     — high concurrency stress vs upstream
//
// Flags:
//   -kmresolv  address of kmresolv          (default 127.0.0.1:53)
//   -upstream  resolver to compare against  (default 1.1.1.1:53)
//   -clients   concurrent client goroutines (default 20)
//   -queries   queries per phase            (default 500)
//   -timeout   per-query timeout            (default 5s)
//   -stress    concurrent clients for burst (default 50)

package main

import (
	"flag"
	"fmt"
	"math"
	"math/rand"
	"net"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	typeA    uint16 = 1
	typeAAAA uint16 = 28
	typeMX   uint16 = 15
	typeTXT  uint16 = 16
)

type query struct {
	name  string
	qtype uint16
}

func buildQuery(name string, qtype uint16) []byte {
	id := uint16(rand.Intn(65534) + 1)
	b := []byte{
		byte(id >> 8), byte(id),
		0x01, 0x00,
		0x00, 0x01,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}
	for _, label := range strings.Split(strings.TrimSuffix(name, "."), ".") {
		b = append(b, byte(len(label)))
		b = append(b, label...)
	}
	b = append(b, 0x00)
	b = append(b, byte(qtype>>8), byte(qtype))
	b = append(b, 0x00, 0x01)
	return b
}

func dnsQuery(server, name string, qtype uint16, timeout time.Duration) (time.Duration, error) {
	t0 := time.Now()
	conn, err := net.DialTimeout("udp", server, timeout)
	if err != nil {
		return time.Since(t0), fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(timeout))

	if _, err := conn.Write(buildQuery(name, qtype)); err != nil {
		return time.Since(t0), fmt.Errorf("write: %w", err)
	}

	var buf [512]byte
	n, err := conn.Read(buf[:])
	elapsed := time.Since(t0)
	if err != nil {
		return elapsed, fmt.Errorf("read: %w", err)
	}
	if n < 4 {
		return elapsed, fmt.Errorf("response too short (%d bytes)", n)
	}
	rcode := buf[3] & 0x0F
	if rcode != 0 && rcode != 3 {
		return elapsed, fmt.Errorf("rcode=%d", rcode)
	}
	return elapsed, nil
}

var benchQueries = []query{
	{"google.com", typeA},
	{"cloudflare.com", typeA},
	{"github.com", typeA},
	{"youtube.com", typeA},
	{"reddit.com", typeA},
	{"amazon.com", typeA},
	{"apple.com", typeA},
	{"microsoft.com", typeA},
	{"netflix.com", typeA},
	{"wikipedia.org", typeA},
	{"instagram.com", typeA},
	{"linkedin.com", typeA},
	{"stackoverflow.com", typeA},
	{"golang.org", typeA},
	{"python.org", typeA},
	{"npmjs.com", typeA},
	{"docker.com", typeA},
	{"kubernetes.io", typeA},
	{"aws.amazon.com", typeA},
	{"icloud.com", typeA},
	{"spotify.com", typeA},
	{"discord.com", typeA},
	{"slack.com", typeA},
	{"zoom.us", typeA},
	{"fastly.com", typeA},
	{"akamai.com", typeA},
	{"cdn.jsdelivr.net", typeA},
	{"fonts.googleapis.com", typeA},
	{"api.github.com", typeA},
	{"raw.githubusercontent.com", typeA},
	{"google.com", typeAAAA},
	{"cloudflare.com", typeAAAA},
	{"netflix.com", typeAAAA},
	{"facebook.com", typeAAAA},
	{"twitter.com", typeAAAA},
	{"gmail.com", typeMX},
	{"yahoo.com", typeMX},
	{"outlook.com", typeMX},
	{"protonmail.com", typeMX},
	{"fastmail.com", typeMX},
	{"google.com", typeTXT},
	{"github.com", typeTXT},
	{"cloudflare.com", typeTXT},
}

type result struct {
	lat time.Duration
	err error
}

type phaseStats struct {
	label   string
	server  string
	clients int
	total   int
	success int
	errors  int
	elapsed time.Duration
	lats    []time.Duration
}

func (s *phaseStats) throughput() float64 {
	if s.elapsed == 0 {
		return 0
	}
	return float64(s.total) / s.elapsed.Seconds()
}

func (s *phaseStats) pct(p float64) time.Duration {
	n := len(s.lats)
	if n == 0 {
		return 0
	}
	idx := int(math.Ceil(p/100.0*float64(n))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= n {
		idx = n - 1
	}
	return s.lats[idx]
}

func runPhase(label, server string, qs []query, clients, total int, timeout time.Duration) phaseStats {
	work := make(chan query, total)
	for i := range total {
		work <- qs[i%len(qs)]
	}
	close(work)

	results := make([]result, total)
	var counter atomic.Int64

	stopProgress := make(chan struct{})
	var progressWg sync.WaitGroup
	progressWg.Add(1)
	go func() {
		defer progressWg.Done()
		tick := time.NewTicker(80 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-tick.C:
				n := counter.Load()
				pct := int(n * 40 / int64(total))
				if pct > 40 {
					pct = 40
				}
				fmt.Printf("\r  [%s%s] %d/%d",
					strings.Repeat("█", pct),
					strings.Repeat("░", 40-pct),
					n, total)
			case <-stopProgress:
				return
			}
		}
	}()

	started := time.Now()
	var wg sync.WaitGroup
	for range clients {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for q := range work {
				i := int(counter.Add(1)) - 1
				lat, err := dnsQuery(server, q.name, q.qtype, timeout)
				results[i] = result{lat: lat, err: err}
			}
		}()
	}
	wg.Wait()
	elapsed := time.Since(started)

	close(stopProgress)
	progressWg.Wait()
	fmt.Printf("\r  [%s] %d/%d\n", strings.Repeat("█", 40), total, total)

	s := phaseStats{label: label, server: server, clients: clients, total: total, elapsed: elapsed}
	for _, r := range results {
		if r.err != nil {
			s.errors++
		} else {
			s.success++
			s.lats = append(s.lats, r.lat)
		}
	}
	sort.Slice(s.lats, func(i, j int) bool { return s.lats[i] < s.lats[j] })
	return s
}

func fmtDur(d time.Duration) string {
	if d == 0 {
		return "---"
	}
	if d < time.Millisecond {
		return fmt.Sprintf("%d µs", d.Microseconds())
	}
	return fmt.Sprintf("%.1f ms", float64(d.Microseconds())/1000)
}

func fmtThroughput(qps float64) string {
	if qps >= 1000 {
		return fmt.Sprintf("%.1f k/s", qps/1000)
	}
	return fmt.Sprintf("%.0f q/s", qps)
}

func printTable(title string, cols []phaseStats) {
	const labelW = 22

	colW := 16
	for _, c := range cols {
		if len(c.label) > colW {
			colW = len(c.label)
		}
	}

	sep := func(l, mid, r, h string) string {
		s := l + strings.Repeat(h, labelW+2)
		for range cols {
			s += mid + strings.Repeat(h, colW+2)
		}
		return s + r
	}

	cell := func(s string, width int) string {
		extra := strings.Count(s, "µ")
		return fmt.Sprintf("%*s", width+extra, s)
	}

	row := func(label string, vals []string) {
		line := fmt.Sprintf("│ %-*s", labelW, label)
		for _, v := range vals {
			line += " │ " + cell(v, colW)
		}
		fmt.Println(line + " │")
	}

	divider := func() { fmt.Println(sep("├", "┼", "┤", "─")) }

	fmt.Println()
	fmt.Println(title)
	fmt.Println(sep("┌", "┬", "┐", "─"))

	hdr := fmt.Sprintf("│ %-*s", labelW, "")
	for _, c := range cols {
		hdr += fmt.Sprintf(" │ %-*s", colW, c.label)
	}
	fmt.Println(hdr + " │")
	fmt.Println(sep("├", "┼", "┤", "─"))

	vals := make([]string, len(cols))

	for i, c := range cols {
		vals[i] = fmt.Sprintf("%d", c.total)
	}
	row("Queries", vals)
	for i, c := range cols {
		vals[i] = fmt.Sprintf("%d", c.success)
	}
	row("Successful", vals)
	for i, c := range cols {
		vals[i] = fmt.Sprintf("%d", c.errors)
	}
	row("Errors", vals)
	for i, c := range cols {
		vals[i] = fmtThroughput(c.throughput())
	}
	row("Throughput", vals)

	divider()

	for i, c := range cols {
		vals[i] = fmtDur(c.pct(0))
	}
	row("Min latency", vals)
	for i, c := range cols {
		vals[i] = fmtDur(c.pct(50))
	}
	row("P50 latency", vals)
	for i, c := range cols {
		vals[i] = fmtDur(c.pct(95))
	}
	row("P95 latency", vals)
	for i, c := range cols {
		vals[i] = fmtDur(c.pct(99))
	}
	row("P99 latency", vals)
	for i, c := range cols {
		vals[i] = fmtDur(c.pct(100))
	}
	row("Max latency", vals)

	fmt.Println(sep("└", "┴", "┘", "─"))
}

func main() {
	kmAddr := flag.String("kmresolv", "127.0.0.1:53", "kmresolv address")
	upAddr := flag.String("upstream", "1.1.1.1:53", "upstream resolver to compare against")
	clients := flag.Int("clients", 20, "concurrent client goroutines per phase")
	queries := flag.Int("queries", 500, "queries per phase")
	timeout := flag.Duration("timeout", 5*time.Second, "per-query timeout")
	stress := flag.Int("stress", 50, "concurrent clients for burst stress phase")
	flag.Parse()

	stressQueries := *stress * (*queries / *clients)
	if stressQueries < *queries {
		stressQueries = *queries
	}

	fmt.Printf("\nkmresolv Benchmark\n")
	fmt.Printf("==================\n")
	fmt.Printf("kmresolv : %s\n", *kmAddr)
	fmt.Printf("Upstream : %s\n", *upAddr)
	fmt.Printf("Clients  : %d  |  Queries/phase : %d  |  Timeout : %s\n",
		*clients, *queries, *timeout)
	fmt.Printf("Domains  : %d  |  Types : A, AAAA, MX, TXT\n\n",
		len(benchQueries))

	fmt.Printf("Phase 1/4 — Cold: kmresolv with empty cache (also warms cache for later phases)\n")
	kmCold := runPhase("kmresolv (cold)", *kmAddr, benchQueries, *clients, *queries, *timeout)

	fmt.Printf("\nPhase 2/4 — Repeated domains via %s\n", *upAddr)
	upRepeated := runPhase(*upAddr, *upAddr, benchQueries, *clients, *queries, *timeout)

	fmt.Printf("\nPhase 3/4 — Repeated domains via kmresolv (warm cache)\n")
	kmRepeated := runPhase("kmresolv (warm)", *kmAddr, benchQueries, *clients, *queries, *timeout)

	fmt.Printf("\nPhase 4/4 — Burst stress: %d clients, %d queries\n", *stress, stressQueries)
	fmt.Printf("  %s:\n", *upAddr)
	upBurst := runPhase(*upAddr+" (burst)", *upAddr, benchQueries, *stress, stressQueries, *timeout)
	fmt.Printf("  kmresolv:\n")
	kmBurst := runPhase("kmresolv (burst)", *kmAddr, benchQueries, *stress, stressQueries, *timeout)

	printTable("Cold vs Warm  (kmresolv cache miss → hit vs upstream)", []phaseStats{kmCold, upRepeated, kmRepeated})
	printTable("Burst Stress Test  ("+fmt.Sprintf("%d", *stress)+" concurrent clients)", []phaseStats{upBurst, kmBurst})

	fmt.Println()
	if p50cold := kmCold.pct(50); p50cold > 0 {
		if p50warm := kmRepeated.pct(50); p50warm > 0 {
			fmt.Printf("  Cold → warm speedup (kmresolv cache effect):\n")
			fmt.Printf("    P50  %6.1fx faster   (%s → %s)\n",
				float64(p50cold)/float64(p50warm),
				fmtDur(p50cold), fmtDur(p50warm))
			fmt.Printf("    P99  %6.1fx faster   (%s → %s)\n",
				float64(kmCold.pct(99))/float64(kmRepeated.pct(99)),
				fmtDur(kmCold.pct(99)), fmtDur(kmRepeated.pct(99)))
			fmt.Printf("    Throughput  %6.1fx higher  (%s → %s)\n",
				kmRepeated.throughput()/kmCold.throughput(),
				fmtThroughput(kmCold.throughput()), fmtThroughput(kmRepeated.throughput()))
		}
	}
	if p50up := upRepeated.pct(50); p50up > 0 {
		if p50km := kmRepeated.pct(50); p50km > 0 {
			fmt.Printf("\n  Warm-cache speedup vs %s:\n", *upAddr)
			fmt.Printf("    P50  %6.1fx faster   (%s → %s)\n",
				float64(p50up)/float64(p50km),
				fmtDur(p50up), fmtDur(p50km))
			fmt.Printf("    P99  %6.1fx faster   (%s → %s)\n",
				float64(upRepeated.pct(99))/float64(kmRepeated.pct(99)),
				fmtDur(upRepeated.pct(99)), fmtDur(kmRepeated.pct(99)))
			fmt.Printf("    Throughput  %6.1fx higher  (%s → %s)\n",
				kmRepeated.throughput()/upRepeated.throughput(),
				fmtThroughput(upRepeated.throughput()), fmtThroughput(kmRepeated.throughput()))
		}
	}
	if p50up := upBurst.pct(50); p50up > 0 {
		if p50km := kmBurst.pct(50); p50km > 0 {
			fmt.Printf("\n  Burst-stress speedup vs %s:\n", *upAddr)
			fmt.Printf("    P50  %6.1fx faster   (%s → %s)\n",
				float64(p50up)/float64(p50km),
				fmtDur(p50up), fmtDur(p50km))
			fmt.Printf("    P99  %6.1fx faster   (%s → %s)\n",
				float64(upBurst.pct(99))/float64(kmBurst.pct(99)),
				fmtDur(upBurst.pct(99)), fmtDur(kmBurst.pct(99)))
			fmt.Printf("    Throughput  %6.1fx higher  (%s → %s)\n",
				kmBurst.throughput()/upBurst.throughput(),
				fmtThroughput(upBurst.throughput()), fmtThroughput(kmBurst.throughput()))
		}
	}
	fmt.Println()

	if kmRepeated.errors > 0 || kmBurst.errors > 0 {
		fmt.Fprintln(os.Stderr, "  Note: kmresolv had errors — check it is running and listening on", *kmAddr)
	}
}
