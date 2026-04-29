package sim

import "rts/internal/sim/fixed"

// DmgEvent records damage to be applied in the apply phase.
type DmgEvent struct {
	Target uint32
	Dmg    fixed.Fix32
}

// resolveCombat collects damage events from all attacking units, then applies them.
func resolveCombat(w *World) {
	var events []DmgEvent

	for i := range w.Units {
		u := &w.Units[i]
		if u.State == UnitDead || u.State != UnitAttacking {
			continue
		}
		if u.Range <= 0 || u.TargetID == 0 {
			continue
		}

		target := w.FindEntityAny(u.TargetID)
		if target == nil || target.IsDead() {
			u.TargetID = 0
			continue
		}

		distSq := u.Pos.DistSq(target.GetPos())
		rangeSq := u.Range.Mul(u.Range)
		if distSq <= rangeSq {
			events = append(events, DmgEvent{Target: u.TargetID, Dmg: u.Damage})
		}
	}

	for _, e := range events {
		ent := w.FindEntityAny(e.Target)
		if ent == nil || ent.IsDead() {
			continue
		}
		ent.SetHP(ent.GetHP().Sub(e.Dmg))
		if ent.GetHP() <= 0 {
			ent.SetDead()
		}
	}
}

// applyPushAway resolves unit-unit overlaps deterministically.
// Lower-ID unit is pushed away from the higher-ID unit.
func applyPushAway(w *World) {
	const unitRadius = fixed.Fix32(1 << 15) // 0.5 cells
	const pushStep = fixed.Fix32(6554)      // 0.1 cells
	minDist := unitRadius.Mul(fixed.FromInt(2))
	minDistSq := minDist.Mul(minDist)

	for i := 0; i < len(w.Units); i++ {
		ui := &w.Units[i]
		if ui.State == UnitDead {
			continue
		}
		for j := i + 1; j < len(w.Units); j++ {
			uj := &w.Units[j]
			if uj.State == UnitDead {
				continue
			}
			dSq := ui.Pos.DistSq(uj.Pos)
			if dSq >= minDistSq {
				continue
			}
			dir := ui.Pos.Sub(uj.Pos)
			if dir.X == 0 && dir.Y == 0 {
				dir = fixed.VInt(1, 0)
			} else {
				dir = dir.Normalize()
			}
			ui.Pos = ui.Pos.Add(dir.Scale(pushStep))
			ui.Pos.X = ui.Pos.X.Clamp(0, w.MapSizeX)
			ui.Pos.Y = ui.Pos.Y.Clamp(0, w.MapSizeY)
		}
	}
}

// stepAttackOrAdvance handles UnitAttacking: direct attack + attack-move with auto-scan.
func stepAttackOrAdvance(w *World, u *Unit) {
	if u.Range <= 0 {
		u.State = UnitIdle
		return
	}

	if u.TargetID != 0 {
		target := w.FindEntityAny(u.TargetID)
		if target != nil && !target.IsDead() {
			distSq := u.Pos.DistSq(target.GetPos())
			rangeSq := u.Range.Mul(u.Range)
			if distSq > rangeSq {
				newPos := fixed.MoveToward(u.Pos, target.GetPos(), u.Speed)
				newPos.X = newPos.X.Clamp(0, w.MapSizeX)
				newPos.Y = newPos.Y.Clamp(0, w.MapSizeY)
				u.Pos = newPos
			}
			return
		}
		u.TargetID = 0
	}

	if u.AttackMoveTarget.X != 0 || u.AttackMoveTarget.Y != 0 {
		scanRange := u.Range.Mul(u.Range).Mul(fixed.FromFloat64(2.25))
		nearest := findNearestEnemy(w, u.Pos, u.Owner, scanRange)
		if nearest != nil {
			u.TargetID = nearest.GetID()
			return
		}

		newPos := fixed.MoveToward(u.Pos, u.AttackMoveTarget, u.Speed)
		newPos.X = newPos.X.Clamp(0, w.MapSizeX)
		newPos.Y = newPos.Y.Clamp(0, w.MapSizeY)
		u.Pos = newPos

		if u.Pos.DistSq(u.AttackMoveTarget) <= fixed.Eps {
			u.AttackMoveTarget = fixed.VInt(0, 0)
			u.State = UnitIdle
		}
		return
	}

	u.State = UnitIdle
}

// findNearestEnemy returns the nearest non-dead entity owned by a different player.
func findNearestEnemy(w *World, pos fixed.Vec2, owner uint8, maxDistSq fixed.Fix32) Entity {
	var best Entity
	var bestDistSq fixed.Fix32
	for i := range w.Units {
		u := &w.Units[i]
		if u.State == UnitDead || u.Owner == owner {
			continue
		}
		dSq := pos.DistSq(u.Pos)
		if dSq <= maxDistSq && (best == nil || dSq < bestDistSq) {
			best = u
			bestDistSq = dSq
		}
	}
	return best
}
