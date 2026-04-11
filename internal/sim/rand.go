package sim

// SplitMix64 is a deterministic PRNG suitable for lockstep simulation.
// Same seed + same call sequence = same output on every platform.
type SplitMix64 struct {
	state uint64
}

// NewRand creates a deterministic PRNG from a seed.
func NewRand(seed uint64) *SplitMix64 {
	return &SplitMix64{state: seed}
}

// Next returns the next pseudo-random uint64.
func (r *SplitMix64) Next() uint64 {
	r.state += 0x9e3779b97f4a7c15
	z := r.state
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

// Intn returns a pseudo-random int in [0, n).
func (r *SplitMix64) Intn(n int) int {
	if n <= 0 {
		return 0
	}
	return int(r.Next() % uint64(n))
}
