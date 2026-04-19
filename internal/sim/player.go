package sim

import "rts/internal/sim/fixed"

// Player is per-player sim state (crystal bank + surrender flag).
type Player struct {
	ID          uint8
	Crystal     fixed.Fix32
	Surrendered bool
}
