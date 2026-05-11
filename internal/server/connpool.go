package server

import (
	"net"
	"sync"
	"time"
)

const poolSizePerServer = 16

type udpPool struct {
	mu    sync.Mutex
	conns map[string][]net.Conn
}

func newUDPPool() *udpPool {
	return &udpPool{conns: make(map[string][]net.Conn)}
}

func (p *udpPool) get(server string, timeout time.Duration) (net.Conn, error) {
	p.mu.Lock()
	list := p.conns[server]
	if len(list) > 0 {
		conn := list[len(list)-1]
		p.conns[server] = list[:len(list)-1]
		p.mu.Unlock()
		conn.SetDeadline(time.Now().Add(timeout))
		return conn, nil
	}
	p.mu.Unlock()

	conn, err := net.DialTimeout("udp", server, timeout)
	if err != nil {
		return nil, err
	}
	conn.SetDeadline(time.Now().Add(timeout))
	return conn, nil
}

func (p *udpPool) put(server string, conn net.Conn, failed bool) {
	if failed {
		conn.Close()
		return
	}
	conn.SetDeadline(time.Time{})
	p.mu.Lock()
	if len(p.conns[server]) < poolSizePerServer {
		p.conns[server] = append(p.conns[server], conn)
		p.mu.Unlock()
		return
	}
	p.mu.Unlock()
	conn.Close()
}
