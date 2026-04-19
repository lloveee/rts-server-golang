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

func TestBuilding_ZeroValueFields(t *testing.T) {
	var b Building
	if b.State != BldConstructing {
		t.Errorf("Building.State zero = %d, want BldConstructing (0)", b.State)
	}
	if b.Type != 0 {
		t.Errorf("Building.Type zero = %d, want 0", b.Type)
	}
	if b.ProductionQueue != nil {
		t.Errorf("Building.ProductionQueue zero = %v, want nil", b.ProductionQueue)
	}
}

func TestBuildingType_Values(t *testing.T) {
	cases := []struct {
		got  BuildingType
		want uint8
	}{
		{BldHQ, 1}, {BldBarracks, 2}, {BldArchery, 3}, {BldStable, 4},
	}
	for _, c := range cases {
		if uint8(c.got) != c.want {
			t.Errorf("BuildingType = %d, want %d", c.got, c.want)
		}
	}
}

func TestBuildingState_Values(t *testing.T) {
	cases := []struct {
		got  BuildingState
		want uint8
	}{
		{BldConstructing, 0}, {BldReady, 1}, {BldDead, 2},
	}
	for _, c := range cases {
		if uint8(c.got) != c.want {
			t.Errorf("BuildingState = %d, want %d", c.got, c.want)
		}
	}
}

func TestCrystal_ZeroValueFields(t *testing.T) {
	var c Crystal
	if c.ID != 0 || c.Remaining != 0 {
		t.Errorf("Crystal zero-values non-zero: id=%d rem=%v", c.ID, c.Remaining)
	}
}

func TestPlayer_ZeroValueFields(t *testing.T) {
	var p Player
	if p.ID != 0 || p.Crystal != 0 || p.Surrendered {
		t.Errorf("Player zero-values non-default: id=%d crystal=%v surr=%v",
			p.ID, p.Crystal, p.Surrendered)
	}
}

func TestNavGrid_Factory_100x100(t *testing.T) {
	g := NewNavGrid(100, 100)
	if g.W != 100 || g.H != 100 {
		t.Errorf("NavGrid dims = (%d,%d), want (100,100)", g.W, g.H)
	}
	// 100*100 = 10000 bits → (10000 + 63) / 64 = 157 uint64 words
	wantWords := 157
	if got := g.BlockedWords(); got != wantWords {
		t.Errorf("BlockedWords = %d, want %d", got, wantWords)
	}
	for i, w := range g.Blocked {
		if w != 0 {
			t.Errorf("Blocked[%d] = %#x, want 0 (fresh NavGrid should be all-free)", i, w)
		}
	}
}

func TestNewWorld_InitialisesNewFields(t *testing.T) {
	w := NewWorld(42, 100, 100)
	if w.Buildings == nil {
		t.Error("Buildings should be non-nil empty slice")
	}
	if len(w.Buildings) != 0 {
		t.Errorf("len(Buildings) = %d, want 0", len(w.Buildings))
	}
	if w.Crystals == nil || len(w.Crystals) != 0 {
		t.Errorf("Crystals should be non-nil empty slice; got len=%d nil=%v",
			len(w.Crystals), w.Crystals == nil)
	}
	if w.Players == nil || len(w.Players) != 0 {
		t.Errorf("Players should be non-nil empty slice; got len=%d nil=%v",
			len(w.Players), w.Players == nil)
	}
	if w.NavGrid == nil {
		t.Fatal("NavGrid must be initialised by NewWorld")
	}
	if w.NavGrid.W != 100 || w.NavGrid.H != 100 {
		t.Errorf("NavGrid dims = (%d,%d), want (100,100)", w.NavGrid.W, w.NavGrid.H)
	}
}

func TestHash_SensitiveToUnitType(t *testing.T) {
	wA := NewWorld(42, 100, 100)
	wA.SpawnUnit(0, fixed.VInt(10, 10), fixed.One, fixed.Half)
	wB := NewWorld(42, 100, 100)
	wB.SpawnUnit(0, fixed.VInt(10, 10), fixed.One, fixed.Half)
	wB.Units[0].Type = UnitSoldier

	if Hash(wA) == Hash(wB) {
		t.Fatal("Hash must differ when Unit.Type differs")
	}
}

func TestHash_SensitiveToBuildingCount(t *testing.T) {
	wA := NewWorld(42, 100, 100)
	wB := NewWorld(42, 100, 100)
	wB.Buildings = append(wB.Buildings, Building{ID: 1, Owner: 0, Type: BldHQ})

	if Hash(wA) == Hash(wB) {
		t.Fatal("Hash must differ when Buildings count differs")
	}
}

func TestHash_SensitiveToNavGridDims(t *testing.T) {
	wA := NewWorld(42, 100, 100)
	wB := NewWorld(42, 100, 100)
	wB.NavGrid = NewNavGrid(50, 50)

	if Hash(wA) == Hash(wB) {
		t.Fatal("Hash must differ when NavGrid dims differ")
	}
}

func TestHash_EmptyWorld_Deterministic(t *testing.T) {
	// Same seed + same (empty) world → identical hash across 10 rebuilds.
	hashes := make([]uint64, 10)
	for i := range hashes {
		hashes[i] = Hash(NewWorld(42, 100, 100))
	}
	for i := 1; i < len(hashes); i++ {
		if hashes[i] != hashes[0] {
			t.Fatalf("empty-world hash drift: run %d = %#x, run 0 = %#x",
				i, hashes[i], hashes[0])
		}
	}
	t.Logf("empty-world hash (seed=42, 100x100): %016x", hashes[0])
}

func TestSnapshot_RoundTrip_ExtendedFields(t *testing.T) {
	w := NewWorld(42, 100, 100)

	w.Units = append(w.Units, Unit{
		ID: 1, Owner: 0, Type: UnitSoldier,
		Pos:              fixed.VInt(10, 10),
		HP:               fixed.FromInt(5),
		MaxHP:            fixed.FromInt(5),
		Speed:            fixed.Half,
		Range:            fixed.FromInt(2),
		Damage:           fixed.Half,
		VisionRange:      fixed.FromInt(6),
		CarryAmount:      0,
		State:            UnitMoving,
		TargetID:         0,
		MoveTo:           fixed.VInt(20, 20),
		AttackMoveTarget: fixed.VInt(30, 30),
		Path: []fixed.Vec2{
			fixed.VInt(15, 15),
			fixed.VInt(20, 20),
		},
	})
	w.Buildings = append(w.Buildings, Building{
		ID: 2, Owner: 0, Type: BldHQ, SizeCells: 4,
		Pos: fixed.VInt(5, 5), HP: fixed.FromInt(50), MaxHP: fixed.FromInt(50),
		State: BldReady, ConstructProgress: fixed.One, RallyPoint: fixed.VInt(10, 5),
		ProductionQueue: []QueueItem{
			{UnitType: UnitWorker, TicksLeft: 100, StartTick: 10},
			{UnitType: UnitSoldier, TicksLeft: 200, StartTick: 10},
		},
	})
	w.Crystals = append(w.Crystals, Crystal{ID: 3, Pos: fixed.VInt(50, 50), Remaining: fixed.FromInt(500)})
	w.Players = append(w.Players,
		Player{ID: 0, Crystal: fixed.FromInt(100), Surrendered: false},
		Player{ID: 1, Crystal: fixed.FromInt(80), Surrendered: true},
	)
	w.NavGrid.Blocked[0] = 0x00000000_0000_FF01
	w.NavGrid.Blocked[156] = 0x8000_0000_0000_0001

	h1 := Hash(w)
	data := Marshal(w)
	w2, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	h2 := Hash(w2)
	if h1 != h2 {
		t.Fatalf("hash mismatch after round-trip: %#x vs %#x", h1, h2)
	}
}

func TestSnapshot_RoundTrip_EmptyWorld(t *testing.T) {
	w := NewWorld(42, 100, 100)
	h1 := Hash(w)
	data := Marshal(w)
	w2, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	h2 := Hash(w2)
	if h1 != h2 {
		t.Fatalf("empty-world hash diverged: %#x vs %#x", h1, h2)
	}
}
