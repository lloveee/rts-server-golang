package sim

import (
	"encoding/binary"
	"sort"
)

// Hash computes a canonical 64-bit hash of the world state.
// FNV-1a-64 over a fixed byte layout. Byte-layout authority:
// docs/superpowers/plans/2026-04-19-sub2-phase0-foundation.md
func Hash(w *World) uint64 {
	var h uint64 = 14695981039346656037 // FNV offset basis

	var buf [8]byte
	binary.LittleEndian.PutUint32(buf[:4], w.Tick)
	h = fnvMix(h, buf[:4])

	// Units — already sorted by Step; mix count then each unit.
	binary.LittleEndian.PutUint32(buf[:4], uint32(len(w.Units)))
	h = fnvMix(h, buf[:4])
	for i := range w.Units {
		h = hashUnit(h, &w.Units[i])
	}

	// Buildings — sort a copy by ID (defensive; Step also sorts pre-step).
	h = hashBuildings(h, w.Buildings)
	h = hashCrystals(h, w.Crystals)
	h = hashPlayers(h, w.Players)
	h = hashNavGrid(h, w.NavGrid)

	return h
}

func hashUnit(h uint64, u *Unit) uint64 {
	// 51-byte fixed block: see plan doc for field order.
	var buf [51]byte
	binary.LittleEndian.PutUint32(buf[0:4], u.ID)
	buf[4] = u.Owner
	buf[5] = uint8(u.State)
	buf[6] = uint8(u.Type)
	binary.LittleEndian.PutUint32(buf[7:11], uint32(u.HP.Raw()))
	binary.LittleEndian.PutUint32(buf[11:15], uint32(u.Range.Raw()))
	binary.LittleEndian.PutUint32(buf[15:19], uint32(u.Damage.Raw()))
	binary.LittleEndian.PutUint32(buf[19:23], uint32(u.VisionRange.Raw()))
	binary.LittleEndian.PutUint32(buf[23:27], uint32(u.CarryAmount.Raw()))
	binary.LittleEndian.PutUint32(buf[27:31], uint32(u.Pos.X.Raw()))
	binary.LittleEndian.PutUint32(buf[31:35], uint32(u.Pos.Y.Raw()))
	binary.LittleEndian.PutUint32(buf[35:39], u.TargetID)
	binary.LittleEndian.PutUint32(buf[39:43], uint32(u.AttackMoveTarget.X.Raw()))
	binary.LittleEndian.PutUint32(buf[43:47], uint32(u.AttackMoveTarget.Y.Raw()))
	binary.LittleEndian.PutUint32(buf[47:51], uint32(len(u.Path)))
	h = fnvMix(h, buf[:])

	if len(u.Path) > 0 {
		var ptBuf [8]byte
		for i := range u.Path {
			binary.LittleEndian.PutUint32(ptBuf[0:4], uint32(u.Path[i].X.Raw()))
			binary.LittleEndian.PutUint32(ptBuf[4:8], uint32(u.Path[i].Y.Raw()))
			h = fnvMix(h, ptBuf[:])
		}
	}
	return h
}

func hashBuildings(h uint64, in []Building) uint64 {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], uint32(len(in)))
	h = fnvMix(h, buf[:])

	sorted := make([]Building, len(in))
	copy(sorted, in)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })

	for i := range sorted {
		h = hashBuilding(h, &sorted[i])
	}
	return h
}

func hashBuilding(h uint64, b *Building) uint64 {
	var buf [40]byte
	binary.LittleEndian.PutUint32(buf[0:4], b.ID)
	buf[4] = b.Owner
	buf[5] = uint8(b.Type)
	buf[6] = uint8(b.State)
	buf[7] = b.SizeCells
	binary.LittleEndian.PutUint32(buf[8:12], uint32(b.Pos.X.Raw()))
	binary.LittleEndian.PutUint32(buf[12:16], uint32(b.Pos.Y.Raw()))
	binary.LittleEndian.PutUint32(buf[16:20], uint32(b.HP.Raw()))
	binary.LittleEndian.PutUint32(buf[20:24], uint32(b.MaxHP.Raw()))
	binary.LittleEndian.PutUint32(buf[24:28], uint32(b.ConstructProgress.Raw()))
	binary.LittleEndian.PutUint32(buf[28:32], uint32(b.RallyPoint.X.Raw()))
	binary.LittleEndian.PutUint32(buf[32:36], uint32(b.RallyPoint.Y.Raw()))
	binary.LittleEndian.PutUint32(buf[36:40], uint32(len(b.ProductionQueue)))
	h = fnvMix(h, buf[:])

	var qBuf [9]byte
	for i := range b.ProductionQueue {
		q := &b.ProductionQueue[i]
		qBuf[0] = uint8(q.UnitType)
		binary.LittleEndian.PutUint32(qBuf[1:5], q.TicksLeft)
		binary.LittleEndian.PutUint32(qBuf[5:9], q.StartTick)
		h = fnvMix(h, qBuf[:])
	}
	return h
}

func hashCrystals(h uint64, in []Crystal) uint64 {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], uint32(len(in)))
	h = fnvMix(h, buf[:])

	sorted := make([]Crystal, len(in))
	copy(sorted, in)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })

	for i := range sorted {
		h = hashCrystal(h, &sorted[i])
	}
	return h
}

func hashCrystal(h uint64, c *Crystal) uint64 {
	var buf [16]byte
	binary.LittleEndian.PutUint32(buf[0:4], c.ID)
	binary.LittleEndian.PutUint32(buf[4:8], uint32(c.Pos.X.Raw()))
	binary.LittleEndian.PutUint32(buf[8:12], uint32(c.Pos.Y.Raw()))
	binary.LittleEndian.PutUint32(buf[12:16], uint32(c.Remaining.Raw()))
	return fnvMix(h, buf[:])
}

func hashPlayers(h uint64, in []Player) uint64 {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], uint32(len(in)))
	h = fnvMix(h, buf[:])

	sorted := make([]Player, len(in))
	copy(sorted, in)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })

	for i := range sorted {
		h = hashPlayer(h, &sorted[i])
	}
	return h
}

func hashPlayer(h uint64, p *Player) uint64 {
	var buf [6]byte
	buf[0] = p.ID
	binary.LittleEndian.PutUint32(buf[1:5], uint32(p.Crystal.Raw()))
	if p.Surrendered {
		buf[5] = 1
	}
	return fnvMix(h, buf[:])
}

func hashNavGrid(h uint64, g *NavGrid) uint64 {
	// Nil NavGrid (e.g. from a Sub-1 snapshot): treat as 0×0 with 0 words.
	if g == nil {
		var hdr [12]byte // all zeros: W=0, H=0, len=0
		return fnvMix(h, hdr[:])
	}
	var hdr [12]byte
	binary.LittleEndian.PutUint32(hdr[0:4], uint32(g.W))
	binary.LittleEndian.PutUint32(hdr[4:8], uint32(g.H))
	binary.LittleEndian.PutUint32(hdr[8:12], uint32(len(g.Blocked)))
	h = fnvMix(h, hdr[:])

	var wBuf [8]byte
	for _, word := range g.Blocked {
		binary.LittleEndian.PutUint64(wBuf[:], word)
		h = fnvMix(h, wBuf[:])
	}
	return h
}

func fnvMix(h uint64, data []byte) uint64 {
	const prime = 1099511628211
	for _, b := range data {
		h ^= uint64(b)
		h *= prime
	}
	return h
}
