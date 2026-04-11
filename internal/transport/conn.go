package transport

import (
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// ConnState represents the connection lifecycle state.
type ConnState int32

const (
	StateClosed      ConnState = 0
	StateSynSent     ConnState = 1
	StateEstablished ConnState = 2
	StateFin         ConnState = 3
)

// Conn is a per-peer reliable UDP connection.
// The reader goroutine calls Receive*; the writer goroutine calls Send/Flush.
// Shared counters use atomics; everything else is goroutine-local.
type Conn struct {
	// Identity
	ConnID   uint16
	RemoteAddr *net.UDPAddr

	// State (atomic for cross-goroutine reads)
	state atomic.Int32

	// Sending (writer goroutine owns)
	sendSeq   uint32
	sendMu    sync.Mutex // protects sendSeq and retx queue
	retx      *retxQueue
	sendFunc  func(data []byte, addr *net.UDPAddr) error

	// Receiving (reader goroutine owns)
	recvAck      uint32
	recvBitmask  uint32
	reorder      *reorderBuffer

	// RTT estimation (writer updates, both read)
	srtt  atomic.Int64 // smoothed RTT in nanoseconds
	rttVar atomic.Int64

	// Chaos hook (nil in production)
	chaos ChaosHook

	// Delivery channel: ordered payloads ready for upper layers
	Inbox chan []byte

	// Lifecycle
	lastRecv atomic.Int64 // UnixNano of last received packet
}

// ConnConfig holds parameters for creating a new connection.
type ConnConfig struct {
	ConnID     uint16
	RemoteAddr *net.UDPAddr
	SendFunc   func(data []byte, addr *net.UDPAddr) error
	Chaos      ChaosHook // nil for production
	InboxSize  int       // channel buffer size
}

// NewConn creates a new connection.
func NewConn(cfg ConnConfig) *Conn {
	if cfg.InboxSize <= 0 {
		cfg.InboxSize = 256
	}
	c := &Conn{
		ConnID:     cfg.ConnID,
		RemoteAddr: cfg.RemoteAddr,
		retx:       newRetxQueue(),
		sendFunc:   cfg.SendFunc,
		reorder:    newReorderBuffer(1, 512),
		chaos:      cfg.Chaos,
		Inbox:      make(chan []byte, cfg.InboxSize),
	}
	c.state.Store(int32(StateClosed))
	c.srtt.Store(int64(100 * time.Millisecond))
	c.lastRecv.Store(time.Now().UnixNano())
	return c
}

// State returns the current connection state.
func (c *Conn) State() ConnState {
	return ConnState(c.state.Load())
}

// SetState sets the connection state.
func (c *Conn) SetState(s ConnState) {
	c.state.Store(int32(s))
}

// SRTT returns the smoothed RTT estimate.
func (c *Conn) SRTT() time.Duration {
	return time.Duration(c.srtt.Load())
}

// LastRecvTime returns when we last received data from this peer.
func (c *Conn) LastRecvTime() time.Time {
	return time.Unix(0, c.lastRecv.Load())
}

// Send queues a reliable payload for sending.
func (c *Conn) Send(payload []byte) error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()

	c.sendSeq++
	pkt := &Packet{
		Flags:      FlagReliable | FlagACK,
		ConnID:     c.ConnID,
		Seq:        c.sendSeq,
		Ack:        c.recvAck,
		AckBitmask: c.recvBitmask,
		Payload:    payload,
	}

	data := pkt.Encode()

	if c.chaos != nil && c.chaos.ShouldDrop() {
		// Simulate drop — don't send, but still track for retx.
		c.retx.Add(pkt, time.Now(), c.rto())
		return nil
	}

	err := c.sendFunc(data, c.RemoteAddr)
	if err != nil {
		return err
	}

	if c.chaos != nil && c.chaos.ShouldDuplicate() {
		_ = c.sendFunc(data, c.RemoteAddr)
	}

	c.retx.Add(pkt, time.Now(), c.rto())
	return nil
}

// SendUnreliable sends a payload without retransmission tracking.
func (c *Conn) SendUnreliable(payload []byte) error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()

	c.sendSeq++
	pkt := &Packet{
		Flags:      FlagACK, // piggyback ACK, but not reliable
		ConnID:     c.ConnID,
		Seq:        c.sendSeq,
		Ack:        c.recvAck,
		AckBitmask: c.recvBitmask,
		Payload:    payload,
	}
	data := pkt.Encode()
	return c.sendFunc(data, c.RemoteAddr)
}

// SendACK sends a pure ACK packet (no payload).
func (c *Conn) SendACK() error {
	pkt := &Packet{
		Flags:      FlagACK,
		ConnID:     c.ConnID,
		Seq:        0, // pure ACK doesn't consume a seq
		Ack:        c.recvAck,
		AckBitmask: c.recvBitmask,
	}
	data := pkt.Encode()
	return c.sendFunc(data, c.RemoteAddr)
}

// SendPing sends a PING keepalive.
func (c *Conn) SendPing() error {
	pkt := &Packet{
		Flags:  FlagPING | FlagACK,
		ConnID: c.ConnID,
		Ack:    c.recvAck,
		AckBitmask: c.recvBitmask,
	}
	data := pkt.Encode()
	return c.sendFunc(data, c.RemoteAddr)
}

// HandleReceive processes an incoming packet (called by reader goroutine).
func (c *Conn) HandleReceive(pkt *Packet) {
	now := time.Now()
	c.lastRecv.Store(now.UnixNano())

	// Process ACK from the peer.
	if pkt.IsACK() {
		c.sendMu.Lock()
		if rtt, ok := c.retx.Ack(pkt.Ack, now); ok {
			c.updateRTT(rtt)
		}
		c.retx.AckBitmask(pkt.Ack, pkt.AckBitmask, now)
		c.sendMu.Unlock()
	}

	// Process reliable data.
	if pkt.IsReliable() && len(pkt.Payload) > 0 {
		c.updateRecvAck(pkt.Seq)
		delivered := c.reorder.Insert(pkt.Seq, pkt.Payload)
		for _, payload := range delivered {
			select {
			case c.Inbox <- payload:
			default:
				// Inbox full — drop oldest or this one. For now, drop this.
			}
		}
	}

	// Non-reliable data with payload (e.g., feedback hints).
	if !pkt.IsReliable() && len(pkt.Payload) > 0 && !pkt.IsPING() {
		select {
		case c.Inbox <- pkt.Payload:
		default:
		}
	}
}

// Tick is called periodically by the writer goroutine to handle retransmissions.
func (c *Conn) Tick(now time.Time) (retransmitted int, expired []uint32) {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()

	pkts, exp := c.retx.CollectRetransmissions(now)
	for _, pkt := range pkts {
		// Update piggyback ACK.
		pkt.Ack = c.recvAck
		pkt.AckBitmask = c.recvBitmask
		data := pkt.Encode()
		_ = c.sendFunc(data, c.RemoteAddr)
	}
	return len(pkts), exp
}

// PendingCount returns the number of unacknowledged reliable packets.
func (c *Conn) PendingCount() int {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	return c.retx.PendingCount()
}

// updateRecvAck updates the receiver-side ACK state for selective ACK.
func (c *Conn) updateRecvAck(seq uint32) {
	if seqLT(c.recvAck, seq) {
		// New highest received. Shift bitmask.
		diff := seq - c.recvAck
		if diff <= 32 {
			c.recvBitmask = (c.recvBitmask << diff) | (1 << (diff - 1))
		} else {
			c.recvBitmask = 0
		}
		c.recvAck = seq
	} else if seq < c.recvAck {
		// Older packet — set bit in bitmask.
		diff := c.recvAck - seq
		if diff <= 32 {
			c.recvBitmask |= 1 << (diff - 1)
		}
	}
}

// updateRTT applies Jacobson's algorithm for SRTT estimation.
func (c *Conn) updateRTT(sample time.Duration) {
	if sample <= 0 {
		return
	}
	srtt := time.Duration(c.srtt.Load())
	rttvar := time.Duration(c.rttVar.Load())

	if srtt == 0 {
		srtt = sample
		rttvar = sample / 2
	} else {
		diff := srtt - sample
		if diff < 0 {
			diff = -diff
		}
		rttvar = (3*rttvar + diff) / 4
		srtt = (7*srtt + sample) / 8
	}

	c.srtt.Store(int64(srtt))
	c.rttVar.Store(int64(rttvar))
}

// rto returns the retransmission timeout: SRTT + 4*RTTVAR, clamped.
func (c *Conn) rto() time.Duration {
	srtt := time.Duration(c.srtt.Load())
	rttvar := time.Duration(c.rttVar.Load())
	rto := srtt + 4*rttvar
	if rto < InitialRTO {
		rto = InitialRTO
	}
	if rto > MaxRTO {
		rto = MaxRTO
	}
	return rto
}
