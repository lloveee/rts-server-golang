package replay

import (
	"bytes"
	"rts/internal/wire"
	"testing"
)

func TestHeaderRoundTrip(t *testing.T) {
	h := &Header{
		ProtoVersion: 1,
		Seed:         42,
		TickRate:     20,
		StartTS:      1700000000,
		PlayerCount:  2,
		MapW:         100,
		MapH:         200,
		Snapshot:     []byte{1, 2, 3, 4, 5, 6, 7, 8},
	}

	data := MarshalHeader(h)
	h2, n, err := UnmarshalHeader(data)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(data) {
		t.Fatalf("consumed %d, expected %d", n, len(data))
	}
	if h2.ProtoVersion != h.ProtoVersion {
		t.Errorf("proto: got %d want %d", h2.ProtoVersion, h.ProtoVersion)
	}
	if h2.Seed != h.Seed {
		t.Errorf("seed: got %d want %d", h2.Seed, h.Seed)
	}
	if h2.TickRate != h.TickRate {
		t.Errorf("tick_rate: got %d want %d", h2.TickRate, h.TickRate)
	}
	if h2.PlayerCount != h.PlayerCount {
		t.Errorf("players: got %d want %d", h2.PlayerCount, h.PlayerCount)
	}
	if h2.MapW != h.MapW || h2.MapH != h.MapH {
		t.Errorf("map: got %dx%d want %dx%d", h2.MapW, h2.MapH, h.MapW, h.MapH)
	}
	if !bytes.Equal(h2.Snapshot, h.Snapshot) {
		t.Errorf("snapshot mismatch")
	}
}

func TestTickRoundTrip(t *testing.T) {
	tr := &TickRecord{
		Tick: 42,
		Cmds: []wire.Cmd{
			{Tick: 42, Player: 0, Op: 1, UnitID: 5, TargetX: 100, TargetY: 200, TargetID: 0},
			{Tick: 42, Player: 1, Op: 2, UnitID: 8, TargetX: -50, TargetY: -100, TargetID: 3},
		},
		Hash: 0xdeadbeefcafe1234,
	}

	data := MarshalTick(tr)
	tr2, n, err := UnmarshalTick(data)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(data) {
		t.Fatalf("consumed %d, expected %d", n, len(data))
	}
	if tr2.Tick != tr.Tick {
		t.Errorf("tick: got %d want %d", tr2.Tick, tr.Tick)
	}
	if tr2.Hash != tr.Hash {
		t.Errorf("hash: got %x want %x", tr2.Hash, tr.Hash)
	}
	if len(tr2.Cmds) != len(tr.Cmds) {
		t.Fatalf("cmds: got %d want %d", len(tr2.Cmds), len(tr.Cmds))
	}
	for i, c := range tr.Cmds {
		c2 := tr2.Cmds[i]
		if c2.Player != c.Player || c2.Op != c.Op || c2.UnitID != c.UnitID ||
			c2.TargetX != c.TargetX || c2.TargetY != c.TargetY || c2.TargetID != c.TargetID {
			t.Errorf("cmd[%d] mismatch: got %+v want %+v", i, c2, c)
		}
	}
}

func TestWriterReader(t *testing.T) {
	var buf bytes.Buffer

	w := NewWriter(&buf)
	h := &Header{
		ProtoVersion: 1,
		Seed:         99,
		TickRate:     20,
		StartTS:      1700000000,
		PlayerCount:  2,
		MapW:         64,
		MapH:         64,
		Snapshot:     []byte{0xAA, 0xBB},
	}
	if err := w.WriteHeader(h); err != nil {
		t.Fatal(err)
	}

	for tick := uint32(1); tick <= 10; tick++ {
		var cmds []wire.Cmd
		if tick == 5 {
			cmds = []wire.Cmd{{Tick: 5, Player: 0, Op: 1, UnitID: 1, TargetX: 10, TargetY: 20}}
		}
		if err := w.WriteTick(tick, cmds, uint64(tick)*0x1111); err != nil {
			t.Fatal(err)
		}
	}

	// Read back.
	r, err := NewReader(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if r.Header.Seed != 99 {
		t.Errorf("seed: got %d want 99", r.Header.Seed)
	}

	ticks, err := r.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(ticks) != 10 {
		t.Fatalf("ticks: got %d want 10", len(ticks))
	}
	if ticks[4].Tick != 5 {
		t.Errorf("tick[4]: got %d want 5", ticks[4].Tick)
	}
	if len(ticks[4].Cmds) != 1 {
		t.Fatalf("tick[4] cmds: got %d want 1", len(ticks[4].Cmds))
	}
	if ticks[4].Hash != 5*0x1111 {
		t.Errorf("tick[4] hash: got %x want %x", ticks[4].Hash, 5*0x1111)
	}
}

func TestBadMagic(t *testing.T) {
	data := []byte("BADMAGICxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx")
	_, _, err := UnmarshalHeader(data)
	if err != ErrBadMagic {
		t.Errorf("expected ErrBadMagic, got %v", err)
	}
}

func TestTruncated(t *testing.T) {
	_, _, err := UnmarshalHeader([]byte{1, 2, 3})
	if err != ErrTruncated {
		t.Errorf("expected ErrTruncated, got %v", err)
	}
}
