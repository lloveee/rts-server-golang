package transport

import (
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
)

// Listener manages the UDP socket and routes packets to per-peer Conns.
type Listener struct {
	conn      *net.UDPConn
	conns     map[uint16]*Conn // connID → Conn
	connsByAddr map[string]*Conn // addr string → Conn (for SYN routing before connID assignment)
	mu        sync.RWMutex
	nextID    uint16
	chaos     ChaosHook
	log       *slog.Logger
	closed    atomic.Bool

	// OnNewConn is called when a new connection is established (SYN received).
	// Set this before calling Serve.
	OnNewConn func(c *Conn)

	// OnMessage is called when a payload is delivered in-order on an established connection.
	// If nil, payloads go to Conn.Inbox channel instead.
	OnMessage func(c *Conn, payload []byte)
}

// ListenerConfig holds parameters for creating a listener.
type ListenerConfig struct {
	Addr    string
	Chaos   ChaosHook
	Logger  *slog.Logger
}

// Listen creates and binds a UDP listener.
func Listen(cfg ListenerConfig) (*Listener, error) {
	addr, err := net.ResolveUDPAddr("udp", cfg.Addr)
	if err != nil {
		return nil, err
	}
	udpConn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, err
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Listener{
		conn:        udpConn,
		conns:       make(map[uint16]*Conn),
		connsByAddr: make(map[string]*Conn),
		nextID:      1,
		chaos:       cfg.Chaos,
		log:         cfg.Logger,
	}, nil
}

// LocalAddr returns the listener's local address.
func (l *Listener) LocalAddr() net.Addr {
	return l.conn.LocalAddr()
}

// Serve starts the read loop. Blocks until Close is called.
func (l *Listener) Serve() error {
	buf := make([]byte, 2048)
	for {
		n, remoteAddr, err := l.conn.ReadFromUDP(buf)
		if err != nil {
			if l.closed.Load() {
				return nil
			}
			l.log.Warn("read error", "err", err)
			continue
		}
		data := make([]byte, n)
		copy(data, buf[:n])
		l.handlePacket(data, remoteAddr)
	}
}

// Close shuts down the listener.
func (l *Listener) Close() error {
	l.closed.Store(true)
	return l.conn.Close()
}

// GetConn returns a connection by ID.
func (l *Listener) GetConn(id uint16) *Conn {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.conns[id]
}

// Conns returns all active connections (snapshot).
func (l *Listener) Conns() []*Conn {
	l.mu.RLock()
	defer l.mu.RUnlock()
	result := make([]*Conn, 0, len(l.conns))
	for _, c := range l.conns {
		result = append(result, c)
	}
	return result
}

// WriteTo sends raw bytes to a UDP address.
func (l *Listener) WriteTo(data []byte, addr *net.UDPAddr) error {
	_, err := l.conn.WriteToUDP(data, addr)
	return err
}

func (l *Listener) handlePacket(data []byte, addr *net.UDPAddr) {
	pkt, err := DecodePacket(data)
	if err != nil {
		return // silently drop malformed packets
	}

	// Route by conn_id first.
	if pkt.ConnID != 0 {
		l.mu.RLock()
		c := l.conns[pkt.ConnID]
		l.mu.RUnlock()
		if c != nil {
			// Update remote addr (supports IP migration).
			c.RemoteAddr = addr
			c.HandleReceive(pkt)
			return
		}
	}

	// SYN handling: new connection.
	if pkt.IsSYN() {
		l.handleSYN(pkt, addr)
		return
	}

	// Unknown conn_id and not SYN: drop.
}

func (l *Listener) handleSYN(pkt *Packet, addr *net.UDPAddr) {
	addrKey := addr.String()

	l.mu.Lock()
	// Check if we already have a connection from this addr (duplicate SYN).
	if existing, ok := l.connsByAddr[addrKey]; ok {
		l.mu.Unlock()
		// Resend SYN-ACK.
		l.sendSYNACK(existing)
		return
	}

	id := l.nextID
	l.nextID++

	c := NewConn(ConnConfig{
		ConnID:     id,
		RemoteAddr: addr,
		SendFunc:   l.WriteTo,
		Chaos:      l.chaos,
	})
	c.SetState(StateEstablished)

	l.conns[id] = c
	l.connsByAddr[addrKey] = c
	l.mu.Unlock()

	l.sendSYNACK(c)

	l.log.Info("new connection", "conn_id", id, "addr", addr)

	if l.OnNewConn != nil {
		l.OnNewConn(c)
	}
}

func (l *Listener) sendSYNACK(c *Conn) {
	pkt := &Packet{
		Flags:  FlagSYN | FlagACK,
		ConnID: c.ConnID,
	}
	_ = l.WriteTo(pkt.Encode(), c.RemoteAddr)
}

// RemoveConn removes a connection from the listener.
func (l *Listener) RemoveConn(id uint16) {
	l.mu.Lock()
	defer l.mu.Unlock()
	c, ok := l.conns[id]
	if ok {
		delete(l.conns, id)
		delete(l.connsByAddr, c.RemoteAddr.String())
	}
}
