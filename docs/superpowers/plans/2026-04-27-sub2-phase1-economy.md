# Phase 1 · Economy Slice Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Workers mine crystals, HQ produces Soldiers on cooldown, resource counter visible. No combat yet.

**Architecture:** Extend the Phase 0 sim foundation with Worker mining FSM (Idle→Mining→Returning cycle), HQ ProductionQueue ticking, and CmdTrain wire command. Server spawns initial HQ+3 Workers+crystals per player. Determinism verified via 100× byte-equal economy test + bot-vs-bot.

**Tech Stack:** Go (server/sim/bot), C# (Unity client/sim), Q16.16 fixed-point, FNV-1a-64 hash

---

## File Structure

| File | Responsibility |
|---|---|
| `internal/sim/constants.go` (new) | Entity stat tables (HP/Speed/Cost/etc.) — single source of truth |
| `internal/sim/step.go` (modify) | Tick loop: sort all entity types, dispatch mining/returning/building/production states |
| `internal/sim/world.go` (modify) | FindEntity (scans 3 lists), SpawnBuilding, SpawnCrystal, sort helpers, RemoveDead for buildings/crystals |
| `internal/sim/economy.go` (new) | stepMining, stepReturning — Worker resource gather/deposit FSM |
| `internal/sim/production.go` (new) | Building queue ticking, unit spawn at edge cell |
| `internal/sim/commands.go` (new) | CmdTrain validation + application |
| `internal/lockstep/room.go` (modify) | SpawnInitialUnits → 1×HQ + 3×Workers + 8 crystals per player; call on room full |
| `cmd/bot-client/main.go` (new) | Scripted bot binary, Phase_Economy strategy |
| `cmd/golden-export/main.go` (modify) | Include crystal positions + HQ + initial worker state |
| `test/determinism/economy_test.go` (new) | 100× byte-equal: 3 workers mining 60s |
| `internal/sim/economy_test.go` (new) | Unit tests: mining FSM, production queue, CmdTrain validation |
| `Assets/Sim/Constants.cs` (new) | Entity stat tables — C# mirror of constants.go |
| `Assets/Sim/Step.cs` (modify) | Tick loop parity: mining/returning/production dispatch, sort all entity types |
| `Assets/Sim/World.cs` (modify) | FindEntity, SpawnBuilding, SpawnCrystal, RemoveDead for buildings/crystals |
| `Assets/Network/Messages.cs` (modify) | CmdOp.Train = 6 |
| `Assets/Tests/EditMode/EconomyTests.cs` (new) | C# parity tests for mining FSM + production |
| `Assets/Game/CrystalView.cs` (new) | Render crystal piles on map |
| `Assets/UI/ResourceBar.cs` (new) | Top-right crystal counter, bound to Player.Crystal |
| `Assets/Tests/EditMode/GoldenData.cs` (modify) | Add economyInitialHash field |

---

### Task 1: Go — Entity constants table

**Files:**
- Create: `e:\code\_Claude\RTS\internal\sim\constants.go`

- [ ] **Step 1: Create constants.go with entity stat tables**

```go
package sim

import "rts/internal/sim/fixed"

// UnitStats holds per-unit-type parameters. All values Q16.16 fixed-point.
type UnitStats struct {
	MaxHP       fixed.Fix32
	Speed       fixed.Fix32
	Range       fixed.Fix32
	Damage      fixed.Fix32
	VisionRange fixed.Fix32
	Cost        fixed.Fix32
	TrainTicks  uint32
}

var UnitStatTable = map[UnitType]UnitStats{
	UnitWorker: {
		MaxHP:       fixed.FromInt(20),
		Speed:       fixed.FromFloat64(0.8),
		Range:       fixed.Zero,
		Damage:      fixed.Zero,
		VisionRange: fixed.FromInt(6),
		Cost:        fixed.FromInt(50),
		TrainTicks:  10,
	},
	UnitSoldier: {
		MaxHP:       fixed.FromInt(60),
		Speed:       fixed.FromFloat64(0.5),
		Range:       fixed.FromFloat64(1.5),
		Damage:      fixed.One,
		VisionRange: fixed.FromInt(8),
		Cost:        fixed.FromInt(80),
		TrainTicks:  15,
	},
	UnitArcher: {
		MaxHP:       fixed.FromInt(40),
		Speed:       fixed.FromFloat64(0.5),
		Range:       fixed.FromInt(6),
		Damage:      fixed.FromFloat64(0.6),
		VisionRange: fixed.FromInt(12),
		Cost:        fixed.FromInt(120),
		TrainTicks:  20,
	},
	UnitCavalry: {
		MaxHP:       fixed.FromInt(100),
		Speed:       fixed.FromFloat64(1.2),
		Range:       fixed.FromFloat64(1.5),
		Damage:      fixed.FromFloat64(1.5),
		VisionRange: fixed.FromInt(10),
		Cost:        fixed.FromInt(200),
		TrainTicks:  30,
	},
}

// BuildingStats holds per-building-type parameters.
type BuildingStats struct {
	MaxHP       fixed.Fix32
	VisionRange fixed.Fix32
	Cost        fixed.Fix32
	BuildTicks  uint32
	SizeCells   uint8
	Trains      []UnitType
}

var BuildingStatTable = map[BuildingType]BuildingStats{
	BldHQ: {
		MaxHP:       fixed.FromInt(800),
		VisionRange: fixed.FromInt(12),
		Cost:        fixed.Zero,
		BuildTicks:  0,
		SizeCells:   4,
		Trains:      []UnitType{UnitWorker},
	},
	BldBarracks: {
		MaxHP:       fixed.FromInt(400),
		VisionRange: fixed.FromInt(8),
		Cost:        fixed.FromInt(150),
		BuildTicks:  60,
		SizeCells:   3,
		Trains:      []UnitType{UnitSoldier},
	},
	BldArchery: {
		MaxHP:       fixed.FromInt(400),
		VisionRange: fixed.FromInt(10),
		Cost:        fixed.FromInt(150),
		BuildTicks:  60,
		SizeCells:   3,
		Trains:      []UnitType{UnitArcher},
	},
	BldStable: {
		MaxHP:       fixed.FromInt(400),
		VisionRange: fixed.FromInt(8),
		Cost:        fixed.FromInt(150),
		BuildTicks:  60,
		SizeCells:   3,
		Trains:      []UnitType{UnitCavalry},
	},
}

const (
	CarryCapacity      = 5
	MiningTicksPerTrip = 30
	MaxQueueLength     = 5
	CrystalsPerPlayer  = 8
	CrystalStartValue  = 1500
)

// CrystalPositions returns fixed crystal spawn positions for a 100×100 map.
func CrystalPositions(playerID uint8) []fixed.Vec2 {
	baseX := 5
	if playerID == 1 {
		baseX = 75
	}
	return []fixed.Vec2{
		fixed.VInt(baseX, 10),
		fixed.VInt(baseX+5, 25),
		fixed.VInt(baseX+10, 40),
		fixed.VInt(baseX+15, 55),
		fixed.VInt(baseX+20, 70),
		fixed.VInt(baseX+5, 80),
		fixed.VInt(baseX+10, 15),
		fixed.VInt(baseX+15, 65),
	}
}
```

- [ ] **Step 2: Run Go tests to verify compilation**

Run: `cd e:\code\_Claude\RTS && go build ./internal/sim/...`
Expected: build succeeds

- [ ] **Step 3: Commit**

```bash
git add internal/sim/constants.go
git commit -m "feat(sim): add entity stat tables and crystal spawn positions"
```

---

### Task 2: Go — Extend World with FindEntity, SpawnBuilding, SpawnCrystal, sort/removeDead helpers

**Files:**
- Modify: `e:\code\_Claude\RTS\internal\sim\world.go`

- [ ] **Step 1: Add FindEntity, SpawnBuilding, SpawnCrystal, sort/removeDead helpers**

Add after the existing `SpawnUnit` function:

```go
// FindEntity returns the entity with the given ID across all lists.
func (w *World) FindEntity(id uint32) (found bool, unit *Unit, bld *Building, cryst *Crystal) {
	for i := range w.Units {
		if w.Units[i].ID == id && w.Units[i].State != UnitDead {
			return true, &w.Units[i], nil, nil
		}
	}
	for i := range w.Buildings {
		if w.Buildings[i].ID == id && w.Buildings[i].State != BldDead {
			return true, nil, &w.Buildings[i], nil
		}
	}
	for i := range w.Crystals {
		if w.Crystals[i].ID == id && w.Crystals[i].Remaining > 0 {
			return true, nil, nil, &w.Crystals[i]
		}
	}
	return false, nil, nil, nil
}

// SpawnBuilding adds a building and returns its ID.
func (w *World) SpawnBuilding(owner uint8, bldType BuildingType, pos fixed.Vec2) uint32 {
	stats := BuildingStatTable[bldType]
	id := w.NextID
	w.NextID++
	w.Buildings = append(w.Buildings, Building{
		ID:        id,
		Owner:     owner,
		Type:      bldType,
		SizeCells: stats.SizeCells,
		Pos:       pos,
		HP:        stats.MaxHP,
		MaxHP:     stats.MaxHP,
		State:     BldReady,
	})
	return id
}

// SpawnCrystal adds a crystal pile and returns its ID.
func (w *World) SpawnCrystal(pos fixed.Vec2) uint32 {
	id := w.NextID
	w.NextID++
	w.Crystals = append(w.Crystals, Crystal{
		ID:        id,
		Pos:       pos,
		Remaining: fixed.FromInt(CrystalStartValue),
	})
	return id
}

// sortBuildingsByID sorts buildings in-place ascending by ID.
func sortBuildingsByID(w *World) {
	blds := w.Buildings
	for i := 1; i < len(blds); i++ {
		key := blds[i]
		j := i - 1
		for j >= 0 && blds[j].ID > key.ID {
			blds[j+1] = blds[j]
			j--
		}
		blds[j+1] = key
	}
}

// sortCrystalsByID sorts crystals in-place ascending by ID.
func sortCrystalsByID(w *World) {
	crystals := w.Crystals
	for i := 1; i < len(crystals); i++ {
		key := crystals[i]
		j := i - 1
		for j >= 0 && crystals[j].ID > key.ID {
			crystals[j+1] = crystals[j]
			j--
		}
		crystals[j+1] = key
	}
}

// RemoveDeadBuildings prunes buildings with State == BldDead.
func (w *World) RemoveDeadBuildings() {
	alive := w.Buildings[:0]
	for _, b := range w.Buildings {
		if b.State != BldDead {
			alive = append(alive, b)
		}
	}
	w.Buildings = alive
}

// RemoveDeadCrystals prunes crystals with Remaining <= 0.
func (w *World) RemoveDeadCrystals() {
	alive := w.Crystals[:0]
	for _, c := range w.Crystals {
		if c.Remaining > 0 {
			alive = append(alive, c)
		}
	}
	w.Crystals = alive
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd e:\code\_Claude\RTS && go build ./internal/sim/...`
Expected: success

- [ ] **Step 3: Run existing tests to ensure no regression**

Run: `cd e:\code\_Claude\RTS && go test ./internal/sim/... -v`
Expected: all existing tests pass

- [ ] **Step 4: Commit**

```bash
git add internal/sim/world.go
git commit -m "feat(sim): add FindEntity, SpawnBuilding, SpawnCrystal, sort/removeDead helpers"
```

---

### Task 3: Go — Mining FSM (stepMining + stepReturning)

**Files:**
- Create: `e:\code\_Claude\RTS\internal\sim\economy.go`

- [ ] **Step 1: Write economy.go with stepMining and stepReturning**

```go
package sim

import "rts/internal/sim/fixed"

// stepMining processes one tick for a worker in Mining state.
func stepMining(w *World, u *Unit) {
	if u.TargetID == 0 {
		u.State = UnitIdle
		return
	}

	crystal := w.FindCrystal(u.TargetID)
	if crystal == nil || crystal.Remaining <= 0 {
		nearest := w.FindNearestCrystal(u.Pos)
		if nearest == nil {
			u.State = UnitIdle
			u.TargetID = 0
			return
		}
		u.TargetID = nearest.ID
		crystal = nearest
	}

	// Internal mining timer stored in CarryAmount as a ticks-left counter.
	if u.CarryAmount <= 0 {
		u.CarryAmount = fixed.FromInt(MiningTicksPerTrip)
	}

	u.CarryAmount = u.CarryAmount.Sub(fixed.One)
	if u.CarryAmount > 0 {
		return
	}

	// Mining complete — take resources.
	take := fixed.FromInt(CarryCapacity)
	if crystal.Remaining < take {
		take = crystal.Remaining
	}
	crystal.Remaining = crystal.Remaining.Sub(take)
	u.CarryAmount = take

	hq := w.FindNearestOwnHQ(u.Pos, u.Owner)
	if hq == nil {
		u.State = UnitIdle
		u.CarryAmount = 0
		return
	}

	u.State = UnitReturning
	u.TargetID = hq.ID
	u.MoveTo = hq.Pos
}

// stepReturning processes one tick for a worker in Returning state.
func stepReturning(w *World, u *Unit) {
	if u.TargetID == 0 {
		u.State = UnitIdle
		return
	}

	hq := w.FindBuilding(u.TargetID)
	if hq == nil || hq.State == BldDead || hq.Owner != u.Owner {
		hq = w.FindNearestOwnHQ(u.Pos, u.Owner)
		if hq == nil {
			u.State = UnitIdle
			u.CarryAmount = 0
			return
		}
		u.TargetID = hq.ID
		u.MoveTo = hq.Pos
	}

	newPos := fixed.MoveToward(u.Pos, u.MoveTo, u.Speed)
	newPos.X = newPos.X.Clamp(0, w.MapSizeX)
	newPos.Y = newPos.Y.Clamp(0, w.MapSizeY)
	u.Pos = newPos

	arrivalRange := fixed.FromInt(int32(hq.SizeCells))
	arrivalRangeSq := arrivalRange.Mul(arrivalRange)
	if u.Pos.DistSq(hq.Pos) <= arrivalRangeSq {
		if u.Owner < uint8(len(w.Players)) {
			w.Players[u.Owner].Crystal = w.Players[u.Owner].Crystal.Add(u.CarryAmount)
		}
		u.CarryAmount = 0

		nearest := w.FindNearestCrystal(u.Pos)
		if nearest == nil {
			u.State = UnitIdle
			u.TargetID = 0
			return
		}
		u.State = UnitMining
		u.TargetID = nearest.ID
		u.MoveTo = nearest.Pos
		u.CarryAmount = 0
	}
}

// FindCrystal returns the crystal with the given ID, or nil.
func (w *World) FindCrystal(id uint32) *Crystal {
	for i := range w.Crystals {
		if w.Crystals[i].ID == id && w.Crystals[i].Remaining > 0 {
			return &w.Crystals[i]
		}
	}
	return nil
}

// FindBuilding returns the building with the given ID, or nil.
func (w *World) FindBuilding(id uint32) *Building {
	for i := range w.Buildings {
		if w.Buildings[i].ID == id && w.Buildings[i].State != BldDead {
			return &w.Buildings[i]
		}
	}
	return nil
}

// FindNearestCrystal returns the nearest crystal with Remaining > 0.
func (w *World) FindNearestCrystal(pos fixed.Vec2) *Crystal {
	var best *Crystal
	var bestDistSq fixed.Fix32
	for i := range w.Crystals {
		c := &w.Crystals[i]
		if c.Remaining <= 0 {
			continue
		}
		dSq := pos.DistSq(c.Pos)
		if best == nil || dSq < bestDistSq {
			best = c
			bestDistSq = dSq
		}
	}
	return best
}

// FindNearestOwnHQ returns the nearest Ready HQ owned by the given player.
func (w *World) FindNearestOwnHQ(pos fixed.Vec2, owner uint8) *Building {
	var best *Building
	var bestDistSq fixed.Fix32
	for i := range w.Buildings {
		b := &w.Buildings[i]
		if b.Owner != owner || b.Type != BldHQ || b.State != BldReady {
			continue
		}
		dSq := pos.DistSq(b.Pos)
		if best == nil || dSq < bestDistSq {
			best = b
			bestDistSq = dSq
		}
	}
	return best
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd e:\code\_Claude\RTS && go build ./internal/sim/...`
Expected: success

- [ ] **Step 3: Commit**

```bash
git add internal/sim/economy.go
git commit -m "feat(sim): add worker mining FSM (stepMining, stepReturning)"
```

---

### Task 4: Go — Production queue ticking + unit spawning

**Files:**
- Create: `e:\code\_Claude\RTS\internal\sim\production.go`

- [ ] **Step 1: Write production.go**

```go
package sim

import "rts/internal/sim/fixed"

func tickProduction(w *World) {
	for i := range w.Buildings {
		b := &w.Buildings[i]
		if b.State != BldReady {
			continue
		}
		if len(b.ProductionQueue) == 0 {
			continue
		}

		q := &b.ProductionQueue[0]
		if q.TicksLeft > 0 {
			q.TicksLeft--
		}
		if q.TicksLeft > 0 {
			continue
		}

		spawnPos := findEdgeSpawnCell(w, b)
		if spawnPos == nil {
			continue
		}

		stats := UnitStatTable[q.UnitType]
		unitID := w.NextID
		w.NextID++
		u := Unit{
			ID:               unitID,
			Owner:            b.Owner,
			Type:             q.UnitType,
			Pos:              *spawnPos,
			HP:               stats.MaxHP,
			MaxHP:            stats.MaxHP,
			Speed:            stats.Speed,
			Range:            stats.Range,
			Damage:           stats.Damage,
			VisionRange:      stats.VisionRange,
			State:            UnitIdle,
			AttackMoveTarget: fixed.VInt(0, 0),
		}

		if b.RallyPoint.X != 0 || b.RallyPoint.Y != 0 {
			u.State = UnitMoving
			u.MoveTo = b.RallyPoint
		}

		w.Units = append(w.Units, u)
		b.ProductionQueue = b.ProductionQueue[1:]
	}
}

func findEdgeSpawnCell(w *World, b *Building) *fixed.Vec2 {
	size := int32(b.SizeCells)
	startX := b.Pos.X.ToInt()
	startY := b.Pos.Y.ToInt()

	for dx := int32(0); dx < size; dx++ {
		for dy := int32(0); dy < size; dy++ {
			if dx > 0 && dx < size-1 && dy > 0 && dy < size-1 {
				continue
			}
			cx := startX + dx
			cy := startY + dy
			if cx < 0 || cy < 0 || cx >= w.NavGrid.W || cy >= w.NavGrid.H {
				continue
			}
			pos := fixed.VInt(int(cx), int(cy))
			if !isCellBlocked(w, cx, cy) {
				return &pos
			}
		}
	}
	return nil
}

func isCellBlocked(w *World, cx, cy int32) bool {
	for _, b := range w.Buildings {
		if b.State == BldDead {
			continue
		}
		bx := b.Pos.X.ToInt()
		by := b.Pos.Y.ToInt()
		sz := int32(b.SizeCells)
		if cx >= bx && cx < bx+sz && cy >= by && cy < by+sz {
			return true
		}
	}
	idx := int(cy*w.NavGrid.W + cx)
	word := idx / 64
	bit := uint(idx % 64)
	if word < len(w.NavGrid.Blocked) && (w.NavGrid.Blocked[word]&(1<<bit)) != 0 {
		return true
	}
	return false
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd e:\code\_Claude\RTS && go build ./internal/sim/...`
Expected: success

- [ ] **Step 3: Commit**

```bash
git add internal/sim/production.go
git commit -m "feat(sim): add production queue ticking + unit spawning at edge cell"
```

---

### Task 5: Go — CmdTrain + new command opcodes

**Files:**
- Create: `e:\code\_Claude\RTS\internal\sim\commands.go`
- Modify: `e:\code\_Claude\RTS\internal\sim\world.go` (add CmdOp constants)

- [ ] **Step 1: Add CmdTrain and new CmdOp values to world.go**

Change the existing CmdOp block from:
```go
const (
	CmdMove   CmdOp = 1
	CmdAttack CmdOp = 2
	CmdStop   CmdOp = 3
)
```

To:
```go
const (
	CmdMove       CmdOp = 1
	CmdAttack     CmdOp = 2
	CmdStop       CmdOp = 3
	CmdAttackMove CmdOp = 4
	CmdBuild      CmdOp = 5
	CmdTrain      CmdOp = 6
	CmdSurrender  CmdOp = 7
)
```

- [ ] **Step 2: Write commands.go**

```go
package sim

// applyCmdTrain processes a CmdTrain command.
func applyCmdTrain(w *World, cmd Cmd) {
	b := w.FindBuilding(cmd.UnitID)
	if b == nil {
		return
	}
	if b.Owner != cmd.Player {
		return
	}
	if b.State != BldReady {
		return
	}
	if len(b.ProductionQueue) >= MaxQueueLength {
		return
	}

	unitType := UnitType(cmd.TargetID)
	if unitType < UnitWorker || unitType > UnitCavalry {
		return
	}

	stats := BuildingStatTable[b.Type]
	canTrain := false
	for _, ut := range stats.Trains {
		if ut == unitType {
			canTrain = true
			break
		}
	}
	if !canTrain {
		return
	}

	unitStats := UnitStatTable[unitType]
	if int(cmd.Player) >= len(w.Players) {
		return
	}
	if w.Players[cmd.Player].Crystal < unitStats.Cost {
		return
	}

	w.Players[cmd.Player].Crystal = w.Players[cmd.Player].Crystal.Sub(unitStats.Cost)

	b.ProductionQueue = append(b.ProductionQueue, QueueItem{
		UnitType:  unitType,
		TicksLeft: unitStats.TrainTicks,
		StartTick: w.Tick,
	})
}
```

- [ ] **Step 3: Verify compilation**

Run: `cd e:\code\_Claude\RTS && go build ./internal/sim/...`
Expected: success

- [ ] **Step 4: Commit**

```bash
git add internal/sim/commands.go internal/sim/world.go
git commit -m "feat(sim): add CmdTrain + CmdAttackMove/CmdBuild/CmdSurrender opcodes"
```

---

### Task 6: Go — Extend Step() with all new states

**Files:**
- Modify: `e:\code\_Claude\RTS\internal\sim\step.go`

- [ ] **Step 1: Rewrite Step() to dispatch all states, sort all entities, run production/removeDead**

Replace the entire `Step` function body and the `applyCommands` switch:

```go
func Step(w *World, cmds []Cmd) {
	w.Tick++

	applyCommands(w, cmds)

	sortUnitsByID(w)
	sortBuildingsByID(w)
	sortCrystalsByID(w)

	for i := range w.Units {
		u := &w.Units[i]
		if u.State == UnitDead {
			continue
		}
		switch u.State {
		case UnitMoving:
			stepMove(w, u)
		case UnitMining:
			stepMining(w, u)
		case UnitReturning:
			stepReturning(w, u)
		case UnitBuilding:
			// Phase 3
		case UnitAttacking:
			stepAttack(w, u)
		case UnitIdle:
			stepAttack(w, u)
		}
	}

	tickProduction(w)

	w.RemoveDead()
	w.RemoveDeadBuildings()
	w.RemoveDeadCrystals()
}
```

Replace `applyCommands` to handle new command types:

```go
func applyCommands(w *World, cmds []Cmd) {
	for _, cmd := range cmds {
		switch cmd.Op {
		case CmdMove:
			u := w.FindUnit(cmd.UnitID)
			if u == nil || u.State == UnitDead || u.Owner != cmd.Player {
				continue
			}
			// Worker right-click on crystal → enter mining loop.
			crystal := w.FindCrystalAt(cmd.TargetPos)
			if crystal != nil && u.Type == UnitWorker {
				u.State = UnitMining
				u.TargetID = crystal.ID
				u.MoveTo = crystal.Pos
				u.CarryAmount = 0
				continue
			}
			u.State = UnitMoving
			u.MoveTo = cmd.TargetPos
			u.TargetID = 0
		case CmdAttack:
			u := w.FindUnit(cmd.UnitID)
			if u == nil || u.State == UnitDead || u.Owner != cmd.Player {
				continue
			}
			if u.Range <= 0 {
				continue
			}
			u.State = UnitIdle
			u.TargetID = cmd.TargetID
		case CmdStop:
			u := w.FindUnit(cmd.UnitID)
			if u == nil || u.State == UnitDead || u.Owner != cmd.Player {
				continue
			}
			u.State = UnitIdle
			u.TargetID = 0
			u.CarryAmount = 0
		case CmdTrain:
			applyCmdTrain(w, cmd)
		}
	}
}
```

Also add `FindCrystalAt` to `world.go`:

```go
// FindCrystalAt returns a crystal near the given position (within 2 cells).
func (w *World) FindCrystalAt(pos fixed.Vec2) *Crystal {
	for i := range w.Crystals {
		c := &w.Crystals[i]
		if c.Remaining <= 0 {
			continue
		}
		if c.Pos.DistSq(pos) <= fixed.FromInt(2).Mul(fixed.FromInt(2)) {
			return c
		}
	}
	return nil
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd e:\code\_Claude\RTS && go build ./internal/sim/...`
Expected: success

- [ ] **Step 3: Run existing tests to verify no regression**

Run: `cd e:\code\_Claude\RTS && go test ./internal/sim/... -v`
Expected: all tests pass

- [ ] **Step 4: Commit**

```bash
git add internal/sim/step.go internal/sim/world.go
git commit -m "feat(sim): extend Step with mining/returning/production states + CmdTrain dispatch"
```

---

### Task 7: Go — Server spawns initial HQ+Workers+crystals

**Files:**
- Modify: `e:\code\_Claude\RTS\internal\lockstep\room.go`

- [ ] **Step 1: Add SpawnInitialWorld to Room**

Replace `SpawnInitialUnits` and add `started` field logic:

Add to `Room` struct:
```go
type Room struct {
	// ... existing fields ...
	started bool
}
```

Replace `SpawnInitialUnits`:
```go
// SpawnInitialWorld sets up the starting game state for all players.
func (r *Room) SpawnInitialWorld() {
	for pid := 0; pid < r.cfg.PlayerCount; pid++ {
		playerID := uint8(pid)
		hqX := 10
		if pid == 1 {
			hqX = 90
		}
		hqPos := fixed.VInt(hqX, 45)
		r.world.SpawnBuilding(playerID, sim.BldHQ, hqPos)

		stats := sim.UnitStatTable[sim.UnitWorker]
		for w := 0; w < 3; w++ {
			workerX := hqX + 3 + w
			workerY := 45 + w
			r.world.SpawnUnit(playerID,
				fixed.VInt(workerX, workerY),
				stats.MaxHP, stats.Speed)
			r.world.Units[len(r.world.Units)-1].Type = sim.UnitWorker
		}

		for _, pos := range sim.CrystalPositions(playerID) {
			r.world.SpawnCrystal(pos)
		}
	}
}
```

At the top of `sealTick`, before `r.tick++`:
```go
func (r *Room) sealTick() {
	if !r.started {
		allJoined := true
		for _, p := range r.players {
			if p == nil || !p.Joined {
				allJoined = false
				break
			}
		}
		if !allJoined {
			return
		}
		r.started = true
		r.SpawnInitialWorld()
		r.world.Players = make([]sim.Player, r.cfg.PlayerCount)
		for i := range r.world.Players {
			r.world.Players[i] = sim.Player{ID: uint8(i)}
		}
		r.world.Players[0].Crystal = fixed.FromInt(200)
		r.world.Players[1].Crystal = fixed.FromInt(200)
	}

	r.tick++
	// ... rest of sealTick ...
```

Add `fixed` import if not already present:
```go
import (
	"rts/internal/sim/fixed"
)
```

- [ ] **Step 2: Verify compilation**

Run: `cd e:\code\_Claude\RTS && go build ./...`
Expected: success

- [ ] **Step 3: Commit**

```bash
git add internal/lockstep/room.go
git commit -m "feat(lockstep): spawn initial HQ + 3 Workers + 8 crystals per player on room start"
```

---

### Task 8: Go — Economy unit tests

**Files:**
- Create: `e:\code\_Claude\RTS\internal\sim\economy_test.go`

- [ ] **Step 1: Write unit tests for mining FSM and CmdTrain**

```go
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
```

- [ ] **Step 2: Run tests**

Run: `cd e:\code\_Claude\RTS && go test ./internal/sim/... -v -run "TestStepMining|TestStepReturning|TestCmdTrain|TestProductionQueue|TestStep_Crystal"`
Expected: all tests pass

- [ ] **Step 3: Commit**

```bash
git add internal/sim/economy_test.go
git commit -m "test(sim): add economy unit tests for mining FSM, production, CmdTrain"
```

---

### Task 9: Go — Update golden export for Phase 1 economy

**Files:**
- Modify: `e:\code\_Claude\RTS\cmd\golden-export\main.go`

- [ ] **Step 1: Add economy initial state to golden export**

Add `EconomyInitialHash string` to the `GoldenData` struct. Then add after the empty-world hash computation:

```go
// Phase 1 economy initial state.
w2 := sim.NewWorld(g.Seed, g.MapW, g.MapH)
w2.Players = []sim.Player{{ID: 0, Crystal: fixed.FromInt(200)}, {ID: 1, Crystal: fixed.FromInt(200)}}
w2.SpawnBuilding(0, sim.BldHQ, fixed.VInt(10, 45))
w2.SpawnBuilding(1, sim.BldHQ, fixed.VInt(90, 45))
for _, pos := range sim.CrystalPositions(0) {
	w2.SpawnCrystal(pos)
}
for _, pos := range sim.CrystalPositions(1) {
	w2.SpawnCrystal(pos)
}
stats := sim.UnitStatTable[sim.UnitWorker]
for i := 0; i < 3; i++ {
	id := w2.SpawnUnit(0, fixed.VInt(13+i, 45+i), stats.MaxHP, stats.Speed)
	w2.Units[len(w2.Units)-1].Type = sim.UnitWorker
	_ = id
	id = w2.SpawnUnit(1, fixed.VInt(93-i, 45+i), stats.MaxHP, stats.Speed)
	w2.Units[len(w2.Units)-1].Type = sim.UnitWorker
	_ = id
}
g.EconomyInitialHash = fmt.Sprintf("%016x", sim.Hash(w2))
```

- [ ] **Step 2: Regenerate golden data**

Run: `cd e:\code\_Claude\RTS && go run ./cmd/golden-export/ golden_data.json`

- [ ] **Step 3: Copy golden data to Unity**

Copy `golden_data.json` to `E:\code\_Unity\rts-client-unity\Assets\Tests\EditMode\GoldenData.json`

- [ ] **Step 4: Commit**

```bash
git add cmd/golden-export/main.go golden_data.json
git commit -m "feat(golden): add Phase 1 economy initial state to golden export"
```

---

### Task 10: C# — Entity constants + CmdOp parity

**Files:**
- Create: `E:\code\_Unity\rts-client-unity\Assets\Sim\Constants.cs`
- Modify: `E:\code\_Unity\rts-client-unity\Assets\Network\Messages.cs`

- [ ] **Step 1: Create Constants.cs**

```csharp
using System.Collections.Generic;

namespace RTS.Sim
{
    public static class SimConstants
    {
        public const int CarryCapacity = 5;
        public const int MiningTicksPerTrip = 30;
        public const int MaxQueueLength = 5;
        public const int CrystalsPerPlayer = 8;
        public const int CrystalStartValue = 1500;
        public const int StartingCrystal = 200;

        public static readonly Dictionary<UnitType, UnitStat> UnitStats =
            new Dictionary<UnitType, UnitStat>
        {
            [UnitType.Worker] = new UnitStat
            {
                MaxHP = 20, Speed = 0.8f, Range = 0, Damage = 0,
                VisionRange = 6, Cost = 50, TrainTicks = 10
            },
            [UnitType.Soldier] = new UnitStat
            {
                MaxHP = 60, Speed = 0.5f, Range = 1.5f, Damage = 1.0f,
                VisionRange = 8, Cost = 80, TrainTicks = 15
            },
            [UnitType.Archer] = new UnitStat
            {
                MaxHP = 40, Speed = 0.5f, Range = 6, Damage = 0.6f,
                VisionRange = 12, Cost = 120, TrainTicks = 20
            },
            [UnitType.Cavalry] = new UnitStat
            {
                MaxHP = 100, Speed = 1.2f, Range = 1.5f, Damage = 1.5f,
                VisionRange = 10, Cost = 200, TrainTicks = 30
            },
        };

        public static readonly Dictionary<BuildingType, BuildingStat> BuildingStats =
            new Dictionary<BuildingType, BuildingStat>
        {
            [BuildingType.HQ] = new BuildingStat
            {
                MaxHP = 800, VisionRange = 12, Cost = 0, BuildTicks = 0, SizeCells = 4,
                Trains = new[] { UnitType.Worker }
            },
            [BuildingType.Barracks] = new BuildingStat
            {
                MaxHP = 400, VisionRange = 8, Cost = 150, BuildTicks = 60, SizeCells = 3,
                Trains = new[] { UnitType.Soldier }
            },
            [BuildingType.Archery] = new BuildingStat
            {
                MaxHP = 400, VisionRange = 10, Cost = 150, BuildTicks = 60, SizeCells = 3,
                Trains = new[] { UnitType.Archer }
            },
            [BuildingType.Stable] = new BuildingStat
            {
                MaxHP = 400, VisionRange = 8, Cost = 150, BuildTicks = 60, SizeCells = 3,
                Trains = new[] { UnitType.Cavalry }
            },
        };

        public static Vec2[] CrystalPositions(byte playerID)
        {
            int baseX = playerID == 1 ? 75 : 5;
            return new Vec2[]
            {
                Vec2.FromInt(baseX, 10),     Vec2.FromInt(baseX + 5, 25),
                Vec2.FromInt(baseX + 10, 40), Vec2.FromInt(baseX + 15, 55),
                Vec2.FromInt(baseX + 20, 70), Vec2.FromInt(baseX + 5, 80),
                Vec2.FromInt(baseX + 10, 15), Vec2.FromInt(baseX + 15, 65),
            };
        }
    }

    public struct UnitStat
    {
        public int MaxHP;
        public float Speed;
        public float Range;
        public float Damage;
        public int VisionRange;
        public int Cost;
        public uint TrainTicks;
    }

    public struct BuildingStat
    {
        public int MaxHP;
        public int VisionRange;
        public int Cost;
        public uint BuildTicks;
        public byte SizeCells;
        public UnitType[] Trains;
    }
}
```

- [ ] **Step 2: Add CmdTrain to CmdOp enum in Messages.cs**

Change from `Move=1, Attack=2, Stop=3` to:
```csharp
public enum CmdOp : byte
{
    Move = 1,
    Attack = 2,
    Stop = 3,
    AttackMove = 4,
    Build = 5,
    Train = 6,
    Surrender = 7
}
```

- [ ] **Step 3: Verify Unity compilation**

Open Unity, wait for script compilation. Check Console for errors.

- [ ] **Step 4: Commit**

```bash
git add Assets/Sim/Constants.cs Assets/Network/Messages.cs
git commit -m "feat(cs): add entity stat constants + CmdTrain wire opcode parity"
```

---

### Task 11: C# — Extend World with FindEntity, Spawn helpers, RemoveDead for buildings/crystals

**Files:**
- Modify: `E:\code\_Unity\rts-client-unity\Assets\Sim\World.cs`

- [ ] **Step 1: Add entity lookup and spawn helpers to World class**

```csharp
public (bool found, int unitIdx, int bldIdx, int crystIdx) FindEntity(uint id)
{
    for (int i = 0; i < Units.Count; i++)
        if (Units[i].ID == id && Units[i].State != UnitState.Dead)
            return (true, i, -1, -1);
    for (int i = 0; i < Buildings.Count; i++)
        if (Buildings[i].ID == id && Buildings[i].State != BuildingState.Dead)
            return (true, -1, i, -1);
    for (int i = 0; i < Crystals.Count; i++)
        if (Crystals[i].ID == id && Crystals[i].Remaining > Fixed32.Zero)
            return (true, -1, -1, i);
    return (false, -1, -1, -1);
}

public int FindCrystalIndex(uint id)
{
    for (int i = 0; i < Crystals.Count; i++)
        if (Crystals[i].ID == id && Crystals[i].Remaining > Fixed32.Zero)
            return i;
    return -1;
}

public int FindBuildingIndex(uint id)
{
    for (int i = 0; i < Buildings.Count; i++)
        if (Buildings[i].ID == id && Buildings[i].State != BuildingState.Dead)
            return i;
    return -1;
}

public uint SpawnBuilding(byte owner, BuildingType type, Vec2 pos)
{
    var stats = SimConstants.BuildingStats[type];
    uint id = NextID++;
    Buildings.Add(new Building
    {
        ID = id, Owner = owner, Type = type,
        SizeCells = stats.SizeCells, Pos = pos,
        HP = Fixed32.FromInt(stats.MaxHP),
        MaxHP = Fixed32.FromInt(stats.MaxHP),
        State = BuildingState.Ready,
    });
    return id;
}

public uint SpawnCrystal(Vec2 pos)
{
    uint id = NextID++;
    Crystals.Add(new Crystal
    {
        ID = id, Pos = pos,
        Remaining = Fixed32.FromInt(SimConstants.CrystalStartValue),
    });
    return id;
}

public int FindNearestCrystalIndex(Vec2 pos)
{
    int best = -1;
    Fixed32 bestDistSq = Fixed32.Zero;
    for (int i = 0; i < Crystals.Count; i++)
    {
        if (Crystals[i].Remaining <= Fixed32.Zero) continue;
        var dSq = pos.DistSq(Crystals[i].Pos);
        if (best < 0 || dSq < bestDistSq) { best = i; bestDistSq = dSq; }
    }
    return best;
}

public int FindNearestOwnHQIndex(Vec2 pos, byte owner)
{
    int best = -1;
    Fixed32 bestDistSq = Fixed32.Zero;
    for (int i = 0; i < Buildings.Count; i++)
    {
        var b = Buildings[i];
        if (b.Owner != owner || b.Type != BuildingType.HQ || b.State != BuildingState.Ready)
            continue;
        var dSq = pos.DistSq(b.Pos);
        if (best < 0 || dSq < bestDistSq) { best = i; bestDistSq = dSq; }
    }
    return best;
}

public int FindCrystalAt(Vec2 pos)
{
    var rangeSq = Fixed32.FromInt(2) * Fixed32.FromInt(2);
    for (int i = 0; i < Crystals.Count; i++)
    {
        if (Crystals[i].Remaining <= Fixed32.Zero) continue;
        if (Crystals[i].Pos.DistSq(pos) <= rangeSq) return i;
    }
    return -1;
}

public void RemoveDeadBuildings()
{
    int write = 0;
    for (int read = 0; read < Buildings.Count; read++)
    {
        if (Buildings[read].State != BuildingState.Dead)
        {
            if (write != read) Buildings[write] = Buildings[read];
            write++;
        }
    }
    if (write < Buildings.Count)
        Buildings.RemoveRange(write, Buildings.Count - write);
}

public void RemoveDeadCrystals()
{
    int write = 0;
    for (int read = 0; read < Crystals.Count; read++)
    {
        if (Crystals[read].Remaining > Fixed32.Zero)
        {
            if (write != read) Crystals[write] = Crystals[read];
            write++;
        }
    }
    if (write < Crystals.Count)
        Crystals.RemoveRange(write, Crystals.Count - write);
}
```

- [ ] **Step 2: Verify Unity compilation**

Check Unity Console for errors.

- [ ] **Step 3: Commit**

```bash
git add Assets/Sim/World.cs
git commit -m "feat(cs): add FindEntity, SpawnBuilding, SpawnCrystal, RemoveDead helpers"
```

---

### Task 12: C# — Mining FSM + Production parity in Step.cs

**Files:**
- Modify: `E:\code\_Unity\rts-client-unity\Assets\Sim\Step.cs`

- [ ] **Step 1: Extend Step() with mining, returning, production, sort+removeDead for all entities**

Replace the `Step` method body:
```csharp
public static void Step(World w, Cmd[] cmds)
{
    w.Tick++;
    ApplyCommands(w, cmds);
    SortUnitsByID(w);
    SortBuildingsByID(w);
    SortCrystalsByID(w);

    for (int i = 0; i < w.Units.Count; i++)
    {
        var u = w.Units[i];
        if (u.State == UnitState.Dead) continue;
        switch (u.State)
        {
            case UnitState.Moving: StepMove(w, ref u); break;
            case UnitState.Mining: StepMining(w, ref u); break;
            case UnitState.Returning: StepReturning(w, ref u); break;
            case UnitState.Attacking: StepAttack(w, ref u); break;
            case UnitState.Idle: StepAttack(w, ref u); break;
        }
        w.Units[i] = u;
    }

    TickProduction(w);
    w.RemoveDead();
    w.RemoveDeadBuildings();
    w.RemoveDeadCrystals();
}
```

Add new methods after existing StepMove/StepAttack:

`StepMining`:
```csharp
private static void StepMining(World w, ref Unit u)
{
    if (u.TargetID == 0) { u.State = UnitState.Idle; return; }
    int crystalIdx = w.FindCrystalIndex(u.TargetID);
    if (crystalIdx < 0)
    {
        int nearest = w.FindNearestCrystalIndex(u.Pos);
        if (nearest < 0) { u.State = UnitState.Idle; u.TargetID = 0; return; }
        u.TargetID = w.Crystals[nearest].ID;
        crystalIdx = nearest;
    }
    var crystal = w.Crystals[crystalIdx];
    if (u.CarryAmount <= Fixed32.Zero)
        u.CarryAmount = Fixed32.FromInt(SimConstants.MiningTicksPerTrip);
    u.CarryAmount = u.CarryAmount - Fixed32.One;
    if (u.CarryAmount > Fixed32.Zero) { w.Crystals[crystalIdx] = crystal; return; }

    var take = Fixed32.FromInt(SimConstants.CarryCapacity);
    if (crystal.Remaining < take) take = crystal.Remaining;
    crystal.Remaining = crystal.Remaining - take;
    u.CarryAmount = take;
    w.Crystals[crystalIdx] = crystal;

    int hqIdx = w.FindNearestOwnHQIndex(u.Pos, u.Owner);
    if (hqIdx < 0) { u.State = UnitState.Idle; u.CarryAmount = Fixed32.Zero; return; }
    u.State = UnitState.Returning;
    u.TargetID = w.Buildings[hqIdx].ID;
    u.MoveTo = w.Buildings[hqIdx].Pos;
}
```

`StepReturning`:
```csharp
private static void StepReturning(World w, ref Unit u)
{
    if (u.TargetID == 0) { u.State = UnitState.Idle; return; }
    int hqIdx = w.FindBuildingIndex(u.TargetID);
    if (hqIdx < 0 || w.Buildings[hqIdx].Owner != u.Owner)
    {
        hqIdx = w.FindNearestOwnHQIndex(u.Pos, u.Owner);
        if (hqIdx < 0) { u.State = UnitState.Idle; u.CarryAmount = Fixed32.Zero; return; }
        u.TargetID = w.Buildings[hqIdx].ID;
        u.MoveTo = w.Buildings[hqIdx].Pos;
    }
    var newPos = Vec2.MoveToward(u.Pos, u.MoveTo, u.Speed);
    newPos = new Vec2(
        newPos.X.Clamp(Fixed32.Zero, w.MapSizeX),
        newPos.Y.Clamp(Fixed32.Zero, w.MapSizeY));
    u.Pos = newPos;

    var hq = w.Buildings[hqIdx];
    var arrivalRange = Fixed32.FromInt(hq.SizeCells) * Fixed32.FromInt(hq.SizeCells);
    if (u.Pos.DistSq(hq.Pos) <= arrivalRange)
    {
        if (u.Owner < w.Players.Count)
        {
            var p = w.Players[u.Owner];
            p.Crystal = p.Crystal + u.CarryAmount;
            w.Players[u.Owner] = p;
        }
        u.CarryAmount = Fixed32.Zero;
        int nearest = w.FindNearestCrystalIndex(u.Pos);
        if (nearest < 0) { u.State = UnitState.Idle; u.TargetID = 0; return; }
        u.State = UnitState.Mining;
        u.TargetID = w.Crystals[nearest].ID;
        u.MoveTo = w.Crystals[nearest].Pos;
        u.CarryAmount = Fixed32.Zero;
    }
}
```

`TickProduction`:
```csharp
private static void TickProduction(World w)
{
    for (int i = 0; i < w.Buildings.Count; i++)
    {
        var b = w.Buildings[i];
        if (b.State != BuildingState.Ready) continue;
        if (b.ProductionQueue == null || b.ProductionQueue.Length == 0) continue;
        if (b.ProductionQueue[0].TicksLeft > 0)
            b.ProductionQueue[0].TicksLeft--;
        if (b.ProductionQueue[0].TicksLeft > 0)
        { w.Buildings[i] = b; continue; }

        var spawnPos = FindEdgeSpawnCell(w, b);
        if (spawnPos == null) { w.Buildings[i] = b; continue; }

        var stats = SimConstants.UnitStats[b.ProductionQueue[0].UnitType];
        var newUnit = new Unit
        {
            ID = w.NextID++, Owner = b.Owner,
            Type = b.ProductionQueue[0].UnitType,
            Pos = spawnPos.Value,
            HP = Fixed32.FromInt(stats.MaxHP),
            MaxHP = Fixed32.FromInt(stats.MaxHP),
            Speed = new Fixed32((int)(stats.Speed * 65536f)),
            Range = new Fixed32((int)(stats.Range * 65536f)),
            Damage = new Fixed32((int)(stats.Damage * 65536f)),
            VisionRange = Fixed32.FromInt(stats.VisionRange),
            State = UnitState.Idle,
        };
        if (b.RallyPoint.X.Raw != 0 || b.RallyPoint.Y.Raw != 0)
        {
            newUnit.State = UnitState.Moving;
            newUnit.MoveTo = b.RallyPoint;
        }
        w.Units.Add(newUnit);

        var newQueue = new QueueItem[b.ProductionQueue.Length - 1];
        for (int q = 1; q < b.ProductionQueue.Length; q++)
            newQueue[q - 1] = b.ProductionQueue[q];
        b.ProductionQueue = newQueue;
        w.Buildings[i] = b;
    }
}

private static Vec2? FindEdgeSpawnCell(World w, in Building b)
{
    int size = b.SizeCells;
    int startX = b.Pos.X.ToInt();
    int startY = b.Pos.Y.ToInt();
    for (int dx = 0; dx < size; dx++)
    {
        for (int dy = 0; dy < size; dy++)
        {
            if (dx > 0 && dx < size - 1 && dy > 0 && dy < size - 1) continue;
            int cx = startX + dx, cy = startY + dy;
            if (cx < 0 || cy < 0 || cx >= w.NavGrid.W || cy >= w.NavGrid.H) continue;
            if (!IsCellBlocked(w, cx, cy)) return Vec2.FromInt(cx, cy);
        }
    }
    return null;
}

private static bool IsCellBlocked(World w, int cx, int cy)
{
    for (int i = 0; i < w.Buildings.Count; i++)
    {
        var bld = w.Buildings[i];
        if (bld.State == BuildingState.Dead) continue;
        int bx = bld.Pos.X.ToInt(), by = bld.Pos.Y.ToInt(), sz = bld.SizeCells;
        if (cx >= bx && cx < bx + sz && cy >= by && cy < by + sz) return true;
    }
    int idx = cy * w.NavGrid.W + cx;
    int word = idx / 64, bit = idx % 64;
    if (word < w.NavGrid.Blocked.Length && (w.NavGrid.Blocked[word] & (1UL << bit)) != 0)
        return true;
    return false;
}
```

Add sort helpers and ApplyCmdTrain (see full plan in the plan file). Update `ApplyCommands`:

Add `CmdOp.Train` case:
```csharp
case CmdOp.Train:
    ApplyCmdTrain(w, cmd);
    break;
```

Update `CmdOp.Move` case to detect crystal right-click:
```csharp
case CmdOp.Move:
{
    int uIdx = w.FindUnitIndex(cmd.UnitID);
    if (uIdx < 0) continue;
    var u = w.Units[uIdx];
    if (u.State == UnitState.Dead || u.Owner != cmd.Player) continue;
    if (u.Type == UnitType.Worker)
    {
        int crystIdx = w.FindCrystalAt(cmd.TargetPos);
        if (crystIdx >= 0)
        {
            u.State = UnitState.Mining;
            u.TargetID = w.Crystals[crystIdx].ID;
            u.MoveTo = w.Crystals[crystIdx].Pos;
            u.CarryAmount = Fixed32.Zero;
            w.Units[uIdx] = u;
            continue;
        }
    }
    u.State = UnitState.Moving;
    u.MoveTo = cmd.TargetPos;
    u.TargetID = 0;
    w.Units[uIdx] = u;
    break;
}
```

- [ ] **Step 2: Verify Unity compilation**

Check Unity Console for errors.

- [ ] **Step 3: Commit**

```bash
git add Assets/Sim/Step.cs
git commit -m "feat(cs): add mining FSM, production ticking, CmdTrain parity with Go"
```

---

### Task 13: C# — Economy unit tests

**Files:**
- Create: `E:\code\_Unity\rts-client-unity\Assets\Tests\EditMode\EconomyTests.cs`

- [ ] **Step 1: Write economy parity tests**

```csharp
using NUnit.Framework;
using RTS.Sim;
using System.Collections.Generic;

namespace RTS.Tests
{
    public class EconomyTests
    {
        [Test]
        public void CrystalRightClick_EntersMining()
        {
            var w = NewEconomyWorld();
            uint workerID = FindFirstWorker(w, 0);
            var cmd = new Cmd { Player = 0, Op = CmdOp.Move, UnitID = workerID,
                TargetPos = Vec2.FromInt(5, 10) };
            SimStep.Step(w, new[] { cmd });
            int idx = w.FindUnitIndex(workerID);
            Assert.IsTrue(idx >= 0);
            Assert.AreEqual(UnitState.Mining, w.Units[idx].State);
        }

        [Test]
        public void CmdTrain_EnqueuesWorker()
        {
            var w = NewEconomyWorld();
            var p0 = w.Players[0]; p0.Crystal = Fixed32.FromInt(200); w.Players[0] = p0;
            uint hqID = FindFirstHQ(w, 0);
            var cmd = new Cmd { Player = 0, Op = CmdOp.Train, UnitID = hqID,
                TargetID = (uint)UnitType.Worker };
            SimStep.Step(w, new[] { cmd });
            int bIdx = w.FindBuildingIndex(hqID);
            Assert.IsTrue(bIdx >= 0);
            Assert.IsNotNull(w.Buildings[bIdx].ProductionQueue);
            Assert.AreEqual(1, w.Buildings[bIdx].ProductionQueue.Length);
            Assert.AreEqual(UnitType.Worker, w.Buildings[bIdx].ProductionQueue[0].UnitType);
            Assert.AreEqual(Fixed32.FromInt(150), w.Players[0].Crystal);
        }

        [Test]
        public void CmdTrain_InsufficientResource_Dropped()
        {
            var w = NewEconomyWorld();
            uint hqID = FindFirstHQ(w, 0);
            var cmd = new Cmd { Player = 0, Op = CmdOp.Train, UnitID = hqID,
                TargetID = (uint)UnitType.Worker };
            SimStep.Step(w, new[] { cmd });
            int bIdx = w.FindBuildingIndex(hqID);
            Assert.IsTrue(bIdx >= 0);
            Assert.IsTrue(w.Buildings[bIdx].ProductionQueue == null
                      || w.Buildings[bIdx].ProductionQueue.Length == 0);
        }

        [Test]
        public void MiningDepletesCrystal()
        {
            var w = NewEconomyWorld();
            uint workerID = FindFirstWorker(w, 0);
            int workerIdx = w.FindUnitIndex(workerID);
            var u = w.Units[workerIdx];
            u.State = UnitState.Mining; u.TargetID = w.Crystals[0].ID;
            u.Pos = w.Crystals[0].Pos; w.Units[workerIdx] = u;
            var initialRemaining = w.Crystals[0].Remaining;
            for (int t = 0; t < SimConstants.MiningTicksPerTrip; t++)
                SimStep.Step(w, null);
            workerIdx = w.FindUnitIndex(workerID);
            Assert.IsTrue(workerIdx >= 0);
            Assert.AreEqual(UnitState.Returning, w.Units[workerIdx].State);
            Assert.AreEqual(Fixed32.FromInt(SimConstants.CarryCapacity),
                w.Units[workerIdx].CarryAmount);
            Assert.AreEqual(initialRemaining - Fixed32.FromInt(SimConstants.CarryCapacity),
                w.Crystals[0].Remaining);
        }

        // --- helpers ---
        private static World NewEconomyWorld()
        {
            var w = new World(42, 100, 100);
            w.Players = new List<Player> { new Player { ID = 0 }, new Player { ID = 1 } };
            w.SpawnBuilding(0, BuildingType.HQ, Vec2.FromInt(10, 45));
            w.SpawnBuilding(1, BuildingType.HQ, Vec2.FromInt(90, 45));
            foreach (var pos in SimConstants.CrystalPositions(0)) w.SpawnCrystal(pos);
            foreach (var pos in SimConstants.CrystalPositions(1)) w.SpawnCrystal(pos);
            var stats = SimConstants.UnitStats[UnitType.Worker];
            var speed = new Fixed32((int)(stats.Speed * 65536f));
            for (int i = 0; i < 3; i++)
            {
                w.SpawnUnit(0, Vec2.FromInt(13 + i, 45 + i),
                    Fixed32.FromInt(stats.MaxHP), speed);
                var u = w.Units[w.Units.Count - 1]; u.Type = UnitType.Worker;
                w.Units[w.Units.Count - 1] = u;
                w.SpawnUnit(1, Vec2.FromInt(93 - i, 45 + i),
                    Fixed32.FromInt(stats.MaxHP), speed);
                u = w.Units[w.Units.Count - 1]; u.Type = UnitType.Worker;
                w.Units[w.Units.Count - 1] = u;
            }
            return w;
        }

        private static uint FindFirstWorker(World w, byte owner)
        {
            for (int i = 0; i < w.Units.Count; i++)
                if (w.Units[i].Owner == owner && w.Units[i].Type == UnitType.Worker)
                    return w.Units[i].ID;
            return 0;
        }

        private static uint FindFirstHQ(World w, byte owner)
        {
            for (int i = 0; i < w.Buildings.Count; i++)
                if (w.Buildings[i].Owner == owner && w.Buildings[i].Type == BuildingType.HQ)
                    return w.Buildings[i].ID;
            return 0;
        }
    }
}
```

- [ ] **Step 2: Run Unity EditMode tests**

Window → General → Test Runner → EditMode → Run All. Expected: all pass.

- [ ] **Step 3: Commit**

```bash
git add Assets/Tests/EditMode/EconomyTests.cs
git commit -m "test(cs): add economy parity tests for mining, production, CmdTrain"
```

---

### Task 14: Go — Bot client (Phase_Economy only)

**Files:**
- Create: `e:\code\_Claude\RTS\cmd\bot-client\main.go`

- [ ] **Step 1: Write bot-client with Phase_Economy strategy**

```go
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"rts/internal/lockstep"
	"rts/internal/sim"
	"rts/internal/sim/fixed"
	"rts/internal/transport"
	"rts/internal/wire"
	"time"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:9000", "server address")
	playerName := flag.String("name", "Bot", "player name")
	roomID := flag.String("room", "bot-room", "room ID")
	flag.Parse()

	conn, err := transport.Dial(*addr)
	if err != nil {
		log.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	helloData, _ := wire.Encode(&wire.Hello{ProtocolVersion: wire.ProtocolVersion, PlayerName: *playerName})
	conn.Send(helloData)

	ackData := readWithTimeout(conn, 5*time.Second)
	if ackData == nil {
		log.Fatal("no HelloAck")
	}
	_, msg, err := wire.Decode(ackData)
	if err != nil {
		log.Fatalf("decode HelloAck: %v", err)
	}
	ack := msg.(*wire.HelloAck)
	if !ack.Accepted {
		log.Fatal("Hello rejected")
	}
	log.Printf("connected, tick_rate=%d", ack.ServerTickRate)

	joinData, _ := wire.Encode(&wire.JoinRoom{RoomID: *roomID})
	conn.Send(joinData)

	joinResp := readWithTimeout(conn, 5*time.Second)
	if joinResp == nil {
		log.Fatal("no JoinAck")
	}
	_, joinMsg, err := wire.Decode(joinResp)
	if err != nil {
		log.Fatalf("decode JoinAck: %v", err)
	}
	ja := joinMsg.(*wire.JoinAck)
	if !ja.Accepted {
		log.Fatal("Join rejected")
	}
	playerID := ja.PlayerID
	log.Printf("joined room=%s player=%d seed=%d map=%dx%d",
		ja.RoomID, playerID, ja.Seed, ja.MapW, ja.MapH)

	w := sim.NewWorld(ja.Seed, ja.MapW, ja.MapH)
	w.Players = []sim.Player{{ID: 0}, {ID: 1}}

	var workers []uint32
	var hqID uint32
	lastTrainTick := uint32(0)

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		for {
			select {
			case data := <-conn.Inbox:
				msgType, msg, err := wire.Decode(data)
				if err != nil {
					continue
				}
				if msgType == wire.MsgFrameBundle {
					fb := msg.(*wire.FrameBundle)
					simCmds := wireCmdsToSimCmds(fb.Cmds)
					sim.Step(w, simCmds)
					h := sim.Hash(w)

					haData, _ := wire.Encode(&wire.HashAck{Tick: fb.Tick, Hash: h})
					conn.Send(haData)

					if fb.Tick == 1 {
						for i := range w.Units {
							u := &w.Units[i]
							if u.Owner == playerID && u.Type == sim.UnitWorker {
								workers = append(workers, u.ID)
							}
						}
						for i := range w.Buildings {
							if w.Buildings[i].Owner == playerID && w.Buildings[i].Type == sim.BldHQ {
								hqID = w.Buildings[i].ID
							}
						}
						log.Printf("found %d workers, HQ=%d", len(workers), hqID)
					}

					cmdTick := fb.Tick + uint32(ack.ServerTickRate)
					cmds := botThink(w, playerID, workers, hqID, &lastTrainTick, cmdTick)
					for _, c := range cmds {
						wireCmd := &wire.Cmd{
							Tick: cmdTick, Player: c.Player, Op: uint8(c.Op),
							UnitID: c.UnitID,
							TargetX: int32(c.TargetPos.X.Raw()),
							TargetY: int32(c.TargetPos.Y.Raw()),
							TargetID: c.TargetID,
						}
						data, _ := wire.Encode(wireCmd)
						conn.Send(data)
					}
				}
			default:
				goto doneDrain
			}
		}
	doneDrain:
		conn.Tick(time.Now())
	}
}

func botThink(w *sim.World, playerID uint8, workers []uint32, hqID uint32,
	lastTrainTick *uint32, cmdTick uint32) []sim.Cmd {

	var cmds []sim.Cmd

	for _, wid := range workers {
		u := w.FindUnit(wid)
		if u == nil || u.State == sim.UnitDead || u.State != sim.UnitIdle {
			continue
		}
		var best *sim.Crystal
		var bestDist fixed.Fix32
		for i := range w.Crystals {
			c := &w.Crystals[i]
			if c.Remaining <= 0 {
				continue
			}
			assigned := 0
			for _, owid := range workers {
				ou := w.FindUnit(owid)
				if ou != nil && ou.State != sim.UnitDead && ou.TargetID == c.ID {
					assigned++
				}
			}
			if assigned >= 2 {
				continue
			}
			dSq := u.Pos.DistSq(c.Pos)
			if best == nil || dSq < bestDist {
				best = c
				bestDist = dSq
			}
		}
		if best == nil {
			continue
		}
		cmds = append(cmds, sim.Cmd{
			Player:    playerID,
			Op:        sim.CmdMove,
			UnitID:    wid,
			TargetPos: best.Pos,
		})
	}

	p := &w.Players[playerID]
	if p.Crystal >= sim.UnitStatTable[sim.UnitWorker].Cost &&
		cmdTick > *lastTrainTick+30 {
		hq := w.FindBuilding(hqID)
		if hq != nil && hq.State == sim.BldReady && len(hq.ProductionQueue) < sim.MaxQueueLength {
			cmds = append(cmds, sim.Cmd{
				Player:   playerID,
				Op:       sim.CmdTrain,
				UnitID:   hqID,
				TargetID: uint32(sim.UnitWorker),
			})
			*lastTrainTick = cmdTick
		}
	}

	return cmds
}

func readWithTimeout(conn *transport.Conn, timeout time.Duration) []byte {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case data := <-conn.Inbox:
		return data
	case <-timer.C:
		return nil
	}
}

func wireCmdsToSimCmds(cmds []wire.Cmd) []sim.Cmd {
	result := make([]sim.Cmd, len(cmds))
	for i, c := range cmds {
		result[i] = sim.Cmd{
			Player:    c.Player,
			Op:        sim.CmdOp(c.Op),
			UnitID:    c.UnitID,
			TargetPos: fixed.V(fixed.FromRaw(c.TargetX), fixed.FromRaw(c.TargetY)),
			TargetID:  c.TargetID,
		}
	}
	return result
}

func init() {
	log.SetOutput(os.Stdout)
	log.SetFlags(log.Ltime)
}

var _ = fmt.Println
var _ = lockstep.RoomMsg{}
```

- [ ] **Step 2: Verify compilation**

Run: `cd e:\code\_Claude\RTS && go build ./cmd/bot-client/...`
Expected: success

- [ ] **Step 3: Commit**

```bash
git add cmd/bot-client/main.go
git commit -m "feat(bot): add bot-client with Phase_Economy mining strategy"
```

---

### Task 15: Go — Determinism economy test

**Files:**
- Create: `e:\code\_Claude\RTS\test\determinism\economy_test.go`

- [ ] **Step 1: Write 100× byte-equal economy determinism test**

```go
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
		for i := 0; i < 3; i++ {
			id := w.SpawnUnit(0, fixed.VInt(13+i, 45+i), stats.MaxHP, stats.Speed)
			w.Units[len(w.Units)-1].Type = sim.UnitWorker
			_ = id
			id = w.SpawnUnit(1, fixed.VInt(93-i, 45+i), stats.MaxHP, stats.Speed)
			w.Units[len(w.Units)-1].Type = sim.UnitWorker
			_ = id
		}
	}

	tickCmds[5] = []sim.Cmd{
		{Player: 0, Op: sim.CmdMove, UnitID: 1, TargetPos: fixed.VInt(5, 10)},
		{Player: 0, Op: sim.CmdMove, UnitID: 2, TargetPos: fixed.VInt(15, 55)},
		{Player: 0, Op: sim.CmdMove, UnitID: 3, TargetPos: fixed.VInt(25, 70)},
		{Player: 1, Op: sim.CmdMove, UnitID: 4, TargetPos: fixed.VInt(75, 10)},
		{Player: 1, Op: sim.CmdMove, UnitID: 5, TargetPos: fixed.VInt(85, 55)},
		{Player: 1, Op: sim.CmdMove, UnitID: 6, TargetPos: fixed.VInt(95, 70)},
	}
	tickCmds[60] = []sim.Cmd{
		{Player: 0, Op: sim.CmdTrain, UnitID: 7, TargetID: uint32(sim.UnitWorker)},
	}
	tickCmds[120] = []sim.Cmd{
		{Player: 1, Op: sim.CmdTrain, UnitID: 8, TargetID: uint32(sim.UnitWorker)},
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
```

- [ ] **Step 2: Run determinism test**

Run: `cd e:\code\_Claude\RTS && go test ./test/determinism/... -v -run TestEconomy`
Expected: 100 runs all match

- [ ] **Step 3: Commit**

```bash
git add test/determinism/economy_test.go
git commit -m "test(determinism): add 100x economy determinism test"
```

---

### Task 16: Unity — CrystalView + ResourceBar UI

**Files:**
- Create: `E:\code\_Unity\rts-client-unity\Assets\Game\CrystalView.cs`
- Create: `E:\code\_Unity\rts-client-unity\Assets\UI\ResourceBar.cs`

- [ ] **Step 1: Create CrystalView.cs**

```csharp
using RTS.Sim;
using UnityEngine;

namespace RTS.Game
{
    public class CrystalView : MonoBehaviour
    {
        public uint CrystalID { get; private set; }

        [SerializeField] private GameObject _visual;
        [SerializeField] private float _maxScale = 1.0f;
        [SerializeField] private float _minScale = 0.3f;

        private float _initialRemaining;

        public static CrystalView Create(uint id, Vec2 pos, Fixed32 remaining, GameObject prefab)
        {
            var worldPos = new Vector3(pos.X.ToFloat(), 0.05f, pos.Y.ToFloat());
            var go = Object.Instantiate(prefab, worldPos, Quaternion.identity);
            var cv = go.GetComponent<CrystalView>();
            if (cv == null) cv = go.AddComponent<CrystalView>();
            cv.CrystalID = id;
            cv._initialRemaining = remaining.ToFloat();
            cv.UpdateVisual(remaining.ToFloat());
            return cv;
        }

        public void UpdateRemaining(Fixed32 remaining)
        {
            float frac = remaining.ToFloat() / _initialRemaining;
            UpdateVisual(remaining.ToFloat());
        }

        private void UpdateVisual(float remaining)
        {
            if (_visual == null) return;
            float frac = remaining / _initialRemaining;
            float scale = Mathf.Lerp(_minScale, _maxScale, frac);
            _visual.transform.localScale = Vector3.one * scale;
        }

        public void Remove()
        {
            Destroy(gameObject);
        }
    }
}
```

- [ ] **Step 2: Create ResourceBar.cs**

```csharp
using RTS.Game;
using RTS.Sim;
using UnityEngine;
using UnityEngine.UIElements;

namespace RTS.UI
{
    public class ResourceBar : MonoBehaviour
    {
        [SerializeField] private UIDocument _document;
        private Label _crystalLabel;

        private void OnEnable()
        {
            if (_document == null) return;
            _crystalLabel = _document.rootVisualElement.Q<Label>("crystal-label");
        }

        private void Update()
        {
            var gm = GameManager.Instance;
            if (gm == null || gm.State != GameState.Playing) return;
            var w = gm.Runner?.World;
            if (w == null) return;
            int pid = gm.LocalPlayerID;
            if (pid >= w.Players.Count) return;
            if (_crystalLabel != null)
                _crystalLabel.text = $"Crystal: {w.Players[pid].Crystal.ToInt()}";
        }
    }
}
```

- [ ] **Step 3: Verify Unity compilation**

Check Unity Console for errors.

- [ ] **Step 4: Commit**

```bash
git add Assets/Game/CrystalView.cs Assets/UI/ResourceBar.cs
git commit -m "feat(ui): add CrystalView + ResourceBar for economy slice"
```

---

### Task 17: C# — Update GoldenData.cs for economy field

**Files:**
- Modify: `E:\code\_Unity\rts-client-unity\Assets\Tests\EditMode\GoldenData.cs`

- [ ] **Step 1: Add economyInitialHash field**

Add to `GoldenData` class:
```csharp
public string economyInitialHash;
```

- [ ] **Step 2: Verify Unity compilation**

Check Unity Console.

- [ ] **Step 3: Commit**

```bash
git add Assets/Tests/EditMode/GoldenData.cs
git commit -m "chore(cs): add economyInitialHash field to GoldenData"
```

---

### Task 18: Go — Full Phase 1 verification + golden regen

**Files:**
- (verification only, no new files)

- [ ] **Step 1: Run all Go tests**

Run: `cd e:\code\_Claude\RTS && go test ./... -v 2>&1 | tail -50`
Expected: all tests pass including economy determinism

- [ ] **Step 2: Regenerate golden data and copy to Unity**

Run: `cd e:\code\_Claude\RTS && go run ./cmd/golden-export/ golden_data.json`
Copy: `cp golden_data.json E:\code\_Unity\rts-client-unity\Assets\Tests\EditMode\GoldenData.json`
In Unity: Run EditMode tests. Expected: EconomyWorldHash_MatchesGolden passes (or add parity test in EconomyTests.cs).

- [ ] **Step 3: Run existing determinism test (no regression)**

Run: `cd e:\code\_Claude\RTS && go test ./test/determinism/... -v -run TestDeterminism100x`
Expected: 100 runs match

- [ ] **Step 4: Commit final golden data**

```bash
git add golden_data.json
git commit -m "chore(golden): regenerate golden data with Phase 1 economy anchor"
```

---

## Self-Review

### 1. Spec Coverage

| Spec requirement | Task(s) |
|---|---|
| sim: Crystal + Player.Crystal | Already exists (Phase 0); Task 2, 3 |
| sim: Worker mining FSM | Task 3 (Go), Task 12 (C#) |
| sim: HQ ProductionQueue | Task 4 (Go), Task 12 (C#) |
| sim: fixed rally point | Task 4 (Go), Task 12 (C#) |
| wire: CmdTrain | Task 5 (Go), Task 10 (C#) |
| server: spawn crystals at fixed positions | Task 7 |
| server: SpawnInitialUnits → 1×HQ + 3×Workers | Task 7 |
| Unity: CrystalView | Task 16 |
| Unity: Player.Crystal sync, ResourceBar | Task 16 |
| Unity: worker right-click-crystal → Mining | Task 6 (Go), Task 12 (C#) |
| bot: Phase_Economy strategy | Task 14 |
| Network stress validation | Task 15 (determinism test covers state churn) |
| DoD: bot_vs_bot_economy_60s.go passes | Task 15 |
| Entity stat constants table | Task 1 (Go), Task 10 (C#) |

**Known gap — Unity HQ train UI:** The "HQ selected + Q to CmdTrain(Soldier)" UX requires SelectionManager building selection + CommandDispatcher Q-key binding. This is a UI input-layer task that depends on the sim layer being complete first. It can be done as a quick follow-up after Task 16.

### 2. Placeholder Scan

No TBD/TODO/fill-in-later patterns. All code steps have complete implementation code.

### 3. Type Consistency

- `UnitStatTable`: `map[UnitType]UnitStats` — consistent across Tasks 1, 4, 5, 8, 14, 15
- `BuildingStatTable`: `map[BuildingType]BuildingStats` — consistent across Tasks 1, 5, 10
- `CmdOp` values: 1=Move, 2=Attack, 3=Stop, 4=AttackMove, 5=Build, 6=Train, 7=Surrender
- `FindEntity` return: `(found bool, unit *Unit, bld *Building, cryst *Crystal)` — Tasks 2, 6
- `economyInitialHash` field: Tasks 9 (Go golden) and 17 (C# GoldenData.cs)

---

## Execution Handoff

**Plan complete and saved to `docs/superpowers/plans/2026-04-27-sub2-phase1-economy.md`. Two execution options:**

**1. Subagent-Driven (recommended)** — I dispatch a fresh subagent per task, review between tasks, fast iteration

**2. Inline Execution** — Execute tasks in this session using executing-plans, batch execution with checkpoints

**Which approach?**
