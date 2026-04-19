package sim

import "rts/internal/sim/fixed"

// Crystal is a harvestable resource pile.
type Crystal struct {
	ID        uint32
	Pos       fixed.Vec2
	Remaining fixed.Fix32
}
