package sim

import (
	"encoding/binary"
	"errors"
	"rts/internal/sim/fixed"
)

// Byte layout authority: docs/superpowers/plans/2026-04-19-sub2-phase0-foundation.md

const headerSize = 56 // tick+seed+randState+nextID+mapW+mapH+5 counts+navW+navH

// Marshal serialises the world to a deterministic byte sequence.
func Marshal(w *World) []byte {
	unitsTail := 0
	for i := range w.Units {
		unitsTail += len(w.Units[i].Path) * 8
	}
	bldsTail := 0
	for i := range w.Buildings {
		bldsTail += len(w.Buildings[i].ProductionQueue) * 9
	}
	navWords := 0
	if w.NavGrid != nil {
		navWords = len(w.NavGrid.Blocked)
	}

	total := headerSize +
		len(w.Units)*67 + unitsTail +
		len(w.Buildings)*40 + bldsTail +
		len(w.Crystals)*16 +
		len(w.Players)*6 +
		4 + navWords*8

	buf := make([]byte, total)
	off := 0

	binary.LittleEndian.PutUint32(buf[off:], w.Tick)
	off += 4
	binary.LittleEndian.PutUint64(buf[off:], w.Seed)
	off += 8
	binary.LittleEndian.PutUint64(buf[off:], w.Rand.state)
	off += 8
	binary.LittleEndian.PutUint32(buf[off:], w.NextID)
	off += 4
	binary.LittleEndian.PutUint32(buf[off:], uint32(w.MapSizeX.Raw()))
	off += 4
	binary.LittleEndian.PutUint32(buf[off:], uint32(w.MapSizeY.Raw()))
	off += 4
	binary.LittleEndian.PutUint32(buf[off:], uint32(len(w.Units)))
	off += 4
	binary.LittleEndian.PutUint32(buf[off:], uint32(len(w.Buildings)))
	off += 4
	binary.LittleEndian.PutUint32(buf[off:], uint32(len(w.Crystals)))
	off += 4
	binary.LittleEndian.PutUint32(buf[off:], uint32(len(w.Players)))
	off += 4
	if w.NavGrid != nil {
		binary.LittleEndian.PutUint32(buf[off:], uint32(w.NavGrid.W))
		off += 4
		binary.LittleEndian.PutUint32(buf[off:], uint32(w.NavGrid.H))
		off += 4
	} else {
		off += 8
	}

	for i := range w.Units {
		u := &w.Units[i]
		binary.LittleEndian.PutUint32(buf[off:], u.ID)
		off += 4
		buf[off] = u.Owner
		off++
		buf[off] = uint8(u.State)
		off++
		buf[off] = uint8(u.Type)
		off++
		binary.LittleEndian.PutUint32(buf[off:], uint32(u.HP.Raw()))
		off += 4
		binary.LittleEndian.PutUint32(buf[off:], uint32(u.MaxHP.Raw()))
		off += 4
		binary.LittleEndian.PutUint32(buf[off:], uint32(u.Speed.Raw()))
		off += 4
		binary.LittleEndian.PutUint32(buf[off:], uint32(u.Range.Raw()))
		off += 4
		binary.LittleEndian.PutUint32(buf[off:], uint32(u.Damage.Raw()))
		off += 4
		binary.LittleEndian.PutUint32(buf[off:], uint32(u.VisionRange.Raw()))
		off += 4
		binary.LittleEndian.PutUint32(buf[off:], uint32(u.CarryAmount.Raw()))
		off += 4
		binary.LittleEndian.PutUint32(buf[off:], uint32(u.Pos.X.Raw()))
		off += 4
		binary.LittleEndian.PutUint32(buf[off:], uint32(u.Pos.Y.Raw()))
		off += 4
		binary.LittleEndian.PutUint32(buf[off:], u.TargetID)
		off += 4
		binary.LittleEndian.PutUint32(buf[off:], uint32(u.MoveTo.X.Raw()))
		off += 4
		binary.LittleEndian.PutUint32(buf[off:], uint32(u.MoveTo.Y.Raw()))
		off += 4
		binary.LittleEndian.PutUint32(buf[off:], uint32(u.AttackMoveTarget.X.Raw()))
		off += 4
		binary.LittleEndian.PutUint32(buf[off:], uint32(u.AttackMoveTarget.Y.Raw()))
		off += 4
		binary.LittleEndian.PutUint32(buf[off:], uint32(len(u.Path)))
		off += 4
		for _, p := range u.Path {
			binary.LittleEndian.PutUint32(buf[off:], uint32(p.X.Raw()))
			off += 4
			binary.LittleEndian.PutUint32(buf[off:], uint32(p.Y.Raw()))
			off += 4
		}
	}

	for i := range w.Buildings {
		b := &w.Buildings[i]
		binary.LittleEndian.PutUint32(buf[off:], b.ID)
		off += 4
		buf[off] = b.Owner
		off++
		buf[off] = uint8(b.Type)
		off++
		buf[off] = uint8(b.State)
		off++
		buf[off] = b.SizeCells
		off++
		binary.LittleEndian.PutUint32(buf[off:], uint32(b.Pos.X.Raw()))
		off += 4
		binary.LittleEndian.PutUint32(buf[off:], uint32(b.Pos.Y.Raw()))
		off += 4
		binary.LittleEndian.PutUint32(buf[off:], uint32(b.HP.Raw()))
		off += 4
		binary.LittleEndian.PutUint32(buf[off:], uint32(b.MaxHP.Raw()))
		off += 4
		binary.LittleEndian.PutUint32(buf[off:], uint32(b.ConstructProgress.Raw()))
		off += 4
		binary.LittleEndian.PutUint32(buf[off:], uint32(b.RallyPoint.X.Raw()))
		off += 4
		binary.LittleEndian.PutUint32(buf[off:], uint32(b.RallyPoint.Y.Raw()))
		off += 4
		binary.LittleEndian.PutUint32(buf[off:], uint32(len(b.ProductionQueue)))
		off += 4
		for _, q := range b.ProductionQueue {
			buf[off] = uint8(q.UnitType)
			off++
			binary.LittleEndian.PutUint32(buf[off:], q.TicksLeft)
			off += 4
			binary.LittleEndian.PutUint32(buf[off:], q.StartTick)
			off += 4
		}
	}

	for i := range w.Crystals {
		c := &w.Crystals[i]
		binary.LittleEndian.PutUint32(buf[off:], c.ID)
		off += 4
		binary.LittleEndian.PutUint32(buf[off:], uint32(c.Pos.X.Raw()))
		off += 4
		binary.LittleEndian.PutUint32(buf[off:], uint32(c.Pos.Y.Raw()))
		off += 4
		binary.LittleEndian.PutUint32(buf[off:], uint32(c.Remaining.Raw()))
		off += 4
	}

	for i := range w.Players {
		p := &w.Players[i]
		buf[off] = p.ID
		off++
		binary.LittleEndian.PutUint32(buf[off:], uint32(p.Crystal.Raw()))
		off += 4
		if p.Surrendered {
			buf[off] = 1
		} else {
			buf[off] = 0
		}
		off++
	}

	binary.LittleEndian.PutUint32(buf[off:], uint32(navWords))
	off += 4
	if w.NavGrid != nil {
		for _, word := range w.NavGrid.Blocked {
			binary.LittleEndian.PutUint64(buf[off:], word)
			off += 8
		}
	}

	return buf[:off]
}

// Unmarshal restores a world from bytes produced by Marshal.
func Unmarshal(data []byte) (*World, error) {
	if len(data) < headerSize {
		return nil, errors.New("snapshot too short")
	}

	off := 0
	tick := binary.LittleEndian.Uint32(data[off:])
	off += 4
	seed := binary.LittleEndian.Uint64(data[off:])
	off += 8
	randState := binary.LittleEndian.Uint64(data[off:])
	off += 8
	nextID := binary.LittleEndian.Uint32(data[off:])
	off += 4
	mapW := fixed.FromRaw(int32(binary.LittleEndian.Uint32(data[off:])))
	off += 4
	mapH := fixed.FromRaw(int32(binary.LittleEndian.Uint32(data[off:])))
	off += 4
	unitCount := binary.LittleEndian.Uint32(data[off:])
	off += 4
	bldCount := binary.LittleEndian.Uint32(data[off:])
	off += 4
	crystalCount := binary.LittleEndian.Uint32(data[off:])
	off += 4
	playerCount := binary.LittleEndian.Uint32(data[off:])
	off += 4
	navW := int32(binary.LittleEndian.Uint32(data[off:]))
	off += 4
	navH := int32(binary.LittleEndian.Uint32(data[off:]))
	off += 4

	w := &World{
		Tick:      tick,
		Seed:      seed,
		Rand:      &SplitMix64{state: randState},
		NextID:    nextID,
		MapSizeX:  mapW,
		MapSizeY:  mapH,
		Units:     make([]Unit, unitCount),
		Buildings: make([]Building, bldCount),
		Crystals:  make([]Crystal, crystalCount),
		Players:   make([]Player, playerCount),
	}

	for i := range w.Units {
		u := &w.Units[i]
		u.ID = binary.LittleEndian.Uint32(data[off:])
		off += 4
		u.Owner = data[off]
		off++
		u.State = UnitState(data[off])
		off++
		u.Type = UnitType(data[off])
		off++
		u.HP = fixed.FromRaw(int32(binary.LittleEndian.Uint32(data[off:])))
		off += 4
		u.MaxHP = fixed.FromRaw(int32(binary.LittleEndian.Uint32(data[off:])))
		off += 4
		u.Speed = fixed.FromRaw(int32(binary.LittleEndian.Uint32(data[off:])))
		off += 4
		u.Range = fixed.FromRaw(int32(binary.LittleEndian.Uint32(data[off:])))
		off += 4
		u.Damage = fixed.FromRaw(int32(binary.LittleEndian.Uint32(data[off:])))
		off += 4
		u.VisionRange = fixed.FromRaw(int32(binary.LittleEndian.Uint32(data[off:])))
		off += 4
		u.CarryAmount = fixed.FromRaw(int32(binary.LittleEndian.Uint32(data[off:])))
		off += 4
		u.Pos.X = fixed.FromRaw(int32(binary.LittleEndian.Uint32(data[off:])))
		off += 4
		u.Pos.Y = fixed.FromRaw(int32(binary.LittleEndian.Uint32(data[off:])))
		off += 4
		u.TargetID = binary.LittleEndian.Uint32(data[off:])
		off += 4
		u.MoveTo.X = fixed.FromRaw(int32(binary.LittleEndian.Uint32(data[off:])))
		off += 4
		u.MoveTo.Y = fixed.FromRaw(int32(binary.LittleEndian.Uint32(data[off:])))
		off += 4
		u.AttackMoveTarget.X = fixed.FromRaw(int32(binary.LittleEndian.Uint32(data[off:])))
		off += 4
		u.AttackMoveTarget.Y = fixed.FromRaw(int32(binary.LittleEndian.Uint32(data[off:])))
		off += 4
		pathLen := binary.LittleEndian.Uint32(data[off:])
		off += 4
		if pathLen > 0 {
			u.Path = make([]fixed.Vec2, pathLen)
			for j := range u.Path {
				u.Path[j].X = fixed.FromRaw(int32(binary.LittleEndian.Uint32(data[off:])))
				off += 4
				u.Path[j].Y = fixed.FromRaw(int32(binary.LittleEndian.Uint32(data[off:])))
				off += 4
			}
		}
	}

	for i := range w.Buildings {
		b := &w.Buildings[i]
		b.ID = binary.LittleEndian.Uint32(data[off:])
		off += 4
		b.Owner = data[off]
		off++
		b.Type = BuildingType(data[off])
		off++
		b.State = BuildingState(data[off])
		off++
		b.SizeCells = data[off]
		off++
		b.Pos.X = fixed.FromRaw(int32(binary.LittleEndian.Uint32(data[off:])))
		off += 4
		b.Pos.Y = fixed.FromRaw(int32(binary.LittleEndian.Uint32(data[off:])))
		off += 4
		b.HP = fixed.FromRaw(int32(binary.LittleEndian.Uint32(data[off:])))
		off += 4
		b.MaxHP = fixed.FromRaw(int32(binary.LittleEndian.Uint32(data[off:])))
		off += 4
		b.ConstructProgress = fixed.FromRaw(int32(binary.LittleEndian.Uint32(data[off:])))
		off += 4
		b.RallyPoint.X = fixed.FromRaw(int32(binary.LittleEndian.Uint32(data[off:])))
		off += 4
		b.RallyPoint.Y = fixed.FromRaw(int32(binary.LittleEndian.Uint32(data[off:])))
		off += 4
		qLen := binary.LittleEndian.Uint32(data[off:])
		off += 4
		if qLen > 0 {
			b.ProductionQueue = make([]QueueItem, qLen)
			for j := range b.ProductionQueue {
				b.ProductionQueue[j].UnitType = UnitType(data[off])
				off++
				b.ProductionQueue[j].TicksLeft = binary.LittleEndian.Uint32(data[off:])
				off += 4
				b.ProductionQueue[j].StartTick = binary.LittleEndian.Uint32(data[off:])
				off += 4
			}
		}
	}

	for i := range w.Crystals {
		c := &w.Crystals[i]
		c.ID = binary.LittleEndian.Uint32(data[off:])
		off += 4
		c.Pos.X = fixed.FromRaw(int32(binary.LittleEndian.Uint32(data[off:])))
		off += 4
		c.Pos.Y = fixed.FromRaw(int32(binary.LittleEndian.Uint32(data[off:])))
		off += 4
		c.Remaining = fixed.FromRaw(int32(binary.LittleEndian.Uint32(data[off:])))
		off += 4
	}

	for i := range w.Players {
		p := &w.Players[i]
		p.ID = data[off]
		off++
		p.Crystal = fixed.FromRaw(int32(binary.LittleEndian.Uint32(data[off:])))
		off += 4
		p.Surrendered = data[off] != 0
		off++
	}

	blockedWords := binary.LittleEndian.Uint32(data[off:])
	off += 4
	nav := &NavGrid{W: navW, H: navH, Blocked: make([]uint64, blockedWords)}
	for i := range nav.Blocked {
		nav.Blocked[i] = binary.LittleEndian.Uint64(data[off:])
		off += 8
	}
	w.NavGrid = nav

	return w, nil
}
