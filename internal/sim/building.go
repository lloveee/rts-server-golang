package sim

import "rts/internal/sim/fixed"

// BuildingType enumerates the 4 Sub-2 building archetypes.
type BuildingType uint8

const (
	BldHQ       BuildingType = 1
	BldBarracks BuildingType = 2
	BldArchery  BuildingType = 3
	BldStable   BuildingType = 4
)

// BuildingState tracks construction lifecycle.
type BuildingState uint8

const (
	BldConstructing BuildingState = 0
	BldReady        BuildingState = 1
	BldDead         BuildingState = 2
)

// QueueItem is a single training slot in a building's ProductionQueue.
type QueueItem struct {
	UnitType  UnitType
	TicksLeft uint32
	StartTick uint32
}

// Building is a placed structure (HQ, Barracks, Archery, Stable).
// Pos is the AABB lower-left corner in world coordinates.
type Building struct {
	ID                uint32
	Owner             uint8
	Type              BuildingType
	SizeCells         uint8
	Pos               fixed.Vec2
	HP                fixed.Fix32
	MaxHP             fixed.Fix32
	State             BuildingState
	ConstructProgress fixed.Fix32
	RallyPoint        fixed.Vec2
	ProductionQueue   []QueueItem
}
