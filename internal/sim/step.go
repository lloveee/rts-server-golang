package sim

import (
	"rts/internal/sim/fixed"
)

var AttackRange = fixed.FromInt(2)
var AttackDamage = fixed.FromFloat64(0.5)

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
			if len(u.Path) > 0 {
				stepMovePath(w, u)
			} else {
				stepMove(w, u)
			}
		case UnitMining:
			stepMining(w, u)
		case UnitReturning:
			stepReturning(w, u)
		case UnitBuilding:
			stepBuilding(w, u)
		case UnitAttacking:
			stepAttackOrAdvance(w, u)
		case UnitIdle:
			// No auto-acquire for idle units.
		}
	}

	tickProduction(w)
	tickConstruction(w)
	resolveCombat(w)
	applyPushAway(w)

	w.RemoveDead()
	w.RemoveDeadBuildings()
	w.RemoveDeadCrystals()

	results := checkVictory(w)
	if results != nil {
		w.GameOver = true
		w.GameOverResults = results
	}
}

func applyCommands(w *World, cmds []Cmd) {
	for _, cmd := range cmds {
		switch cmd.Op {
		case CmdMove:
			u := w.FindUnit(cmd.UnitID)
			if u == nil || u.State == UnitDead || u.Owner != cmd.Player {
				continue
			}
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
			u.Path = FindPath(w, u.Pos, cmd.TargetPos)
			u.TargetID = 0
		case CmdAttack:
			u := w.FindUnit(cmd.UnitID)
			if u == nil || u.State == UnitDead || u.Owner != cmd.Player {
				continue
			}
			if u.Range <= 0 {
				continue
			}
			u.State = UnitAttacking
			u.TargetID = cmd.TargetID
		case CmdAttackMove:
			u := w.FindUnit(cmd.UnitID)
			if u == nil || u.State == UnitDead || u.Owner != cmd.Player {
				continue
			}
			if u.Range <= 0 {
				continue
			}
			u.State = UnitAttacking
			u.AttackMoveTarget = cmd.TargetPos
			u.Path = FindPath(w, u.Pos, cmd.TargetPos)
			u.TargetID = 0
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
		case CmdBuild:
			applyCmdBuild(w, cmd)
		case CmdSurrender:
			if int(cmd.Player) < len(w.Players) {
				w.Players[cmd.Player].Surrendered = true
			}
		}
	}
}

func stepMove(w *World, u *Unit) {
	newPos := fixed.MoveToward(u.Pos, u.MoveTo, u.Speed)
	newPos.X = newPos.X.Clamp(0, w.MapSizeX)
	newPos.Y = newPos.Y.Clamp(0, w.MapSizeY)
	u.Pos = newPos
	if u.Pos.DistSq(u.MoveTo) <= fixed.Eps {
		u.State = UnitIdle
	}
}

func sortUnitsByID(w *World) {
	units := w.Units
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
