package sim

import (
	"rts/internal/sim/fixed"
	"sort"
)

// UnitState represents a unit's finite-state.
type UnitState uint8

const (
	UnitIdle   UnitState = 0
	UnitMoving UnitState = 1
	UnitDead   UnitState = 2
)

// Unit is the minimal RTS unit for lockstep simulation.
type Unit struct {
	ID       uint32
	Owner    uint8
	Pos      fixed.Vec2
	HP       fixed.Fix32
	MaxHP    fixed.Fix32
	Speed    fixed.Fix32 // distance per tick
	State    UnitState
	TargetID uint32      // attack target (0 = none)
	MoveTo   fixed.Vec2  // move destination
}

// CmdOp identifies the type of command.
type CmdOp uint8

const (
	CmdMove   CmdOp = 1
	CmdAttack CmdOp = 2
	CmdStop   CmdOp = 3
)

// Cmd is a player command to be executed at a specific tick.
type Cmd struct {
	Player uint8
	Op     CmdOp
	UnitID uint32
	// Move/Attack target position (for CmdMove) or target unit (for CmdAttack via TargetID).
	TargetPos fixed.Vec2
	TargetID  uint32
}

// World holds the entire deterministic game state.
type World struct {
	Tick     uint32
	Seed     uint64
	Rand     *SplitMix64
	Units    []Unit
	NextID   uint32
	MapSizeX fixed.Fix32 // map width
	MapSizeY fixed.Fix32 // map height
}

// NewWorld creates a new world with the given seed and map dimensions.
func NewWorld(seed uint64, mapW, mapH int32) *World {
	return &World{
		Tick:     0,
		Seed:     seed,
		Rand:     NewRand(seed),
		Units:    make([]Unit, 0, 64),
		NextID:   1,
		MapSizeX: fixed.FromInt(mapW),
		MapSizeY: fixed.FromInt(mapH),
	}
}

// SpawnUnit adds a unit and returns its ID.
func (w *World) SpawnUnit(owner uint8, pos fixed.Vec2, hp, speed fixed.Fix32) uint32 {
	id := w.NextID
	w.NextID++
	w.Units = append(w.Units, Unit{
		ID:    id,
		Owner: owner,
		Pos:   pos,
		HP:    hp,
		MaxHP: hp,
		Speed: speed,
		State: UnitIdle,
	})
	return id
}

// FindUnit returns a pointer to the unit with the given ID, or nil.
// IMPORTANT: pointer is only valid until next append to Units slice.
func (w *World) FindUnit(id uint32) *Unit {
	for i := range w.Units {
		if w.Units[i].ID == id {
			return &w.Units[i]
		}
	}
	return nil
}

// SortedUnitsByID returns units sorted by ID for deterministic iteration.
func (w *World) SortedUnitsByID() []Unit {
	sorted := make([]Unit, len(w.Units))
	copy(sorted, w.Units)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ID < sorted[j].ID
	})
	return sorted
}

// SpawnForGolden is an alias for SpawnUnit used by the golden export tool.
func SpawnForGolden(w *World, owner uint8, pos fixed.Vec2, hp, speed fixed.Fix32) uint32 {
	return w.SpawnUnit(owner, pos, hp, speed)
}

// RemoveDead removes all dead units.
func (w *World) RemoveDead() {
	alive := w.Units[:0]
	for _, u := range w.Units {
		if u.State != UnitDead {
			alive = append(alive, u)
		}
	}
	w.Units = alive
}
