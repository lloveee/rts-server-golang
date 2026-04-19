package sim

import "testing"

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
