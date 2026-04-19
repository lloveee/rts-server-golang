package sim

// NavGrid is the A* collision bitmap. 1:1 with map cells (1 unit = 1 cell).
// Blocked[(y*W + x) / 64] bit (y*W + x) % 64 == 1 → cell is impassable.
// Buildings occupy their AABB cells; units do not enter the bitmap.
type NavGrid struct {
	W       int32
	H       int32
	Blocked []uint64
}

// NewNavGrid allocates a fresh all-passable grid sized (w, h).
func NewNavGrid(w, h int32) *NavGrid {
	total := int(w) * int(h)
	words := (total + 63) / 64
	return &NavGrid{
		W:       w,
		H:       h,
		Blocked: make([]uint64, words),
	}
}

// BlockedWords returns the length of the underlying word slice.
func (n *NavGrid) BlockedWords() int {
	return len(n.Blocked)
}
