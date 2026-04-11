// Package replay implements recording and playback of lockstep game sessions.
//
// File format:
//
//	Header:
//	  magic[6]     "RPLY1\0"
//	  proto_ver(2) wire protocol version
//	  seed(8)      world seed
//	  tick_rate(2) ticks per second
//	  start_ts(8)  game start unix timestamp (seconds)
//	  players(1)   player count
//	  mapW(4)      map width (fixed raw)
//	  mapH(4)      map height (fixed raw)
//	  snap_len(4)  initial snapshot length
//	  snapshot[]   initial world snapshot
//
//	Tick stream (repeated):
//	  tick(4)      tick number
//	  cmd_count(2) number of commands
//	  cmds[]       each cmd is 22 bytes (player+op+unitid+tx+ty+targetid)
//	  hash(8)      state hash after this tick
package replay

import (
	"encoding/binary"
	"rts/internal/wire"
)

var Magic = [6]byte{'R', 'P', 'L', 'Y', '1', 0}

// Header is the replay file header.
type Header struct {
	ProtoVersion uint16
	Seed         uint64
	TickRate     uint16
	StartTS      int64 // unix seconds
	PlayerCount  uint8
	MapW         int32
	MapH         int32
	Snapshot     []byte // initial world snapshot
}

// TickRecord is one tick's data in the replay stream.
type TickRecord struct {
	Tick uint32
	Cmds []wire.Cmd
	Hash uint64
}

const cmdRecordSize = 22 // player(1) + op(1) + unitid(4) + tx(4) + ty(4) + targetid(4) + tick(4)

// MarshalHeader serializes the header.
func MarshalHeader(h *Header) []byte {
	// 6 + 2 + 8 + 2 + 8 + 1 + 4 + 4 + 4 = 39 + snapshot
	buf := make([]byte, 39+len(h.Snapshot))
	copy(buf[0:6], Magic[:])
	binary.LittleEndian.PutUint16(buf[6:], h.ProtoVersion)
	binary.LittleEndian.PutUint64(buf[8:], h.Seed)
	binary.LittleEndian.PutUint16(buf[16:], h.TickRate)
	binary.LittleEndian.PutUint64(buf[18:], uint64(h.StartTS))
	buf[26] = h.PlayerCount
	binary.LittleEndian.PutUint32(buf[27:], uint32(h.MapW))
	binary.LittleEndian.PutUint32(buf[31:], uint32(h.MapH))
	binary.LittleEndian.PutUint32(buf[35:], uint32(len(h.Snapshot)))
	copy(buf[39:], h.Snapshot)
	return buf
}

// UnmarshalHeader deserializes the header, returning the header and bytes consumed.
func UnmarshalHeader(data []byte) (*Header, int, error) {
	if len(data) < 39 {
		return nil, 0, ErrTruncated
	}
	if data[0] != Magic[0] || data[1] != Magic[1] || data[2] != Magic[2] ||
		data[3] != Magic[3] || data[4] != Magic[4] || data[5] != Magic[5] {
		return nil, 0, ErrBadMagic
	}
	h := &Header{
		ProtoVersion: binary.LittleEndian.Uint16(data[6:]),
		Seed:         binary.LittleEndian.Uint64(data[8:]),
		TickRate:     binary.LittleEndian.Uint16(data[16:]),
		StartTS:      int64(binary.LittleEndian.Uint64(data[18:])),
		PlayerCount:  data[26],
		MapW:         int32(binary.LittleEndian.Uint32(data[27:])),
		MapH:         int32(binary.LittleEndian.Uint32(data[31:])),
	}
	snapLen := int(binary.LittleEndian.Uint32(data[35:]))
	total := 39 + snapLen
	if len(data) < total {
		return nil, 0, ErrTruncated
	}
	h.Snapshot = make([]byte, snapLen)
	copy(h.Snapshot, data[39:total])
	return h, total, nil
}

// MarshalTick serializes a tick record.
func MarshalTick(tr *TickRecord) []byte {
	// tick(4) + cmd_count(2) + cmds(N*cmdRecordSize) + hash(8)
	size := 4 + 2 + len(tr.Cmds)*cmdRecordSize + 8
	buf := make([]byte, size)
	off := 0
	binary.LittleEndian.PutUint32(buf[off:], tr.Tick)
	off += 4
	binary.LittleEndian.PutUint16(buf[off:], uint16(len(tr.Cmds)))
	off += 2
	for _, c := range tr.Cmds {
		buf[off] = c.Player
		off++
		buf[off] = c.Op
		off++
		binary.LittleEndian.PutUint32(buf[off:], c.UnitID)
		off += 4
		binary.LittleEndian.PutUint32(buf[off:], uint32(c.TargetX))
		off += 4
		binary.LittleEndian.PutUint32(buf[off:], uint32(c.TargetY))
		off += 4
		binary.LittleEndian.PutUint32(buf[off:], c.TargetID)
		off += 4
		binary.LittleEndian.PutUint32(buf[off:], c.Tick)
		off += 4
	}
	binary.LittleEndian.PutUint64(buf[off:], tr.Hash)
	return buf
}

// UnmarshalTick deserializes a tick record, returning the record and bytes consumed.
func UnmarshalTick(data []byte) (*TickRecord, int, error) {
	if len(data) < 6 {
		return nil, 0, ErrTruncated
	}
	tick := binary.LittleEndian.Uint32(data[0:])
	cmdCount := int(binary.LittleEndian.Uint16(data[4:]))
	needed := 4 + 2 + cmdCount*cmdRecordSize + 8
	if len(data) < needed {
		return nil, 0, ErrTruncated
	}
	off := 6
	cmds := make([]wire.Cmd, cmdCount)
	for i := range cmds {
		cmds[i].Player = data[off]
		off++
		cmds[i].Op = data[off]
		off++
		cmds[i].UnitID = binary.LittleEndian.Uint32(data[off:])
		off += 4
		cmds[i].TargetX = int32(binary.LittleEndian.Uint32(data[off:]))
		off += 4
		cmds[i].TargetY = int32(binary.LittleEndian.Uint32(data[off:]))
		off += 4
		cmds[i].TargetID = binary.LittleEndian.Uint32(data[off:])
		off += 4
		cmds[i].Tick = binary.LittleEndian.Uint32(data[off:])
		off += 4
	}
	hash := binary.LittleEndian.Uint64(data[off:])
	tr := &TickRecord{Tick: tick, Cmds: cmds, Hash: hash}
	return tr, needed, nil
}
