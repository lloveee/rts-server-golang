package sim

import (
	"rts/internal/sim/fixed"
	"testing"
)

func TestResolveCombat_DamagesEnemy(t *testing.T) {
	w := NewWorld(42, 100, 100)
	w.Players = []Player{{ID: 0}, {ID: 1}}

	s := UnitStatTable[UnitSoldier]
	id1 := w.SpawnUnit(0, fixed.VInt(10, 10), s.MaxHP, s.Speed)
	w.Units[len(w.Units)-1].Type = UnitSoldier
	w.Units[len(w.Units)-1].Range = s.Range
	w.Units[len(w.Units)-1].Damage = s.Damage
	w.Units[len(w.Units)-1].State = UnitAttacking

	id2 := w.SpawnUnit(1, fixed.VInt(11, 10), s.MaxHP, s.Speed)
	w.Units[len(w.Units)-1].Type = UnitSoldier
	w.Units[len(w.Units)-1].Range = s.Range
	w.Units[len(w.Units)-1].Damage = s.Damage
	w.Units[len(w.Units)-1].State = UnitAttacking

	w.Units[0].TargetID = id2
	w.Units[1].TargetID = id1

	Step(w, nil)

	u0 := w.FindUnit(id1)
	u1 := w.FindUnit(id2)
	if u0 == nil || u1 == nil {
		t.Fatal("units died too soon")
	}
	expected := s.MaxHP.Sub(s.Damage)
	if u0.HP != expected {
		t.Fatalf("u0 HP=%v want %v", u0.HP, expected)
	}
	if u1.HP != expected {
		t.Fatalf("u1 HP=%v want %v", u1.HP, expected)
	}
}

func TestResolveCombat_UnitDies(t *testing.T) {
	w := NewWorld(42, 100, 100)
	w.Players = []Player{{ID: 0}, {ID: 1}}

	s := UnitStatTable[UnitSoldier]
	id0 := w.SpawnUnit(0, fixed.VInt(10, 10), fixed.One, s.Speed)
	w.Units[len(w.Units)-1].Type = UnitSoldier
	w.Units[len(w.Units)-1].Range = s.Range
	w.Units[len(w.Units)-1].Damage = s.Damage
	w.Units[len(w.Units)-1].State = UnitAttacking

	id2 := w.SpawnUnit(1, fixed.VInt(11, 10), s.MaxHP, s.Speed)
	w.Units[len(w.Units)-1].Type = UnitSoldier
	w.Units[len(w.Units)-1].Range = s.Range
	w.Units[len(w.Units)-1].Damage = s.Damage
	w.Units[len(w.Units)-1].State = UnitAttacking

	w.Units[0].TargetID = id2
	w.Units[1].TargetID = id0

	Step(w, nil)

	if w.FindUnit(id0) != nil {
		t.Fatal("unit 0 should be dead (HP=1 vs 1.0 dmg)")
	}
}

func TestCmdAttackMove_MovesTowardTarget(t *testing.T) {
	w := NewWorld(42, 100, 100)
	w.Players = []Player{{ID: 0}, {ID: 1}}

	s := UnitStatTable[UnitSoldier]
	id := w.SpawnUnit(0, fixed.VInt(10, 10), s.MaxHP, s.Speed)
	w.Units[len(w.Units)-1].Type = UnitSoldier
	w.Units[len(w.Units)-1].Range = s.Range
	w.Units[len(w.Units)-1].Damage = s.Damage

	cmd := Cmd{Player: 0, Op: CmdAttackMove, UnitID: id, TargetPos: fixed.VInt(50, 50)}
	Step(w, []Cmd{cmd})

	u := w.FindUnit(id)
	if u == nil {
		t.Fatal("unit disappeared")
	}
	if u.State != UnitAttacking {
		t.Fatalf("state=%d want Attacking(%d)", u.State, UnitAttacking)
	}
	if u.Pos.X <= fixed.FromInt(10) {
		t.Fatal("unit did not move")
	}
}

func TestCmdSurrender_TriggersGameOver(t *testing.T) {
	w := NewWorld(42, 100, 100)
	w.Players = []Player{{ID: 0}, {ID: 1}}
	w.SpawnBuilding(0, BldHQ, fixed.VInt(10, 10))
	w.SpawnBuilding(1, BldHQ, fixed.VInt(90, 90))

	Step(w, []Cmd{{Player: 0, Op: CmdSurrender}})

	if !w.Players[0].Surrendered {
		t.Fatal("P0 should have surrendered")
	}
	if !w.GameOver {
		t.Fatal("game should be over")
	}
}

func TestPushAway_Overlapping(t *testing.T) {
	w := NewWorld(42, 100, 100)
	w.Players = []Player{{ID: 0}, {ID: 1}}
	s := UnitStatTable[UnitSoldier]
	w.SpawnUnit(0, fixed.VInt(10, 10), s.MaxHP, s.Speed)
	w.SpawnUnit(0, fixed.VInt(10, 10), s.MaxHP, s.Speed)
	w.Units[0].Type = UnitSoldier
	w.Units[1].Type = UnitSoldier

	applyPushAway(w)
	if w.Units[0].Pos == w.Units[1].Pos {
		t.Fatal("units should be pushed apart")
	}
}

func TestCheckVictory_AllBuildingsDestroyed(t *testing.T) {
	w := NewWorld(42, 100, 100)
	w.Players = []Player{{ID: 0}, {ID: 1}}
	bldID := w.SpawnBuilding(0, BldHQ, fixed.VInt(10, 10))
	w.FindBuilding(bldID).State = BldDead

	results := checkVictory(w)
	if results == nil {
		t.Fatal("game should be over")
	}
}
