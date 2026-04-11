package lockstep

// HashState represents the state of hash collection for a single tick.
type HashState int

const (
	HashPending HashState = 0
	HashPartial HashState = 1
	HashMatch   HashState = 2
	HashTimeout HashState = 3
	HashDesync  HashState = 4
)

// tickHash stores the hash collection state for one tick.
type tickHash struct {
	tick     uint32
	hashes   map[uint8]uint64 // playerID → hash
	expected int              // how many players we expect
	state    HashState
}

// HashAgg aggregates per-tick hashes from all clients and detects desync.
type HashAgg struct {
	window     map[uint32]*tickHash
	windowSize int
	oldestTick uint32
	playerCount int
}

// NewHashAgg creates a hash aggregator with the given window size and player count.
func NewHashAgg(windowSize, playerCount int) *HashAgg {
	return &HashAgg{
		window:      make(map[uint32]*tickHash),
		windowSize:  windowSize,
		playerCount: playerCount,
	}
}

// CreateSlot prepares a hash collection slot for a tick.
func (h *HashAgg) CreateSlot(tick uint32) {
	h.window[tick] = &tickHash{
		tick:     tick,
		hashes:   make(map[uint8]uint64),
		expected: h.playerCount,
		state:    HashPending,
	}
	// Slide window: remove old ticks.
	if tick > uint32(h.windowSize) {
		cutoff := tick - uint32(h.windowSize)
		for t := range h.window {
			if t < cutoff {
				delete(h.window, t)
			}
		}
	}
}

// Report records a hash from a player for a tick.
// Returns the new state for that tick.
func (h *HashAgg) Report(tick uint32, playerID uint8, hash uint64) HashState {
	th, ok := h.window[tick]
	if !ok {
		return HashTimeout // tick already expired from window
	}

	th.hashes[playerID] = hash

	if len(th.hashes) < th.expected {
		th.state = HashPartial
		return HashPartial
	}

	// All players reported. Check consistency.
	var ref uint64
	first := true
	for _, v := range th.hashes {
		if first {
			ref = v
			first = false
			continue
		}
		if v != ref {
			th.state = HashDesync
			return HashDesync
		}
	}
	th.state = HashMatch
	return HashMatch
}

// CheckTimeouts marks old pending/partial ticks as timed out.
// Returns ticks that timed out.
func (h *HashAgg) CheckTimeouts(currentTick uint32, maxAge uint32) []uint32 {
	var timedOut []uint32
	for tick, th := range h.window {
		if th.state == HashPending || th.state == HashPartial {
			if currentTick-tick > maxAge {
				th.state = HashTimeout
				timedOut = append(timedOut, tick)
			}
		}
	}
	return timedOut
}

// GetState returns the hash state for a tick.
func (h *HashAgg) GetState(tick uint32) (HashState, map[uint8]uint64) {
	th, ok := h.window[tick]
	if !ok {
		return HashTimeout, nil
	}
	return th.state, th.hashes
}
