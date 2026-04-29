package sim

import "rts/internal/sim/fixed"

func stepBuilding(w *World, u *Unit) {
	if u.TargetID == 0 { u.State = UnitIdle; return }
	bld := w.FindBuilding(u.TargetID)
	if bld == nil || bld.State == BldDead { u.State = UnitIdle; u.TargetID = 0; return }

	newPos := fixed.MoveToward(u.Pos, bld.Pos, u.Speed)
	newPos.X = newPos.X.Clamp(0, w.MapSizeX)
	newPos.Y = newPos.Y.Clamp(0, w.MapSizeY)
	u.Pos = newPos

	ar := fixed.FromInt(int32(bld.SizeCells))
	if u.Pos.DistSq(bld.Pos) > ar.Mul(ar) { return }
	if bld.State != BldConstructing { u.State = UnitIdle; u.TargetID = 0; return }

	stats := BuildingStatTable[bld.Type]
	inc := fixed.One.Div(fixed.FromInt(int32(stats.BuildTicks)))
	bld.ConstructProgress = bld.ConstructProgress.Add(inc)
	bld.HP = bld.MaxHP.Mul(bld.ConstructProgress)
	if bld.HP <= 0 { bld.HP = fixed.One }

	if bld.ConstructProgress >= fixed.One {
		bld.State = BldReady
		bld.ConstructProgress = fixed.One
		bld.HP = bld.MaxHP
		u.State = UnitIdle
		u.TargetID = 0
	}
}

func tickConstruction(w *World) {}

func applyCmdBuild(w *World, cmd Cmd) {
	u := w.FindUnit(cmd.UnitID)
	if u == nil || u.State == UnitDead || u.Owner != cmd.Player || u.Type != UnitWorker { return }

	bldType := BuildingType(cmd.TargetID)
	stats, ok := BuildingStatTable[bldType]
	if !ok || bldType == BldHQ { return }
	if int(cmd.Player) >= len(w.Players) { return }
	if w.Players[cmd.Player].Crystal < stats.Cost { return }

	pos := cmd.TargetPos
	size := int32(stats.SizeCells)
	if pos.X.ToInt() < 0 || pos.Y.ToInt() < 0 ||
		pos.X.ToInt()+size > w.NavGrid.W || pos.Y.ToInt()+size > w.NavGrid.H { return }
	if !isAABBFree(w, pos, size) { return }

	w.Players[cmd.Player].Crystal = w.Players[cmd.Player].Crystal.Sub(stats.Cost)
	id := w.NextID; w.NextID++
	w.Buildings = append(w.Buildings, Building{
		ID: id, Owner: cmd.Player, Type: bldType, SizeCells: stats.SizeCells,
		Pos: pos, HP: fixed.One, MaxHP: stats.MaxHP, State: BldConstructing,
	})
	u.State = UnitBuilding
	u.TargetID = id
	u.MoveTo = pos
}

func isAABBFree(w *World, pos fixed.Vec2, size int32) bool {
	sx, sy := pos.X.ToInt(), pos.Y.ToInt()
	for i := range w.Buildings {
		b := &w.Buildings[i]
		if b.State == BldDead { continue }
		bx, by, bs := b.Pos.X.ToInt(), b.Pos.Y.ToInt(), int32(b.SizeCells)
		if sx < bx+bs && sx+size > bx && sy < by+bs && sy+size > by { return false }
	}
	return true
}
