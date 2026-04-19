package sim

import (
	"encoding/binary"
	"errors"
	"rts/internal/sim/fixed"
)

// Marshal serializes the world to a deterministic byte sequence.
// Format:
//   tick(4) + seed(8) + rand_state(8) + next_id(4) + mapW(4) + mapH(4)
//   + unit_count(4) + [unit_count × unit_bytes]
// Unit: id(4) + owner(1) + state(1) + hp(4) + max_hp(4) + speed(4) + pos_x(4) + pos_y(4)
//       + target_id(4) + move_to_x(4) + move_to_y(4) = 38 bytes
func Marshal(w *World) []byte {
	headerSize := 4 + 8 + 8 + 4 + 4 + 4 + 4 // 36 bytes
	unitSize := 38
	buf := make([]byte, headerSize+len(w.Units)*unitSize)

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

	for i := range w.Units {
		u := &w.Units[i]
		binary.LittleEndian.PutUint32(buf[off:], u.ID)
		off += 4
		buf[off] = u.Owner
		off++
		buf[off] = uint8(u.State)
		off++
		binary.LittleEndian.PutUint32(buf[off:], uint32(u.HP.Raw()))
		off += 4
		binary.LittleEndian.PutUint32(buf[off:], uint32(u.MaxHP.Raw()))
		off += 4
		binary.LittleEndian.PutUint32(buf[off:], uint32(u.Speed.Raw()))
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
	}

	return buf[:off]
}

// Unmarshal restores a world from bytes produced by Marshal.
func Unmarshal(data []byte) (*World, error) {
	if len(data) < 36 {
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

	unitSize := 38
	expected := 36 + int(unitCount)*unitSize
	if len(data) < expected {
		return nil, errors.New("snapshot truncated: not enough unit data")
	}

	w := &World{
		Tick:      tick,
		Seed:      seed,
		Rand:      &SplitMix64{state: randState},
		NextID:    nextID,
		MapSizeX:  mapW,
		MapSizeY:  mapH,
		Units:     make([]Unit, unitCount),
		Buildings: make([]Building, 0),
		Crystals:  make([]Crystal, 0),
		Players:   make([]Player, 0),
		NavGrid:   NewNavGrid(mapW.ToInt(), mapH.ToInt()),
	}

	for i := range w.Units {
		u := &w.Units[i]
		u.ID = binary.LittleEndian.Uint32(data[off:])
		off += 4
		u.Owner = data[off]
		off++
		u.State = UnitState(data[off])
		off++
		u.HP = fixed.FromRaw(int32(binary.LittleEndian.Uint32(data[off:])))
		off += 4
		u.MaxHP = fixed.FromRaw(int32(binary.LittleEndian.Uint32(data[off:])))
		off += 4
		u.Speed = fixed.FromRaw(int32(binary.LittleEndian.Uint32(data[off:])))
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
	}

	return w, nil
}
