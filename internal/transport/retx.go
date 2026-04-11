package transport

import (
	"time"
)

// Retransmission timer constants.
const (
	InitialRTO    = 200 * time.Millisecond
	MaxRTO        = 2 * time.Second
	RTOMultiplier = 2 // exponential backoff factor
	MaxRetries    = 10
)

// pendingPacket tracks an unacknowledged reliable packet.
type pendingPacket struct {
	pkt       *Packet
	sentAt    time.Time
	nextRetx  time.Time
	rto       time.Duration
	retries   int
}

// retxQueue manages retransmission of unacknowledged reliable packets.
type retxQueue struct {
	pending map[uint32]*pendingPacket // seq → pending
}

func newRetxQueue() *retxQueue {
	return &retxQueue{
		pending: make(map[uint32]*pendingPacket),
	}
}

// Add registers a packet for retransmission tracking.
func (q *retxQueue) Add(pkt *Packet, now time.Time, rto time.Duration) {
	if rto < InitialRTO {
		rto = InitialRTO
	}
	q.pending[pkt.Seq] = &pendingPacket{
		pkt:      pkt,
		sentAt:   now,
		nextRetx: now.Add(rto),
		rto:      rto,
	}
}

// Ack removes a packet from the retransmission queue (it was acknowledged).
// Returns the RTT sample if the packet was found (first transmission only).
func (q *retxQueue) Ack(seq uint32, now time.Time) (rttSample time.Duration, ok bool) {
	pp, exists := q.pending[seq]
	if !exists {
		return 0, false
	}
	delete(q.pending, seq)
	// Only use first-transmission for RTT sample (avoid retransmission ambiguity).
	if pp.retries == 0 {
		return now.Sub(pp.sentAt), true
	}
	return 0, true
}

// AckBitmask acknowledges multiple packets using the selective ACK bitmask.
// ackSeq is the highest acknowledged seq; bitmask covers [ackSeq-32..ackSeq-1].
func (q *retxQueue) AckBitmask(ackSeq uint32, bitmask uint32, now time.Time) {
	q.Ack(ackSeq, now)
	for i := uint32(0); i < 32; i++ {
		if bitmask&(1<<i) != 0 {
			seq := ackSeq - 1 - i
			q.Ack(seq, now)
		}
	}
}

// CollectRetransmissions returns packets that need retransmission now.
// Also applies exponential backoff. Returns nil for packets exceeding MaxRetries.
func (q *retxQueue) CollectRetransmissions(now time.Time) ([]*Packet, []uint32) {
	var retx []*Packet
	var expired []uint32

	for seq, pp := range q.pending {
		if now.Before(pp.nextRetx) {
			continue
		}
		pp.retries++
		if pp.retries > MaxRetries {
			expired = append(expired, seq)
			continue
		}
		pp.rto *= time.Duration(RTOMultiplier)
		if pp.rto > MaxRTO {
			pp.rto = MaxRTO
		}
		pp.nextRetx = now.Add(pp.rto)
		retx = append(retx, pp.pkt)
	}

	for _, seq := range expired {
		delete(q.pending, seq)
	}

	return retx, expired
}

// PendingCount returns the number of unacknowledged packets.
func (q *retxQueue) PendingCount() int {
	return len(q.pending)
}
