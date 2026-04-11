package lockstep

import "time"

// AdaptiveN manages the dynamic input delay N based on RTT reports.
type AdaptiveN struct {
	current    uint8
	minN       uint8
	maxN       uint8
	tickRate   int   // Hz
	rttSamples map[uint8]time.Duration // playerID → latest RTT
}

// NewAdaptiveN creates an adaptive N manager.
func NewAdaptiveN(initial, minN, maxN uint8, tickRate int) *AdaptiveN {
	return &AdaptiveN{
		current:    initial,
		minN:       minN,
		maxN:       maxN,
		tickRate:   tickRate,
		rttSamples: make(map[uint8]time.Duration),
	}
}

// Current returns the current input delay N.
func (a *AdaptiveN) Current() uint8 {
	return a.current
}

// ReportRTT updates the RTT sample for a player.
func (a *AdaptiveN) ReportRTT(playerID uint8, rtt time.Duration) {
	a.rttSamples[playerID] = rtt
}

// Recalculate computes the optimal N based on current RTT samples.
// Returns (newN, changed).
func (a *AdaptiveN) Recalculate() (uint8, bool) {
	if len(a.rttSamples) == 0 {
		return a.current, false
	}

	// Find max RTT across all players.
	var maxRTT time.Duration
	for _, rtt := range a.rttSamples {
		if rtt > maxRTT {
			maxRTT = rtt
		}
	}

	// N = ceil(maxRTT / tickInterval) + 1 safety margin.
	tickInterval := time.Second / time.Duration(a.tickRate)
	needed := int(maxRTT/tickInterval) + 1 + 1 // +1 for ceil, +1 safety
	if needed < int(a.minN) {
		needed = int(a.minN)
	}
	if needed > int(a.maxN) {
		needed = int(a.maxN)
	}

	newN := uint8(needed)
	if newN == a.current {
		return a.current, false
	}

	old := a.current
	a.current = newN
	_ = old
	return newN, true
}
