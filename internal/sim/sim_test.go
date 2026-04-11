package sim

import (
	"rts/internal/sim/fixed"
	"testing"
)

func setupTestWorld() *World {
	w := NewWorld(42, 100, 100)
	// Player 0: 3 units at left side
	w.SpawnUnit(0, fixed.VInt(10, 50), fixed.FromInt(10), fixed.FromFloat64(0.5))
	w.SpawnUnit(0, fixed.VInt(10, 52), fixed.FromInt(10), fixed.FromFloat64(0.5))
	w.SpawnUnit(0, fixed.VInt(10, 48), fixed.FromInt(10), fixed.FromFloat64(0.5))
	// Player 1: 3 units at right side
	w.SpawnUnit(1, fixed.VInt(90, 50), fixed.FromInt(10), fixed.FromFloat64(0.5))
	w.SpawnUnit(1, fixed.VInt(90, 52), fixed.FromInt(10), fixed.FromFloat64(0.5))
	w.SpawnUnit(1, fixed.VInt(90, 48), fixed.FromInt(10), fixed.FromFloat64(0.5))
	return w
}

func TestStepMove(t *testing.T) {
	w := setupTestWorld()
	// Command unit 1 to move right.
	cmds := []Cmd{
		{Player: 0, Op: CmdMove, UnitID: 1, TargetPos: fixed.VInt(50, 50)},
	}
	Step(w, cmds)

	u := w.FindUnit(1)
	if u == nil {
		t.Fatal("unit 1 not found")
	}
	if u.State != UnitMoving {
		t.Errorf("unit 1 state = %d, want Moving", u.State)
	}
	// Should have moved 0.5 units to the right.
	if u.Pos.X.ToFloat64() < 10.4 || u.Pos.X.ToFloat64() > 10.6 {
		t.Errorf("unit 1 x = %f, want ~10.5", u.Pos.X.ToFloat64())
	}
}

func TestStepAttack(t *testing.T) {
	w := NewWorld(42, 100, 100)
	// Two adjacent units.
	w.SpawnUnit(0, fixed.VInt(10, 10), fixed.FromInt(10), fixed.FromFloat64(0.5))
	w.SpawnUnit(1, fixed.VInt(11, 10), fixed.FromInt(10), fixed.FromFloat64(0.5))

	// Player 0 attacks player 1's unit.
	cmds := []Cmd{
		{Player: 0, Op: CmdAttack, UnitID: 1, TargetID: 2},
	}
	Step(w, cmds)

	target := w.FindUnit(2)
	if target == nil {
		t.Fatal("unit 2 not found")
	}
	// HP should have decreased by AttackDamage (0.5).
	expected := 10.0 - 0.5
	got := target.HP.ToFloat64()
	if got < expected-0.01 || got > expected+0.01 {
		t.Errorf("target HP = %f, want %f", got, expected)
	}
}

func TestHashDeterminism(t *testing.T) {
	// Same setup + same commands = same hash, 100 times.
	hashes := make([]uint64, 100)
	for i := range hashes {
		w := setupTestWorld()
		cmds := []Cmd{
			{Player: 0, Op: CmdMove, UnitID: 1, TargetPos: fixed.VInt(50, 50)},
			{Player: 1, Op: CmdAttack, UnitID: 4, TargetID: 1},
		}
		for tick := 0; tick < 200; tick++ {
			if tick == 0 {
				Step(w, cmds)
			} else {
				Step(w, nil)
			}
		}
		hashes[i] = Hash(w)
	}
	for i := 1; i < len(hashes); i++ {
		if hashes[i] != hashes[0] {
			t.Fatalf("hash mismatch: run %d = %016x, run 0 = %016x", i, hashes[i], hashes[0])
		}
	}
	t.Logf("deterministic hash after 200 ticks: %016x", hashes[0])
}

func TestSnapshotRoundTrip(t *testing.T) {
	w := setupTestWorld()
	Step(w, []Cmd{
		{Player: 0, Op: CmdMove, UnitID: 1, TargetPos: fixed.VInt(50, 50)},
	})

	data := Marshal(w)
	w2, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	h1 := Hash(w)
	h2 := Hash(w2)
	if h1 != h2 {
		t.Fatalf("hash mismatch after roundtrip: %016x vs %016x", h1, h2)
	}

	// Run more ticks on both and compare.
	for i := 0; i < 100; i++ {
		Step(w, nil)
		Step(w2, nil)
	}
	h1 = Hash(w)
	h2 = Hash(w2)
	if h1 != h2 {
		t.Fatalf("hash diverged after snapshot roundtrip + 100 ticks: %016x vs %016x", h1, h2)
	}
}
