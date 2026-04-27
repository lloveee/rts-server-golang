package sim

import "rts/internal/sim/fixed"

// stepMining processes one tick for a worker in Mining state.
func stepMining(w *World, u *Unit) {
	if u.TargetID == 0 {
		u.State = UnitIdle
		return
	}

	crystal := w.FindCrystal(u.TargetID)
	if crystal == nil || crystal.Remaining <= 0 {
		nearest := w.FindNearestCrystal(u.Pos)
		if nearest == nil {
			u.State = UnitIdle
			u.TargetID = 0
			return
		}
		u.TargetID = nearest.ID
		crystal = nearest
	}

	// Internal mining timer stored in CarryAmount as a ticks-left counter.
	if u.CarryAmount <= 0 {
		u.CarryAmount = fixed.FromInt(MiningTicksPerTrip)
	}

	u.CarryAmount = u.CarryAmount.Sub(fixed.One)
	if u.CarryAmount > 0 {
		return
	}

	// Mining complete — take resources.
	take := fixed.FromInt(CarryCapacity)
	if crystal.Remaining < take {
		take = crystal.Remaining
	}
	crystal.Remaining = crystal.Remaining.Sub(take)
	u.CarryAmount = take

	hq := w.FindNearestOwnHQ(u.Pos, u.Owner)
	if hq == nil {
		u.State = UnitIdle
		u.CarryAmount = 0
		return
	}

	u.State = UnitReturning
	u.TargetID = hq.ID
	u.MoveTo = hq.Pos
}

// stepReturning processes one tick for a worker in Returning state.
func stepReturning(w *World, u *Unit) {
	if u.TargetID == 0 {
		u.State = UnitIdle
		return
	}

	hq := w.FindBuilding(u.TargetID)
	if hq == nil || hq.State == BldDead || hq.Owner != u.Owner {
		hq = w.FindNearestOwnHQ(u.Pos, u.Owner)
		if hq == nil {
			u.State = UnitIdle
			u.CarryAmount = 0
			return
		}
		u.TargetID = hq.ID
		u.MoveTo = hq.Pos
	}

	newPos := fixed.MoveToward(u.Pos, u.MoveTo, u.Speed)
	newPos.X = newPos.X.Clamp(0, w.MapSizeX)
	newPos.Y = newPos.Y.Clamp(0, w.MapSizeY)
	u.Pos = newPos

	arrivalRange := fixed.FromInt(int32(hq.SizeCells))
	arrivalRangeSq := arrivalRange.Mul(arrivalRange)
	if u.Pos.DistSq(hq.Pos) <= arrivalRangeSq {
		if u.Owner < uint8(len(w.Players)) {
			w.Players[u.Owner].Crystal = w.Players[u.Owner].Crystal.Add(u.CarryAmount)
		}
		u.CarryAmount = 0

		nearest := w.FindNearestCrystal(u.Pos)
		if nearest == nil {
			u.State = UnitIdle
			u.TargetID = 0
			return
		}
		u.State = UnitMining
		u.TargetID = nearest.ID
		u.MoveTo = nearest.Pos
		u.CarryAmount = 0
	}
}

// FindCrystal returns the crystal with the given ID, or nil.
func (w *World) FindCrystal(id uint32) *Crystal {
	for i := range w.Crystals {
		if w.Crystals[i].ID == id && w.Crystals[i].Remaining > 0 {
			return &w.Crystals[i]
		}
	}
	return nil
}

// FindBuilding returns the building with the given ID, or nil.
func (w *World) FindBuilding(id uint32) *Building {
	for i := range w.Buildings {
		if w.Buildings[i].ID == id && w.Buildings[i].State != BldDead {
			return &w.Buildings[i]
		}
	}
	return nil
}

// FindNearestCrystal returns the nearest crystal with Remaining > 0.
func (w *World) FindNearestCrystal(pos fixed.Vec2) *Crystal {
	var best *Crystal
	var bestDistSq fixed.Fix32
	for i := range w.Crystals {
		c := &w.Crystals[i]
		if c.Remaining <= 0 {
			continue
		}
		dSq := pos.DistSq(c.Pos)
		if best == nil || dSq < bestDistSq {
			best = c
			bestDistSq = dSq
		}
	}
	return best
}

// FindNearestOwnHQ returns the nearest Ready HQ owned by the given player.
func (w *World) FindNearestOwnHQ(pos fixed.Vec2, owner uint8) *Building {
	var best *Building
	var bestDistSq fixed.Fix32
	for i := range w.Buildings {
		b := &w.Buildings[i]
		if b.Owner != owner || b.Type != BldHQ || b.State != BldReady {
			continue
		}
		dSq := pos.DistSq(b.Pos)
		if best == nil || dSq < bestDistSq {
			best = b
			bestDistSq = dSq
		}
	}
	return best
}
