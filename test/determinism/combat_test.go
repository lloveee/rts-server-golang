package determinism

import (
	"rts/internal/sim"
	"rts/internal/sim/fixed"
	"testing"
)

const combatTicks = 600

func combatScenario() (uint64, func(*sim.World), map[uint32][]sim.Cmd) {
	seed := uint64(0xBEEF0001)
	cmds := map[uint32][]sim.Cmd{}

	spawn := func(w *sim.World) {
		w.Players = []sim.Player{{ID: 0, Crystal: fixed.FromInt(500)}, {ID: 1, Crystal: fixed.FromInt(500)}}
		w.SpawnBuilding(0, sim.BldHQ, fixed.VInt(10, 45))
		w.SpawnBuilding(1, sim.BldHQ, fixed.VInt(90, 45))
		ws := sim.UnitStatTable[sim.UnitWorker]
		ss := sim.UnitStatTable[sim.UnitSoldier]
		for i := int32(0); i < 3; i++ {
			w.SpawnUnit(0, fixed.VInt(13+i, 45+i), ws.MaxHP, ws.Speed)
			w.Units[len(w.Units)-1].Type = sim.UnitWorker
			w.SpawnUnit(1, fixed.VInt(93-i, 45+i), ws.MaxHP, ws.Speed)
			w.Units[len(w.Units)-1].Type = sim.UnitWorker
		}
		for i := int32(0); i < 5; i++ {
			w.SpawnUnit(0, fixed.VInt(40+i, 48), ss.MaxHP, ss.Speed)
			u := &w.Units[len(w.Units)-1]; u.Type = sim.UnitSoldier; u.Range = ss.Range; u.Damage = ss.Damage
			w.SpawnUnit(1, fixed.VInt(60-i, 48), ss.MaxHP, ss.Speed)
			u = &w.Units[len(w.Units)-1]; u.Type = sim.UnitSoldier; u.Range = ss.Range; u.Damage = ss.Damage
		}
	}

	// Attack-move soldiers toward enemy HQ.
	for _, id := range []uint32{25, 27, 29, 31, 33} {
		cmds[3] = append(cmds[3], sim.Cmd{Player: 0, Op: sim.CmdAttackMove, UnitID: id, TargetPos: fixed.VInt(90, 45)})
	}
	for _, id := range []uint32{26, 28, 30, 32, 34} {
		cmds[3] = append(cmds[3], sim.Cmd{Player: 1, Op: sim.CmdAttackMove, UnitID: id, TargetPos: fixed.VInt(10, 45)})
	}

	return seed, spawn, cmds
}

func TestCombatDeterminism100x(t *testing.T) {
	seed, spawn, cmds := combatScenario()
	w := sim.NewWorld(seed, 100, 100)
	spawn(w)
	ref := make([]uint64, combatTicks)
	for t := uint32(1); t <= combatTicks; t++ { sim.Step(w, cmds[t]); ref[t-1] = sim.Hash(w) }

	for run := 1; run < 100; run++ {
		w2 := sim.NewWorld(seed, 100, 100)
		spawn(w2)
		for tk := uint32(1); tk <= combatTicks; tk++ { sim.Step(w2, cmds[tk]); h := sim.Hash(w2); if h != ref[tk-1] { t.Fatalf("DESYNC run=%d tick=%d", run, tk) } }
	}
	t.Logf("combat: 100x%d OK, final=%016x", combatTicks, ref[combatTicks-1])
}
