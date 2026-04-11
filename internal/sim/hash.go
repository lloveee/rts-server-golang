package sim

import (
	"encoding/binary"
)

// Hash computes a canonical 64-bit hash of the world state.
// Deterministic: same state = same hash, regardless of slice capacity/etc.
//
// Algorithm: xxhash64-style mixing of (tick, unit count, then each unit
// in ID-sorted order with fields in a fixed layout).
//
// We use a simple FNV-1a-64 variant here for simplicity and zero dependencies.
// Can be swapped to xxhash64 for speed later without affecting correctness
// (as long as both sides use the same implementation).
func Hash(w *World) uint64 {
	var h uint64 = 14695981039346656037 // FNV offset basis

	// Mix tick and unit count.
	var buf [8]byte
	binary.LittleEndian.PutUint32(buf[:4], w.Tick)
	h = fnvMix(h, buf[:4])

	binary.LittleEndian.PutUint32(buf[:4], uint32(len(w.Units)))
	h = fnvMix(h, buf[:4])

	// Mix each unit in ID order (units are already sorted by step).
	for i := range w.Units {
		u := &w.Units[i]
		h = hashUnit(h, u)
	}

	return h
}

func hashUnit(h uint64, u *Unit) uint64 {
	// Fixed-layout: ID(4) + Owner(1) + State(1) + HP(4) + PosX(4) + PosY(4) + TargetID(4) = 22 bytes
	var buf [22]byte
	binary.LittleEndian.PutUint32(buf[0:4], u.ID)
	buf[4] = u.Owner
	buf[5] = uint8(u.State)
	binary.LittleEndian.PutUint32(buf[6:10], uint32(u.HP.Raw()))
	binary.LittleEndian.PutUint32(buf[10:14], uint32(u.Pos.X.Raw()))
	binary.LittleEndian.PutUint32(buf[14:18], uint32(u.Pos.Y.Raw()))
	binary.LittleEndian.PutUint32(buf[18:22], u.TargetID)
	return fnvMix(h, buf[:])
}

// fnvMix applies FNV-1a mixing to h with the given bytes.
func fnvMix(h uint64, data []byte) uint64 {
	const prime = 1099511628211
	for _, b := range data {
		h ^= uint64(b)
		h *= prime
	}
	return h
}
