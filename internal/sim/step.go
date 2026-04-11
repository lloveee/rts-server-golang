package sim

import (
	"rts/internal/sim/fixed"
)

// AttackRange is the fixed range at which units can attack.
var AttackRange = fixed.FromInt(2)

// AttackDamage is the damage dealt per tick when in range.
var AttackDamage = fixed.FromFloat64(0.5)

// Step advances the world by one tick, applying the given commands.
// This function MUST be deterministic: same world state + same cmds = same result.
// Single-threaded only — never call concurrently.
func Step(w *World, cmds []Cmd) {
	w.Tick++

	// Phase 1: Apply commands (sorted by UnitID for determinism).
	// Commands are already grouped per-tick by the lockstep layer.
	applyCommands(w, cmds)

	// Phase 2: Update units in ID order.
	// We iterate over indices (not a copy) and update in-place.
	// Sort first to ensure deterministic order.
	sortUnitsByID(w)

	for i := range w.Units {
		u := &w.Units[i]
		if u.State == UnitDead {
			continue
		}
		switch u.State {
		case UnitMoving:
			stepMove(w, u)
		case UnitIdle:
			stepAttack(w, u)
		}
	}

	// Phase 3: Remove dead units.
	w.RemoveDead()
}

func applyCommands(w *World, cmds []Cmd) {
	for _, cmd := range cmds {
		u := w.FindUnit(cmd.UnitID)
		if u == nil || u.State == UnitDead {
			continue
		}
		// Only the owner can command their unit.
		if u.Owner != cmd.Player {
			continue
		}
		switch cmd.Op {
		case CmdMove:
			u.State = UnitMoving
			u.MoveTo = cmd.TargetPos
			u.TargetID = 0
		case CmdAttack:
			u.State = UnitIdle // will attack in stepAttack
			u.TargetID = cmd.TargetID
		case CmdStop:
			u.State = UnitIdle
			u.TargetID = 0
		}
	}
}

func stepMove(w *World, u *Unit) {
	newPos := fixed.MoveToward(u.Pos, u.MoveTo, u.Speed)
	// Clamp to map bounds.
	newPos.X = newPos.X.Clamp(0, w.MapSizeX)
	newPos.Y = newPos.Y.Clamp(0, w.MapSizeY)
	u.Pos = newPos
	// Arrived?
	if u.Pos.DistSq(u.MoveTo) <= fixed.Eps {
		u.State = UnitIdle
	}
}

func stepAttack(w *World, u *Unit) {
	if u.TargetID == 0 {
		return
	}
	target := w.FindUnit(u.TargetID)
	if target == nil || target.State == UnitDead {
		u.TargetID = 0
		return
	}

	distSq := u.Pos.DistSq(target.Pos)
	rangeSq := AttackRange.Mul(AttackRange)

	if distSq <= rangeSq {
		// In range: deal damage.
		target.HP = target.HP.Sub(AttackDamage)
		if target.HP <= 0 {
			target.State = UnitDead
			target.HP = 0
		}
	} else {
		// Out of range: move toward target.
		u.Pos = fixed.MoveToward(u.Pos, target.Pos, u.Speed)
	}
}

// sortUnitsByID sorts units in-place by ID.
func sortUnitsByID(w *World) {
	units := w.Units
	// Insertion sort — fast for nearly-sorted small slices (~300 units).
	for i := 1; i < len(units); i++ {
		key := units[i]
		j := i - 1
		for j >= 0 && units[j].ID > key.ID {
			units[j+1] = units[j]
			j--
		}
		units[j+1] = key
	}
}
