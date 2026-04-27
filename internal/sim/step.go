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

	applyCommands(w, cmds)

	sortUnitsByID(w)
	sortBuildingsByID(w)
	sortCrystalsByID(w)

	for i := range w.Units {
		u := &w.Units[i]
		if u.State == UnitDead {
			continue
		}
		switch u.State {
		case UnitMoving:
			stepMove(w, u)
		case UnitMining:
			stepMining(w, u)
		case UnitReturning:
			stepReturning(w, u)
		case UnitBuilding:
			// Phase 3 — no-op for now
		case UnitAttacking:
			stepAttack(w, u)
		case UnitIdle:
			stepAttack(w, u)
		}
	}

	tickProduction(w)

	w.RemoveDead()
	w.RemoveDeadBuildings()
	w.RemoveDeadCrystals()
}

func applyCommands(w *World, cmds []Cmd) {
	for _, cmd := range cmds {
		switch cmd.Op {
		case CmdMove:
			u := w.FindUnit(cmd.UnitID)
			if u == nil || u.State == UnitDead || u.Owner != cmd.Player {
				continue
			}
			// Worker right-click on crystal → enter mining loop.
			crystal := w.FindCrystalAt(cmd.TargetPos)
			if crystal != nil && u.Type == UnitWorker {
				u.State = UnitMining
				u.TargetID = crystal.ID
				u.MoveTo = crystal.Pos
				u.CarryAmount = 0
				continue
			}
			u.State = UnitMoving
			u.MoveTo = cmd.TargetPos
			u.TargetID = 0
		case CmdAttack:
			u := w.FindUnit(cmd.UnitID)
			if u == nil || u.State == UnitDead || u.Owner != cmd.Player {
				continue
			}
			if u.Range <= 0 {
				continue // Workers can't attack
			}
			u.State = UnitIdle
			u.TargetID = cmd.TargetID
		case CmdStop:
			u := w.FindUnit(cmd.UnitID)
			if u == nil || u.State == UnitDead || u.Owner != cmd.Player {
				continue
			}
			u.State = UnitIdle
			u.TargetID = 0
			u.CarryAmount = 0
		case CmdTrain:
			applyCmdTrain(w, cmd)
		// CmdAttackMove, CmdBuild, CmdSurrender — Phase 2/3
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
