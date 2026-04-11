// Package determinism runs the same simulation scenario 100 times and
// asserts that every run produces identical state hashes at every tick.
// This is the ultimate guard for the "no desync" guarantee.
//
// CI MUST run this test on every commit and on every Go version upgrade.
package determinism

import (
	"rts/internal/sim"
	"rts/internal/sim/fixed"
	"testing"
)

// scenario builds a repeatable command sequence for a 2-player game.
func scenario() (seed uint64, mapW, mapH int32, spawns []sim.Unit, tickCmds map[uint32][]sim.Cmd) {
	seed = 0xDEADBEEFCAFE
	mapW, mapH = 100, 100

	spawns = []sim.Unit{
		{Owner: 0, Pos: fixed.VInt(10, 50), HP: fixed.FromInt(10), MaxHP: fixed.FromInt(10), Speed: fixed.FromFloat64(0.5)},
		{Owner: 0, Pos: fixed.VInt(12, 50), HP: fixed.FromInt(10), MaxHP: fixed.FromInt(10), Speed: fixed.FromFloat64(0.5)},
		{Owner: 0, Pos: fixed.VInt(14, 50), HP: fixed.FromInt(10), MaxHP: fixed.FromInt(10), Speed: fixed.FromFloat64(0.5)},
		{Owner: 0, Pos: fixed.VInt(10, 55), HP: fixed.FromInt(10), MaxHP: fixed.FromInt(10), Speed: fixed.FromFloat64(0.5)},
		{Owner: 0, Pos: fixed.VInt(12, 55), HP: fixed.FromInt(10), MaxHP: fixed.FromInt(10), Speed: fixed.FromFloat64(0.5)},
		{Owner: 1, Pos: fixed.VInt(90, 50), HP: fixed.FromInt(10), MaxHP: fixed.FromInt(10), Speed: fixed.FromFloat64(0.5)},
		{Owner: 1, Pos: fixed.VInt(88, 50), HP: fixed.FromInt(10), MaxHP: fixed.FromInt(10), Speed: fixed.FromFloat64(0.5)},
		{Owner: 1, Pos: fixed.VInt(86, 50), HP: fixed.FromInt(10), MaxHP: fixed.FromInt(10), Speed: fixed.FromFloat64(0.5)},
		{Owner: 1, Pos: fixed.VInt(90, 55), HP: fixed.FromInt(10), MaxHP: fixed.FromInt(10), Speed: fixed.FromFloat64(0.5)},
		{Owner: 1, Pos: fixed.VInt(88, 55), HP: fixed.FromInt(10), MaxHP: fixed.FromInt(10), Speed: fixed.FromFloat64(0.5)},
	}

	tickCmds = map[uint32][]sim.Cmd{
		1: {
			{Player: 0, Op: sim.CmdMove, UnitID: 1, TargetPos: fixed.VInt(50, 50)},
			{Player: 0, Op: sim.CmdMove, UnitID: 2, TargetPos: fixed.VInt(50, 52)},
			{Player: 0, Op: sim.CmdMove, UnitID: 3, TargetPos: fixed.VInt(50, 48)},
			{Player: 1, Op: sim.CmdMove, UnitID: 6, TargetPos: fixed.VInt(50, 50)},
			{Player: 1, Op: sim.CmdMove, UnitID: 7, TargetPos: fixed.VInt(50, 52)},
		},
		80: {
			{Player: 0, Op: sim.CmdAttack, UnitID: 1, TargetID: 6},
			{Player: 1, Op: sim.CmdAttack, UnitID: 6, TargetID: 1},
		},
		150: {
			{Player: 0, Op: sim.CmdMove, UnitID: 4, TargetPos: fixed.VInt(80, 55)},
			{Player: 0, Op: sim.CmdMove, UnitID: 5, TargetPos: fixed.VInt(80, 55)},
			{Player: 1, Op: sim.CmdStop, UnitID: 9},
		},
	}
	return
}

const totalTicks = 300

func runOnce() []uint64 {
	seed, mapW, mapH, spawns, tickCmds := scenario()
	w := sim.NewWorld(seed, mapW, mapH)
	for _, s := range spawns {
		w.SpawnUnit(s.Owner, s.Pos, s.HP, s.Speed)
	}

	hashes := make([]uint64, totalTicks)
	for t := uint32(1); t <= totalTicks; t++ {
		cmds := tickCmds[t] // nil if no commands this tick
		sim.Step(w, cmds)
		hashes[t-1] = sim.Hash(w)
	}
	return hashes
}

func TestDeterminism100x(t *testing.T) {
	reference := runOnce()

	for run := 1; run < 100; run++ {
		hashes := runOnce()
		for tick := 0; tick < totalTicks; tick++ {
			if hashes[tick] != reference[tick] {
				t.Fatalf("DESYNC run=%d tick=%d: got %016x, want %016x",
					run, tick+1, hashes[tick], reference[tick])
			}
		}
	}
	t.Logf("100 runs × %d ticks: all hashes match", totalTicks)
	t.Logf("final hash: %016x", reference[totalTicks-1])
}
