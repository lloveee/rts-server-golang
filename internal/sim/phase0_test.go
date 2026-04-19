package sim

import (
	"rts/internal/sim/fixed"
	"testing"
)

func TestUnitStateValues(t *testing.T) {
	cases := []struct {
		got  UnitState
		want uint8
		name string
	}{
		{UnitIdle, 0, "Idle"},
		{UnitMoving, 1, "Moving"},
		{UnitAttacking, 2, "Attacking"},
		{UnitMining, 3, "Mining"},
		{UnitReturning, 4, "Returning"},
		{UnitBuilding, 5, "Building"},
		{UnitDead, 6, "Dead"},
	}
	for _, c := range cases {
		if uint8(c.got) != c.want {
			t.Errorf("UnitState %s = %d, want %d", c.name, c.got, c.want)
		}
	}
}

func TestUnitTypeValues(t *testing.T) {
	cases := []struct {
		got  UnitType
		want uint8
		name string
	}{
		{UnitWorker, 1, "Worker"},
		{UnitSoldier, 2, "Soldier"},
		{UnitArcher, 3, "Archer"},
		{UnitCavalry, 4, "Cavalry"},
	}
	for _, c := range cases {
		if uint8(c.got) != c.want {
			t.Errorf("UnitType %s = %d, want %d", c.name, c.got, c.want)
		}
	}
}

func TestUnit_ZeroValueFields(t *testing.T) {
	var u Unit
	if u.Type != 0 {
		t.Errorf("Unit.Type zero = %d, want 0", u.Type)
	}
	if u.Range != 0 || u.Damage != 0 || u.VisionRange != 0 || u.CarryAmount != 0 {
		t.Errorf("Unit combat fields not zero: range=%v dmg=%v vr=%v carry=%v",
			u.Range, u.Damage, u.VisionRange, u.CarryAmount)
	}
	if u.Path != nil {
		t.Errorf("Unit.Path zero = %v, want nil", u.Path)
	}
	var zeroVec fixed.Vec2
	if u.AttackMoveTarget != zeroVec {
		t.Errorf("Unit.AttackMoveTarget zero = %v, want zero vec", u.AttackMoveTarget)
	}
}
