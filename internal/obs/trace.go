package obs

import (
	"fmt"
	"io"
	"sync"
	"time"
)

const DefaultTraceSize = 4096

// TraceEntry records one tick's internal timing.
type TraceEntry struct {
	Tick       uint32
	Timestamp  int64 // unix microseconds
	SealDurUs  int64 // seal duration in microseconds
	CmdCount   int
	PlayerMask uint8 // bitmask of players who submitted commands
	HashState  uint8 // 0=pending, 1=match, 2=desync, 3=timeout
}

// TraceRing is a fixed-size ring buffer of trace entries.
type TraceRing struct {
	mu      sync.Mutex
	entries []TraceEntry
	head    int  // next write position
	full    bool // whether we've wrapped around
	size    int
}

// NewTraceRing creates a trace ring buffer.
func NewTraceRing(size int) *TraceRing {
	if size <= 0 {
		size = DefaultTraceSize
	}
	return &TraceRing{
		entries: make([]TraceEntry, size),
		size:    size,
	}
}

// Record adds a trace entry.
func (tr *TraceRing) Record(entry TraceEntry) {
	tr.mu.Lock()
	tr.entries[tr.head] = entry
	tr.head = (tr.head + 1) % tr.size
	if tr.head == 0 {
		tr.full = true
	}
	tr.mu.Unlock()
}

// RecordTick is a convenience for recording a tick with timing.
func (tr *TraceRing) RecordTick(tick uint32, sealStart time.Time, cmdCount int, playerMask uint8) {
	tr.Record(TraceEntry{
		Tick:       tick,
		Timestamp:  time.Now().UnixMicro(),
		SealDurUs:  time.Since(sealStart).Microseconds(),
		CmdCount:   cmdCount,
		PlayerMask: playerMask,
	})
}

// Snapshot returns a copy of all entries in chronological order.
func (tr *TraceRing) Snapshot() []TraceEntry {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	if !tr.full && tr.head == 0 {
		return nil
	}

	var result []TraceEntry
	if tr.full {
		// head..end, then 0..head-1
		result = make([]TraceEntry, tr.size)
		copy(result, tr.entries[tr.head:])
		copy(result[tr.size-tr.head:], tr.entries[:tr.head])
	} else {
		result = make([]TraceEntry, tr.head)
		copy(result, tr.entries[:tr.head])
	}
	return result
}

// Count returns the number of entries stored.
func (tr *TraceRing) Count() int {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	if tr.full {
		return tr.size
	}
	return tr.head
}

// WriteTo writes trace entries as a text dump.
func (tr *TraceRing) WriteTo(w io.Writer) (int64, error) {
	entries := tr.Snapshot()
	var total int64
	n, _ := fmt.Fprintf(w, "tick\ttimestamp_us\tseal_dur_us\tcmd_count\tplayer_mask\thash_state\n")
	total += int64(n)
	for _, e := range entries {
		n, _ := fmt.Fprintf(w, "%d\t%d\t%d\t%d\t%08b\t%d\n",
			e.Tick, e.Timestamp, e.SealDurUs, e.CmdCount, e.PlayerMask, e.HashState)
		total += int64(n)
	}
	return total, nil
}
