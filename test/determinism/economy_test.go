package determinism

import (
	"rts/internal/sim"
	"rts/internal/sim/fixed"
	"testing"
)

func economyScenario() (seed uint64, spawnFn func(w *sim.World), tickCmds map[uint32][]sim.Cmd) {
	seed = 0xEC0EC0EC0
	tickCmds = map[uint32][]sim.Cmd{}

	spawnFn = func(w *sim.World) {
		w.Players = []sim.Player{
			{ID: 0, Crystal: fixed.FromInt(200)},
			{ID: 1, Crystal: fixed.FromInt(200)},
		}
		w.SpawnBuilding(0, sim.BldHQ, fixed.VInt(10, 45))
		w.SpawnBuilding(1, sim.BldHQ, fixed.VInt(90, 45))
		for _, pos := range sim.CrystalPositions(0) {
			w.SpawnCrystal(pos)
		}
		for _, pos := range sim.CrystalPositions(1) {
			w.SpawnCrystal(pos)
		}
		stats := sim.UnitStatTable[sim.UnitWorker]
		for i := int32(0); i < 3; i++ {
			id := w.SpawnUnit(0, fixed.VInt(13+i, 45+i), stats.MaxHP, stats.Speed)
			w.Units[len(w.Units)-1].Type = sim.UnitWorker
			_ = id
			id = w.SpawnUnit(1, fixed.VInt(93-i, 45+i), stats.MaxHP, stats.Speed)
			w.Units[len(w.Units)-1].Type = sim.UnitWorker
			_ = id
		}
	}

	// Tick 5: send all workers to nearest crystals.
	tickCmds[5] = []sim.Cmd{
		{Player: 0, Op: sim.CmdMove, UnitID: 19, TargetPos: fixed.VInt(5, 10)},
		{Player: 0, Op: sim.CmdMove, UnitID: 21, TargetPos: fixed.VInt(15, 55)},
		{Player: 0, Op: sim.CmdMove, UnitID: 23, TargetPos: fixed.VInt(25, 70)},
		{Player: 1, Op: sim.CmdMove, UnitID: 20, TargetPos: fixed.VInt(75, 10)},
		{Player: 1, Op: sim.CmdMove, UnitID: 22, TargetPos: fixed.VInt(85, 55)},
		{Player: 1, Op: sim.CmdMove, UnitID: 24, TargetPos: fixed.VInt(95, 70)},
	}

	// Tick 60: P0 trains a worker from HQ (ID 1).
	tickCmds[60] = []sim.Cmd{
		{Player: 0, Op: sim.CmdTrain, UnitID: 1, TargetID: uint32(sim.UnitWorker)},
	}

	// Tick 120: P1 trains a worker from HQ (ID 2).
	tickCmds[120] = []sim.Cmd{
		{Player: 1, Op: sim.CmdTrain, UnitID: 2, TargetID: uint32(sim.UnitWorker)},
	}

	return
}

const econTotalTicks = 300

func runEconomyOnce() []uint64 {
	seed, spawnFn, tickCmds := economyScenario()
	w := sim.NewWorld(seed, 100, 100)
	spawnFn(w)
	hashes := make([]uint64, econTotalTicks)
	for t := uint32(1); t <= econTotalTicks; t++ {
		cmds := tickCmds[t]
		sim.Step(w, cmds)
		hashes[t-1] = sim.Hash(w)
	}
	return hashes
}

func TestEconomyDeterminism100x(t *testing.T) {
	reference := runEconomyOnce()
	for run := 1; run < 100; run++ {
		hashes := runEconomyOnce()
		for tick := 0; tick < econTotalTicks; tick++ {
			if hashes[tick] != reference[tick] {
				t.Fatalf("DESYNC run=%d tick=%d: got %016x, want %016x",
					run, tick+1, hashes[tick], reference[tick])
			}
		}
	}
	t.Logf("economy: 100 runs x %d ticks: all hashes match", econTotalTicks)
	t.Logf("final hash: %016x", reference[econTotalTicks-1])
}
