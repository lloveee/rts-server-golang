package sim

import (
	"rts/internal/sim/fixed"
	"sort"
)

// UnitState represents a unit's finite-state.
type UnitState uint8

const (
	UnitIdle      UnitState = 0
	UnitMoving    UnitState = 1
	UnitAttacking UnitState = 2
	UnitMining    UnitState = 3
	UnitReturning UnitState = 4
	UnitBuilding  UnitState = 5
	UnitDead      UnitState = 6
)

// UnitType enumerates the 4 Sub-2 unit archetypes.
type UnitType uint8

const (
	UnitWorker  UnitType = 1
	UnitSoldier UnitType = 2
	UnitArcher  UnitType = 3
	UnitCavalry UnitType = 4
)

// Unit is the RTS unit state for lockstep simulation.
type Unit struct {
	ID               uint32
	Owner            uint8
	Type             UnitType
	Pos              fixed.Vec2
	HP               fixed.Fix32
	MaxHP            fixed.Fix32
	Speed            fixed.Fix32
	Range            fixed.Fix32
	Damage           fixed.Fix32
	VisionRange      fixed.Fix32
	CarryAmount      fixed.Fix32
	State            UnitState
	TargetID         uint32
	MoveTo           fixed.Vec2
	AttackMoveTarget fixed.Vec2
	Path             []fixed.Vec2
}

// CmdOp identifies the type of command.
type CmdOp uint8

const (
	CmdMove       CmdOp = 1
	CmdAttack     CmdOp = 2
	CmdStop       CmdOp = 3
	CmdAttackMove CmdOp = 4
	CmdBuild      CmdOp = 5
	CmdTrain      CmdOp = 6
	CmdSurrender  CmdOp = 7
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
// ID pool (NextID) is shared across Units, Buildings, and Crystals — any new
// entity takes the next value, so IDs are globally unique within a World.
type World struct {
	Tick     uint32
	Seed     uint64
	Rand     *SplitMix64
	NextID   uint32
	MapSizeX fixed.Fix32
	MapSizeY fixed.Fix32

	Units     []Unit
	Buildings []Building
	Crystals  []Crystal
	Players   []Player

	NavGrid *NavGrid

	GameOver        bool
	GameOverResults []PlayerResult
}

// PlayerResult encodes one player's outcome.
type PlayerResult struct {
	PlayerID uint8
	Result   uint8 // 0=Ongoing, 1=Victory, 2=Defeat, 3=Draw
}

// Entity is implemented by all sim entity types for combat targeting.
type Entity interface {
	IsDead() bool
	GetPos() fixed.Vec2
	GetHP() fixed.Fix32
	SetHP(v fixed.Fix32)
	SetDead()
	GetID() uint32
}

func (u *Unit) IsDead() bool          { return u.State == UnitDead }
func (u *Unit) GetPos() fixed.Vec2    { return u.Pos }
func (u *Unit) GetHP() fixed.Fix32    { return u.HP }
func (u *Unit) SetHP(v fixed.Fix32)   { u.HP = v }
func (u *Unit) SetDead()              { u.State = UnitDead; u.HP = 0 }
func (u *Unit) GetID() uint32         { return u.ID }

func (b *Building) IsDead() bool        { return b.State == BldDead }
func (b *Building) GetPos() fixed.Vec2  { return b.Pos }
func (b *Building) GetHP() fixed.Fix32  { return b.HP }
func (b *Building) SetHP(v fixed.Fix32) { b.HP = v }
func (b *Building) SetDead()            { b.State = BldDead; b.HP = 0 }
func (b *Building) GetID() uint32       { return b.ID }

func (c *Crystal) IsDead() bool         { return c.Remaining <= 0 }
func (c *Crystal) GetPos() fixed.Vec2   { return c.Pos }
func (c *Crystal) GetHP() fixed.Fix32   { return c.Remaining }
func (c *Crystal) SetHP(v fixed.Fix32)  { c.Remaining = v }
func (c *Crystal) SetDead()             { c.Remaining = 0 }
func (c *Crystal) GetID() uint32        { return c.ID }

// FindEntityAny returns any entity by ID as the Entity interface.
func (w *World) FindEntityAny(id uint32) Entity {
	for i := range w.Units {
		if w.Units[i].ID == id && w.Units[i].State != UnitDead {
			return &w.Units[i]
		}
	}
	for i := range w.Buildings {
		if w.Buildings[i].ID == id && w.Buildings[i].State != BldDead {
			return &w.Buildings[i]
		}
	}
	for i := range w.Crystals {
		if w.Crystals[i].ID == id && w.Crystals[i].Remaining > 0 {
			return &w.Crystals[i]
		}
	}
	return nil
}

// NewWorld creates a new world with the given seed and map dimensions.
// Buildings/Crystals/Players start as empty (non-nil) slices; NavGrid is
// allocated 1:1 with map dimensions, all cells passable.
func NewWorld(seed uint64, mapW, mapH int32) *World {
	return &World{
		Tick:      0,
		Seed:      seed,
		Rand:      NewRand(seed),
		Units:     make([]Unit, 0, 64),
		Buildings: make([]Building, 0, 8),
		Crystals:  make([]Crystal, 0, 16),
		Players:   make([]Player, 0, 4),
		NextID:    1,
		MapSizeX:  fixed.FromInt(mapW),
		MapSizeY:  fixed.FromInt(mapH),
		NavGrid:   NewNavGrid(mapW, mapH),
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

// FindEntity returns the entity with the given ID across all lists.
func (w *World) FindEntity(id uint32) (found bool, unit *Unit, bld *Building, cryst *Crystal) {
	for i := range w.Units {
		if w.Units[i].ID == id && w.Units[i].State != UnitDead {
			return true, &w.Units[i], nil, nil
		}
	}
	for i := range w.Buildings {
		if w.Buildings[i].ID == id && w.Buildings[i].State != BldDead {
			return true, nil, &w.Buildings[i], nil
		}
	}
	for i := range w.Crystals {
		if w.Crystals[i].ID == id && w.Crystals[i].Remaining > 0 {
			return true, nil, nil, &w.Crystals[i]
		}
	}
	return false, nil, nil, nil
}

// SpawnBuilding adds a building and returns its ID.
func (w *World) SpawnBuilding(owner uint8, bldType BuildingType, pos fixed.Vec2) uint32 {
	stats := BuildingStatTable[bldType]
	id := w.NextID
	w.NextID++
	w.Buildings = append(w.Buildings, Building{
		ID:        id,
		Owner:     owner,
		Type:      bldType,
		SizeCells: stats.SizeCells,
		Pos:       pos,
		HP:        stats.MaxHP,
		MaxHP:     stats.MaxHP,
		State:     BldReady,
	})
	return id
}

// SpawnCrystal adds a crystal pile and returns its ID.
func (w *World) SpawnCrystal(pos fixed.Vec2) uint32 {
	id := w.NextID
	w.NextID++
	w.Crystals = append(w.Crystals, Crystal{
		ID:        id,
		Pos:       pos,
		Remaining: fixed.FromInt(CrystalStartValue),
	})
	return id
}

// sortBuildingsByID sorts buildings in-place ascending by ID.
func sortBuildingsByID(w *World) {
	blds := w.Buildings
	for i := 1; i < len(blds); i++ {
		key := blds[i]
		j := i - 1
		for j >= 0 && blds[j].ID > key.ID {
			blds[j+1] = blds[j]
			j--
		}
		blds[j+1] = key
	}
}

// sortCrystalsByID sorts crystals in-place ascending by ID.
func sortCrystalsByID(w *World) {
	crystals := w.Crystals
	for i := 1; i < len(crystals); i++ {
		key := crystals[i]
		j := i - 1
		for j >= 0 && crystals[j].ID > key.ID {
			crystals[j+1] = crystals[j]
			j--
		}
		crystals[j+1] = key
	}
}

// RemoveDeadBuildings prunes buildings with State == BldDead.
func (w *World) RemoveDeadBuildings() {
	alive := w.Buildings[:0]
	for _, b := range w.Buildings {
		if b.State != BldDead {
			alive = append(alive, b)
		}
	}
	w.Buildings = alive
}

// RemoveDeadCrystals prunes crystals with Remaining <= 0.
func (w *World) RemoveDeadCrystals() {
	alive := w.Crystals[:0]
	for _, c := range w.Crystals {
		if c.Remaining > 0 {
			alive = append(alive, c)
		}
	}
	w.Crystals = alive
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

// FindCrystalAt returns a crystal near the given position (within 2 cells).
func (w *World) FindCrystalAt(pos fixed.Vec2) *Crystal {
	for i := range w.Crystals {
		c := &w.Crystals[i]
		if c.Remaining <= 0 {
			continue
		}
		if c.Pos.DistSq(pos) <= fixed.FromInt(2).Mul(fixed.FromInt(2)) {
			return c
		}
	}
	return nil
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
