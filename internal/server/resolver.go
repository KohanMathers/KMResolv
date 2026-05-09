package server

import (
	"encoding/binary"
	"fmt"
	"io"
	"math/rand"
	"net"
	"strings"
	"time"

	"github.com/kohanmathers/kmresolv/internal/dns"
	"github.com/kohanmathers/kmresolv/internal/logger"
)

// Last updated: 7th May 2026 14:14 UTC
var RootServers = []string{
	"198.41.0.4",
	"170.247.170.2",
	"192.33.4.12",
	"199.7.91.13",
	"192.203.230.10",
	"192.5.5.241",
	"192.112.36.4",
	"198.97.190.53",
	"192.36.148.17",
	"192.58.128.30",
	"193.0.14.129",
	"199.7.83.42",
	"202.12.27.33",
}

const udpBufSize = 4096

func (s *Server) resolve(name string, qtype uint16) (*dns.Message, error) {
	if s.cache.IsNegative(name, qtype) {
		logger.LogDebug("negative cache hit: %s", name)
		return nil, fmt.Errorf("NXDOMAIN: %s does not exist", name)
	}
	if msg := s.cache.Get(name, qtype, s.cfg); msg != nil {
		logger.LogDebug("cache hit: %s", name)
		return msg, nil
	}
	msg, err := s.resolveAt(name, qtype, RootServers, 0)
	if err != nil {
		if strings.Contains(err.Error(), "NXDOMAIN") {
			s.cache.SetNegative(name, qtype, s.cfg.Resolver.Cache.NegativeTTL)
		}
		return nil, err
	}
	s.cache.Set(name, qtype, msg)
	return msg, nil
}

func (s *Server) resolveAt(name string, qtype uint16, servers []string, depth int) (*dns.Message, error) {
	if depth > s.cfg.Resolver.MaxDepth {
		return nil, fmt.Errorf("max referral depth exceeded resolving %s", name)
	}

	shuffled := make([]string, len(servers))
	copy(shuffled, servers)
	rand.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })

	var lastErr error
	for _, sv := range shuffled {
		server := sv + ":53"
		resp, err := s.query(server, name, qtype)
		if err != nil {
			logger.LogDebug("server %s failed for %s: %v — trying next", server, name, err)
			lastErr = err
			continue
		}

		if len(resp.Answers) > 0 {
			if qtype != dns.TypeCNAME {
				for _, rr := range resp.Answers {
					if rr.Type == dns.TypeCNAME {
						target, err := parseCNAME(resp, rr)
						if err != nil {
							return nil, fmt.Errorf("cname parse: %w", err)
						}
						logger.LogDebug("following CNAME %s → %s", name, target)
						return s.resolveAt(target, qtype, RootServers, depth+1)
					}
				}
			}
			return resp, nil
		}

		nsNames := extractNS(resp)
		if len(nsNames) == 0 {
			lastErr = fmt.Errorf("no answers and no referral from %s for %s", server, name)
			continue
		}

		logger.LogDebug("referral from %s → %v (depth %d)", server, nsNames, depth)

		glue := extractGlue(resp, nsNames)
		if len(glue) > 0 {
			return s.resolveAt(name, qtype, glue, depth+1)
		}

		nsIPs, err := s.resolveNSParallel(nsNames, depth)
		if err != nil {
			lastErr = err
			continue
		}
		return s.resolveAt(name, qtype, nsIPs, depth+1)
	}

	return nil, fmt.Errorf("all servers failed for %s: %w", name, lastErr)
}

func (s *Server) buildQuery(name string, qtype uint16) *dns.Message {
	req := &dns.Message{}
	req.ID = uint16(rand.Intn(65535))
	req.SetRD(false)
	req.Questions = []dns.Question{{Name: name, Type: qtype, Class: dns.ClassIN}}

	if s.cfg.Resolver.EDNS0 {
		req.Additional = append(req.Additional, dns.RR{
			Name:  "",
			Type:  41,
			Class: 4096,
			TTL:   0,
			Data:  []byte{},
		})
	}
	return req
}

func (s *Server) query(server, name string, qtype uint16) (*dns.Message, error) {
	timeout := time.Duration(s.cfg.Resolver.Timeout) * time.Second
	conn, err := net.DialTimeout("udp", server, timeout)
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(timeout))

	req := s.buildQuery(name, qtype)
	packed, err := req.Pack()
	if err != nil {
		return nil, fmt.Errorf("pack: %w", err)
	}
	if _, err := conn.Write(packed); err != nil {
		return nil, fmt.Errorf("write: %w", err)
	}

	buf := make([]byte, udpBufSize)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}

	resp, err := dns.ParseMessage(buf[:n])
	if err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if resp.ID != req.ID {
		return nil, fmt.Errorf("response ID mismatch: got %d want %d", resp.ID, req.ID)
	}
	if resp.Rcode() == dns.RcodeNXDomain {
		return nil, fmt.Errorf("NXDOMAIN: %s does not exist", name)
	}
	if resp.Rcode() != dns.RcodeNoError {
		return nil, fmt.Errorf("rcode %d from %s", resp.Rcode(), server)
	}
	if resp.Flags&0x0200 != 0 && s.cfg.Resolver.TCPFallback {
		logger.LogDebug("response truncated, retrying over TCP: %s", server)
		return s.queryTCP(server, name, qtype)
	}

	return resp, nil
}

func (s *Server) queryTCP(server, name string, qtype uint16) (*dns.Message, error) {
	timeout := time.Duration(s.cfg.Resolver.Timeout) * time.Second
	conn, err := net.DialTimeout("tcp", server, timeout)
	if err != nil {
		return nil, fmt.Errorf("tcp dial: %w", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(timeout))

	req := s.buildQuery(name, qtype)
	packed, err := req.Pack()
	if err != nil {
		return nil, err
	}

	length := make([]byte, 2)
	binary.BigEndian.PutUint16(length, uint16(len(packed)))
	if _, err := conn.Write(append(length, packed...)); err != nil {
		return nil, fmt.Errorf("tcp write: %w", err)
	}

	if _, err := io.ReadFull(conn, length); err != nil {
		return nil, fmt.Errorf("tcp read length: %w", err)
	}
	msgLen := int(binary.BigEndian.Uint16(length))
	buf := make([]byte, msgLen)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return nil, fmt.Errorf("tcp read body: %w", err)
	}

	return dns.ParseMessage(buf)
}

func extractNS(m *dns.Message) []string {
	var names []string
	for _, rr := range m.Authority {
		if rr.Type == dns.TypeNS {
			name, _, err := dns.ParseName(m.Raw, rr.Offset)
			if err == nil {
				names = append(names, name)
			}
		}
	}
	return names
}

func extractGlue(m *dns.Message, nsNames []string) []string {
	nsSet := make(map[string]bool)
	for _, n := range nsNames {
		nsSet[n] = true
	}
	var ips []string
	for _, rr := range m.Additional {
		if rr.Type == dns.TypeA && nsSet[rr.Name] {
			ip, err := dns.ParseA(rr.Data)
			if err == nil {
				ips = append(ips, ip)
			}
		}
	}
	return ips
}

func parseCNAME(m *dns.Message, rr dns.RR) (string, error) {
	name, _, err := dns.ParseName(m.Raw, rr.Offset)
	return name, err
}

func (s *Server) resolveNSParallel(nsNames []string, depth int) ([]string, error) {
	type result struct {
		ips []string
		err error
	}

	results := make(chan result, len(nsNames))

	for _, ns := range nsNames {
		ns := ns
		go func() {
			nsResp, err := s.resolveAt(ns, dns.TypeA, RootServers, depth+1)
			if err != nil {
				logger.LogDebug("parallel NS resolve failed for %s: %v", ns, err)
				results <- result{err: err}
				return
			}
			var ips []string
			for _, rr := range nsResp.Answers {
				if rr.Type == dns.TypeA {
					ip, err := dns.ParseA(rr.Data)
					if err == nil {
						ips = append(ips, ip)
					}
				}
			}
			results <- result{ips: ips}
		}()
	}

	var allIPs []string
	var lastErr error
	for range nsNames {
		r := <-results
		if r.err != nil {
			lastErr = r.err
			continue
		}
		allIPs = append(allIPs, r.ips...)
	}

	if len(allIPs) == 0 {
		if lastErr != nil {
			return nil, fmt.Errorf("all NS resolutions failed: %w", lastErr)
		}
		return nil, fmt.Errorf("no IPs found for any NS")
	}

	return allIPs, nil
}
