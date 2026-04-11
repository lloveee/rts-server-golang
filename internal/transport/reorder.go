package transport

// reorderBuffer provides in-order delivery of out-of-order reliable packets.
// It buffers packets arriving ahead of sequence and delivers them once the gap is filled.
type reorderBuffer struct {
	nextExpected uint32
	buffer       map[uint32][]byte // seq → payload
	maxBuffered  int               // max out-of-order packets to hold
}

func newReorderBuffer(startSeq uint32, maxBuffered int) *reorderBuffer {
	return &reorderBuffer{
		nextExpected: startSeq,
		buffer:       make(map[uint32][]byte),
		maxBuffered:  maxBuffered,
	}
}

// Insert adds a received packet. Returns payloads ready for in-order delivery.
// Duplicate or old packets are silently dropped.
func (rb *reorderBuffer) Insert(seq uint32, payload []byte) [][]byte {
	// Old or duplicate?
	if seqLT(seq, rb.nextExpected) {
		return nil
	}

	// Too far ahead? Drop to prevent memory exhaustion.
	if int(seq-rb.nextExpected) > rb.maxBuffered {
		return nil
	}

	if seq == rb.nextExpected {
		// In-order: deliver immediately, then flush any buffered consecutive packets.
		var delivered [][]byte
		delivered = append(delivered, payload)
		rb.nextExpected++

		for {
			p, ok := rb.buffer[rb.nextExpected]
			if !ok {
				break
			}
			delivered = append(delivered, p)
			delete(rb.buffer, rb.nextExpected)
			rb.nextExpected++
		}
		return delivered
	}

	// Out of order: buffer it.
	if _, exists := rb.buffer[seq]; !exists {
		rb.buffer[seq] = payload
	}
	return nil
}

// NextExpected returns the next sequence number expected for in-order delivery.
func (rb *reorderBuffer) NextExpected() uint32 {
	return rb.nextExpected
}

// seqLT returns true if a < b in sequence number space (handles wraparound).
func seqLT(a, b uint32) bool {
	return int32(a-b) < 0
}
