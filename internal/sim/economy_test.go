package sim

import (
	"rts/internal/sim/fixed"
	"testing"
)

func TestStepMining_DepletesCrystal(t *testing.T) {
	w := NewWorld(42, 100, 100)
	w.Players = []Player{{ID: 0}, {ID: 1}}
	w.SpawnBuilding(0, BldHQ, fixed.VInt(10, 10))
	crystalID := w.SpawnCrystal(fixed.VInt(20, 20))
	crystal := w.FindCrystal(crystalID)
	if crystal == nil {
		t.Fatal("crystal not found")
	}

	workerStats := UnitStatTable[UnitWorker]
	workerID := w.SpawnUnit(0, fixed.VInt(20, 21), workerStats.MaxHP, workerStats.Speed)
	w.Units[len(w.Units)-1].Type = UnitWorker
	w.Units[len(w.Units)-1].State = UnitMining
	w.Units[len(w.Units)-1].TargetID = crystalID

	initialRemaining := crystal.Remaining

	for tick := 0; tick < MiningTicksPerTrip; tick++ {
		Step(w, nil)
	}

	worker := w.FindUnit(workerID)
	if worker == nil {
		t.Fatal("worker died unexpectedly")
	}
	if worker.State != UnitReturning {
		t.Fatalf("worker state = %d, want Returning(%d)", worker.State, UnitReturning)
	}
	if worker.CarryAmount != fixed.FromInt(CarryCapacity) {
		t.Fatalf("worker CarryAmount = %v, want %d", worker.CarryAmount, CarryCapacity)
	}

	crystal = w.FindCrystal(crystalID)
	if crystal == nil {
		t.Fatal("crystal was removed prematurely")
	}
	expectedRem := initialRemaining.Sub(fixed.FromInt(CarryCapacity))
	if crystal.Remaining != expectedRem {
		t.Fatalf("crystal Remaining = %v, want %v", crystal.Remaining, expectedRem)
	}
}

func TestStepReturning_DepositsAtHQ(t *testing.T) {
	w := NewWorld(42, 100, 100)
	w.Players = []Player{{ID: 0, Crystal: 0}, {ID: 1}}
	hqID := w.SpawnBuilding(0, BldHQ, fixed.VInt(10, 10))

	workerStats := UnitStatTable[UnitWorker]
	workerID := w.SpawnUnit(0, fixed.VInt(11, 10), workerStats.MaxHP, workerStats.Speed)
	w.Units[len(w.Units)-1].Type = UnitWorker
	w.Units[len(w.Units)-1].State = UnitReturning
	w.Units[len(w.Units)-1].TargetID = hqID
	w.Units[len(w.Units)-1].CarryAmount = fixed.FromInt(CarryCapacity)
	w.Units[len(w.Units)-1].MoveTo = fixed.VInt(10, 10)

	for tick := 0; tick < 20; tick++ {
		Step(w, nil)
		if w.Players[0].Crystal > 0 {
			break
		}
	}

	if w.Players[0].Crystal != fixed.FromInt(CarryCapacity) {
		t.Fatalf("player crystal = %v, want %d", w.Players[0].Crystal, CarryCapacity)
	}
	_ = workerID
}

func TestCmdTrain_Valid(t *testing.T) {
	w := NewWorld(42, 100, 100)
	w.Players = []Player{{ID: 0, Crystal: fixed.FromInt(200)}, {ID: 1}}
	hqID := w.SpawnBuilding(0, BldHQ, fixed.VInt(10, 10))

	cmd := Cmd{
		Player:   0,
		Op:       CmdTrain,
		UnitID:   hqID,
		TargetID: uint32(UnitWorker),
	}
	Step(w, []Cmd{cmd})

	hq := w.FindBuilding(hqID)
	if hq == nil {
		t.Fatal("HQ missing")
	}
	if len(hq.ProductionQueue) != 1 {
		t.Fatalf("queue length = %d, want 1", len(hq.ProductionQueue))
	}
	if hq.ProductionQueue[0].UnitType != UnitWorker {
		t.Fatalf("queued unit type = %d, want Worker(%d)", hq.ProductionQueue[0].UnitType, UnitWorker)
	}
	cost := UnitStatTable[UnitWorker].Cost
	expectedCrystal := fixed.FromInt(200).Sub(cost)
	if w.Players[0].Crystal != expectedCrystal {
		t.Fatalf("player crystal = %v, want %v", w.Players[0].Crystal, expectedCrystal)
	}
}

func TestCmdTrain_InsufficientResource(t *testing.T) {
	w := NewWorld(42, 100, 100)
	w.Players = []Player{{ID: 0, Crystal: 0}, {ID: 1}}
	hqID := w.SpawnBuilding(0, BldHQ, fixed.VInt(10, 10))

	cmd := Cmd{
		Player:   0,
		Op:       CmdTrain,
		UnitID:   hqID,
		TargetID: uint32(UnitWorker),
	}
	Step(w, []Cmd{cmd})

	hq := w.FindBuilding(hqID)
	if len(hq.ProductionQueue) != 0 {
		t.Fatalf("queue length = %d, want 0 (insufficient resource)", len(hq.ProductionQueue))
	}
}

func TestCmdTrain_WrongBuildingType(t *testing.T) {
	w := NewWorld(42, 100, 100)
	w.Players = []Player{{ID: 0, Crystal: fixed.FromInt(500)}, {ID: 1}}
	hqID := w.SpawnBuilding(0, BldHQ, fixed.VInt(10, 10))

	// HQ can only train Workers — try to train Soldier.
	cmd := Cmd{
		Player:   0,
		Op:       CmdTrain,
		UnitID:   hqID,
		TargetID: uint32(UnitSoldier),
	}
	Step(w, []Cmd{cmd})

	hq := w.FindBuilding(hqID)
	if len(hq.ProductionQueue) != 0 {
		t.Fatalf("queue length = %d, want 0 (wrong building type)", len(hq.ProductionQueue))
	}
}

func TestProductionQueue_SpawnsUnit(t *testing.T) {
	w := NewWorld(42, 100, 100)
	w.Players = []Player{{ID: 0, Crystal: fixed.FromInt(200)}, {ID: 1}}
	hqID := w.SpawnBuilding(0, BldHQ, fixed.VInt(10, 10))

	hq := w.FindBuilding(hqID)
	hq.ProductionQueue = append(hq.ProductionQueue, QueueItem{
		UnitType:  UnitWorker,
		TicksLeft: 10,
		StartTick: 1,
	})

	initialUnitCount := len(w.Units)
	for tick := 0; tick < 15; tick++ {
		Step(w, nil)
		if len(w.Units) > initialUnitCount {
			break
		}
	}

	if len(w.Units) != initialUnitCount+1 {
		t.Fatalf("unit count = %d, want %d", len(w.Units), initialUnitCount+1)
	}
	newUnit := &w.Units[len(w.Units)-1]
	if newUnit.Type != UnitWorker {
		t.Fatalf("spawned unit type = %d, want Worker(%d)", newUnit.Type, UnitWorker)
	}
	if newUnit.Owner != 0 {
		t.Fatalf("spawned unit owner = %d, want 0", newUnit.Owner)
	}
	if len(hq.ProductionQueue) != 0 {
		t.Fatalf("queue should be empty after spawn, got %d items", len(hq.ProductionQueue))
	}
}

func TestStep_CrystalRightClick_EntersMining(t *testing.T) {
	w := NewWorld(42, 100, 100)
	w.Players = []Player{{ID: 0}, {ID: 1}}
	w.SpawnBuilding(0, BldHQ, fixed.VInt(10, 10))
	crystalID := w.SpawnCrystal(fixed.VInt(20, 20))

	workerStats := UnitStatTable[UnitWorker]
	workerID := w.SpawnUnit(0, fixed.VInt(10, 11), workerStats.MaxHP, workerStats.Speed)
	w.Units[len(w.Units)-1].Type = UnitWorker

	cmd := Cmd{
		Player:    0,
		Op:        CmdMove,
		UnitID:    workerID,
		TargetPos: fixed.VInt(20, 20),
	}
	Step(w, []Cmd{cmd})

	worker := w.FindUnit(workerID)
	if worker == nil {
		t.Fatal("worker missing")
	}
	if worker.State != UnitMining {
		t.Fatalf("worker state = %d, want Mining(%d)", worker.State, UnitMining)
	}
	if worker.TargetID != crystalID {
		t.Fatalf("worker TargetID = %d, want crystal %d", worker.TargetID, crystalID)
	}
}
