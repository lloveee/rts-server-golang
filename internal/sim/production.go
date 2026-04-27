package sim

import "rts/internal/sim/fixed"

// tickProduction advances each Ready building's production queue by one tick.
// When the head item's TicksLeft reaches 0, the unit is spawned at the first
// free edge cell of the building's AABB.
func tickProduction(w *World) {
	for i := range w.Buildings {
		b := &w.Buildings[i]
		if b.State != BldReady {
			continue
		}
		if len(b.ProductionQueue) == 0 {
			continue
		}

		q := &b.ProductionQueue[0]
		if q.TicksLeft > 0 {
			q.TicksLeft--
		}
		if q.TicksLeft > 0 {
			continue
		}

		spawnPos := findEdgeSpawnCell(w, b)
		if spawnPos == nil {
			continue // all edge cells occupied, retry next tick
		}

		stats := UnitStatTable[q.UnitType]
		unitID := w.NextID
		w.NextID++
		u := Unit{
			ID:               unitID,
			Owner:            b.Owner,
			Type:             q.UnitType,
			Pos:              *spawnPos,
			HP:               stats.MaxHP,
			MaxHP:            stats.MaxHP,
			Speed:            stats.Speed,
			Range:            stats.Range,
			Damage:           stats.Damage,
			VisionRange:      stats.VisionRange,
			State:            UnitIdle,
			AttackMoveTarget: fixed.VInt(0, 0),
		}

		// If rally point is set, auto-command Move.
		if b.RallyPoint.X != 0 || b.RallyPoint.Y != 0 {
			u.State = UnitMoving
			u.MoveTo = b.RallyPoint
		}

		w.Units = append(w.Units, u)

		// Dequeue the completed item.
		b.ProductionQueue = b.ProductionQueue[1:]
	}
}

// findEdgeSpawnCell finds a free cell on the perimeter of the building's AABB.
// Scans all edge cells (where at least one coordinate is on the AABB boundary).
func findEdgeSpawnCell(w *World, b *Building) *fixed.Vec2 {
	size := int32(b.SizeCells)
	startX := b.Pos.X.ToInt()
	startY := b.Pos.Y.ToInt()

	for dx := int32(0); dx < size; dx++ {
		for dy := int32(0); dy < size; dy++ {
			// Only check edge cells (at least one side on perimeter).
			if dx > 0 && dx < size-1 && dy > 0 && dy < size-1 {
				continue
			}
			cx := startX + dx
			cy := startY + dy
			if cx < 0 || cy < 0 || cx >= w.NavGrid.W || cy >= w.NavGrid.H {
				continue
			}
			pos := fixed.VInt(cx, cy)
			if !isCellBlocked(w, cx, cy) {
				return &pos
			}
		}
	}
	return nil
}

// isCellBlocked checks if a cell is occupied by a building or blocked in NavGrid.
func isCellBlocked(w *World, cx, cy int32) bool {
	for _, b := range w.Buildings {
		if b.State == BldDead {
			continue
		}
		bx := b.Pos.X.ToInt()
		by := b.Pos.Y.ToInt()
		sz := int32(b.SizeCells)
		if cx >= bx && cx < bx+sz && cy >= by && cy < by+sz {
			return true
		}
	}
	idx := int(cy*w.NavGrid.W + cx)
	word := idx / 64
	bit := uint(idx % 64)
	if word < len(w.NavGrid.Blocked) && (w.NavGrid.Blocked[word]&(1<<bit)) != 0 {
		return true
	}
	return false
}
