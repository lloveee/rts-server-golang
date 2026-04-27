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

// findEdgeSpawnCell finds a free cell immediately outside the building's AABB.
// Scans the one-cell perimeter ring outside the AABB on all four sides (including corners).
func findEdgeSpawnCell(w *World, b *Building) *fixed.Vec2 {
	size := int32(b.SizeCells)
	startX := b.Pos.X.ToInt()
	startY := b.Pos.Y.ToInt()

	// Scan cells from one row/col before the AABB to one row/col after.
	// Skip cells strictly inside the AABB (they are occupied by the building).
	for dx := int32(-1); dx <= size; dx++ {
		for dy := int32(-1); dy <= size; dy++ {
			if dx >= 0 && dx < size && dy >= 0 && dy < size {
				continue // inside the building AABB
			}
			cx := startX + dx
			cy := startY + dy
			if cx < 0 || cy < 0 || cx >= w.NavGrid.W || cy >= w.NavGrid.H {
				continue
			}
			if !isCellBlocked(w, cx, cy) {
				pos := fixed.VInt(cx, cy)
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
