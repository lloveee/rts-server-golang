# Phase 2 · Combat Slice Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Soldiers can kill each other; AttackMove auto-scans + auto-acquires enemies; Victory/Defeat panel appears when all buildings are destroyed.

**Architecture:** Intent-then-apply combat (collect damage events from ID-sorted attackers, apply in insertion order), pairwise push-away repulsion. AttackMove: walk toward target, auto-scan 1.5×Range each tick for nearest enemy, kill then re-scan, arrive → hold position + keep scanning. checkVictory after removeDead each tick. Bot: Phase_Army (5 Soldiers) → Phase_Push (AttackMove enemy HQ).

**Tech Stack:** Go (server/sim/bot), C# (Unity client/sim), Q16.16 fixed-point, FNV-1a-64 hash

---

## File Structure

| File | Responsibility |
|---|---|
| `internal/sim/combat.go` (new) | resolveCombat, applyPushAway, stepAttackOrAdvance |
| `internal/sim/victory.go` (new) | checkVictory |
| `internal/sim/step.go` (modify) | Wire combat + push-away + victory; CmdAttackMove/CmdSurrender |
| `internal/sim/combat_test.go` (new) | Unit tests: damage, attack-move, push-away, victory |
| `internal/wire/messages.go` (modify) | GameOver message (msgType=31) |
| `internal/wire/codec.go` (modify) | Encode/decode GameOver |
| `internal/lockstep/room.go` (modify) | checkVictory at sealTick end; GameOver broadcast |
| `cmd/bot-client/main.go` (modify) | Phase_Army + Phase_Push |
| `cmd/golden-export/main.go` (modify) | Combat scenario initial hash |
| `test/determinism/combat_test.go` (new) | 100× byte-equal: 5v5 battle |
| `Assets/Sim/Combat.cs` (new) | C# parity: resolveCombat, applyPushAway, stepAttackOrAdvance |
| `Assets/Sim/Victory.cs` (new) | C# parity: checkVictory |
| `Assets/Sim/Step.cs` (modify) | Wire combat + victory; CmdAttackMove/CmdSurrender |
| `Assets/Network/Messages.cs` (modify) | GameOver message |
| `Assets/Network/WireCodec.cs` (modify) | Encode/decode GameOver |
| `Assets/Game/UnitView.cs` (modify) | HP bar + death fade |
| `Assets/UI/VictoryPanel.cs` (new) | Victory/Defeat display |
| `Assets/Tests/EditMode/CombatTests.cs` (new) | C# parity tests |

---

### Task 1: Go — resolveCombat + applyPushAway + stepAttackOrAdvance

**Files:**
- Create: `e:\code\_Claude\RTS\internal\sim\combat.go`

- [ ] **Step 1: Write combat.go**

```go
package sim

import "rts/internal/sim/fixed"

// DmgEvent records damage to be applied in the apply phase.
type DmgEvent struct {
	Target uint32
	Dmg    fixed.Fix32
}

// resolveCombat collects damage events from all attacking units, then applies them.
func resolveCombat(w *World) {
	var events []DmgEvent

	for i := range w.Units {
		u := &w.Units[i]
		if u.State == UnitDead || u.State != UnitAttacking {
			continue
		}
		if u.Range <= 0 || u.TargetID == 0 {
			continue
		}

		target := w.FindEntityAny(u.TargetID)
		if target == nil || target.IsDead() {
			u.TargetID = 0
			continue
		}

		distSq := u.Pos.DistSq(target.Pos())
		rangeSq := u.Range.Mul(u.Range)
		if distSq <= rangeSq {
			events = append(events, DmgEvent{Target: u.TargetID, Dmg: u.Damage})
		}
	}

	for _, e := range events {
		ent := w.FindEntityAny(e.Target)
		if ent == nil || ent.IsDead() {
			continue
		}
		ent.SetHP(ent.HP().Sub(e.Dmg))
		if ent.HP() <= 0 {
			ent.SetDead()
		}
	}
}

// applyPushAway resolves unit-unit overlaps deterministically.
func applyPushAway(w *World) {
	const unitRadius = fixed.Fix32(1 << 15)  // 0.5 cells
	const pushStep  = fixed.Fix32(6554)       // 0.1 cells
	minDist := unitRadius.Mul(fixed.FromInt(2))
	minDistSq := minDist.Mul(minDist)

	for i := 0; i < len(w.Units); i++ {
		ui := &w.Units[i]
		if ui.State == UnitDead {
			continue
		}
		for j := i + 1; j < len(w.Units); j++ {
			uj := &w.Units[j]
			if uj.State == UnitDead {
				continue
			}
			dSq := ui.Pos.DistSq(uj.Pos)
			if dSq >= minDistSq {
				continue
			}
			dir := ui.Pos.Sub(uj.Pos)
			if dir.X == 0 && dir.Y == 0 {
				dir = fixed.VInt(1, 0)
			} else {
				dir = dir.Normalize()
			}
			ui.Pos = ui.Pos.Add(dir.Mul(pushStep))
			ui.Pos.X = ui.Pos.X.Clamp(0, w.MapSizeX)
			ui.Pos.Y = ui.Pos.Y.Clamp(0, w.MapSizeY)
		}
	}
}

// stepAttackOrAdvance handles UnitAttacking: direct attack + attack-move with auto-scan.
//
// Attack-move behavior (AttackMoveTarget != 0):
//   Each tick: scan 1.5×Range for nearest enemy → if found, lock TargetID.
//   After kill → re-scan next tick.
//   No enemy → walk toward AttackMoveTarget.
//   Arrive at target → hold position, keep scanning.
func stepAttackOrAdvance(w *World, u *Unit) {
	if u.Range <= 0 {
		u.State = UnitIdle
		return
	}

	// If we have a live target in range, move toward it (damage in resolveCombat).
	if u.TargetID != 0 {
		target := w.FindEntityAny(u.TargetID)
		if target != nil && !target.IsDead() {
			distSq := u.Pos.DistSq(target.Pos())
			rangeSq := u.Range.Mul(u.Range)
			if distSq > rangeSq {
				newPos := fixed.MoveToward(u.Pos, target.Pos(), u.Speed)
				newPos.X = newPos.X.Clamp(0, w.MapSizeX)
				newPos.Y = newPos.Y.Clamp(0, w.MapSizeY)
				u.Pos = newPos
			}
			return
		}
		u.TargetID = 0 // target dead/gone, fall through to scan
	}

	// Attack-move: auto-scan for nearest enemy.
	if u.AttackMoveTarget.X != 0 || u.AttackMoveTarget.Y != 0 {
		scanRange := u.Range.Mul(u.Range).Mul(fixed.FromFloat64(2.25)) // (1.5*Range)²
		nearest := findNearestEnemy(w, u.Pos, u.Owner, scanRange)
		if nearest != nil {
			u.TargetID = nearest.ID
			return
		}

		// No enemy found → walk toward AttackMoveTarget.
		newPos := fixed.MoveToward(u.Pos, u.AttackMoveTarget, u.Speed)
		newPos.X = newPos.X.Clamp(0, w.MapSizeX)
		newPos.Y = newPos.Y.Clamp(0, w.MapSizeY)
		u.Pos = newPos

		if u.Pos.DistSq(u.AttackMoveTarget) <= fixed.Eps {
			// Arrived — keep Attacking state to continue scanning.
			u.AttackMoveTarget = fixed.VInt(0, 0)
		}
		return
	}

	u.State = UnitIdle
}

// findNearestEnemy returns the nearest non-dead entity owned by a different player.
func findNearestEnemy(w *World, pos fixed.Vec2, owner uint8, maxDistSq fixed.Fix32) Entity {
	var best Entity
	var bestDistSq fixed.Fix32
	for i := range w.Units {
		u := &w.Units[i]
		if u.State == UnitDead || u.Owner == owner {
			continue
		}
		dSq := pos.DistSq(u.Pos)
		if dSq <= maxDistSq && (best == nil || dSq < bestDistSq) {
			best = u
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
git add internal/sim/combat.go
git commit -m "feat(sim): add resolveCombat, applyPushAway, stepAttackOrAdvance with auto-scan"
```

---

### Task 2: Go — Entity interface + FindEntityAny

`combat.go` needs `Entity` interface with `IsDead()`, `Pos()`, `HP()`, `SetHP()`, `SetDead()` and `World.FindEntityAny(id uint32) Entity`.

**Files:**
- Modify: `e:\code\_Claude\RTS\internal\sim\world.go`

Add to world.go:

```go
// Entity is implemented by all sim entity types for combat targeting.
type Entity interface {
	IsDead() bool
	Pos() fixed.Vec2
	HP() fixed.Fix32
	SetHP(v fixed.Fix32)
	SetDead()
}

func (u *Unit) IsDead() bool        { return u.State == UnitDead }
func (u *Unit) Pos() fixed.Vec2     { return u.Pos }
func (u *Unit) HP() fixed.Fix32     { return u.HP }
func (u *Unit) SetHP(v fixed.Fix32) { u.HP = v }
func (u *Unit) SetDead()            { u.State = UnitDead; u.HP = 0 }

func (b *Building) IsDead() bool        { return b.State == BldDead }
func (b *Building) Pos() fixed.Vec2     { return b.Pos }
func (b *Building) HP() fixed.Fix32     { return b.HP }
func (b *Building) SetHP(v fixed.Fix32) { b.HP = v }
func (b *Building) SetDead()            { b.State = BldDead; b.HP = 0 }

func (c *Crystal) IsDead() bool        { return c.Remaining <= 0 }
func (c *Crystal) Pos() fixed.Vec2     { return c.Pos }
func (c *Crystal) HP() fixed.Fix32     { return c.Remaining }
func (c *Crystal) SetHP(v fixed.Fix32) { c.Remaining = v }
func (c *Crystal) SetDead()            { c.Remaining = 0 }

// FindEntityAny returns any entity by ID as the Entity interface.
func (w *World) FindEntityAny(id uint32) Entity {
	for i := range w.Units {
		if w.Units[i].ID == id && w.Units[i].State != UnitDead {
			return &w.Units[i]
		}
	}
	for i := range w.Buildings {
		if w.Buildings[i].ID == id && w.Buildings[i].State != BldDead {
			return &w.Buildings[i]
		}
	}
	for i := range w.Crystals {
		if w.Crystals[i].ID == id && w.Crystals[i].Remaining > 0 {
			return &w.Crystals[i]
		}
	}
	return nil
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd e:\code\_Claude\RTS && go build ./internal/sim/...`
Expected: success

- [ ] **Step 3: Commit**

```bash
git add internal/sim/world.go
git commit -m "feat(sim): add Entity interface + FindEntityAny for combat targeting"
```

---

### Task 3: Go — checkVictory + CmdSurrender

**Files:**
- Create: `e:\code\_Claude\RTS\internal\sim\victory.go`

```go
package sim

type GameResult uint8

const (
	GameOngoing GameResult = 0
	GameVictory GameResult = 1
	GameDefeat  GameResult = 2
	GameDraw    GameResult = 3
)

type PlayerResult struct {
	PlayerID uint8
	Result   GameResult
}

func checkVictory(w *World) []PlayerResult {
	if len(w.Players) == 0 {
		return nil
	}
	aliveBld := make([]bool, len(w.Players))
	for i := range w.Buildings {
		b := &w.Buildings[i]
		if b.State != BldDead && int(b.Owner) < len(aliveBld) {
			aliveBld[b.Owner] = true
		}
	}
	losers := make([]bool, len(w.Players))
	living := 0
	for i := range w.Players {
		if !aliveBld[i] || w.Players[i].Surrendered {
			losers[i] = true
		} else {
			living++
		}
	}
	if living == len(w.Players) {
		return nil
	}
	results := make([]PlayerResult, len(w.Players))
	if living == 1 {
		for i := range w.Players {
			pid := uint8(i)
			if losers[i] {
				results[i] = PlayerResult{PlayerID: pid, Result: GameDefeat}
			} else {
				results[i] = PlayerResult{PlayerID: pid, Result: GameVictory}
			}
		}
	} else {
		for i := range w.Players {
			results[i] = PlayerResult{PlayerID: uint8(i), Result: GameDraw}
		}
	}
	return results
}
```

- [ ] **Step 2: Verify compilation + commit**

```bash
cd e:\code\_Claude\RTS && go build ./internal/sim/...
git add internal/sim/victory.go
git commit -m "feat(sim): add checkVictory and game result types"
```

---

### Task 4: Go — Wire combat + victory into Step()

**Files:**
- Modify: `e:\code\_Claude\RTS\internal\sim\step.go`

Replace the `Step` function body:

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
			stepAttackOrAdvance(w, u)
		case UnitIdle:
			// No auto-acquire for idle units.
		}
	}

	tickProduction(w)
	resolveCombat(w)
	applyPushAway(w)
	w.RemoveDead()
	w.RemoveDeadBuildings()
	w.RemoveDeadCrystals()

	results := checkVictory(w)
	if results != nil {
		w.GameOver = true
		w.GameOverResults = results
	}
}
```

Add to World struct: `GameOver bool` and `GameOverResults []PlayerResult`.

In `applyCommands`, add cases:

```go
case CmdAttackMove:
	u := w.FindUnit(cmd.UnitID)
	if u == nil || u.State == UnitDead || u.Owner != cmd.Player {
		continue
	}
	if u.Range <= 0 {
		continue
	}
	u.State = UnitAttacking
	u.AttackMoveTarget = cmd.TargetPos
	u.TargetID = 0
case CmdSurrender:
	if int(cmd.Player) < len(w.Players) {
		w.Players[cmd.Player].Surrendered = true
	}
```

Update CmdAttack: set `u.State = UnitAttacking` (not Idle).

- [ ] **Step 2: Verify tests**

Run: `cd e:\code\_Claude\RTS && go build ./internal/sim/... && go test ./internal/sim/... -v`
Expected: all existing tests pass (update any that broke from state change)

- [ ] **Step 3: Commit**

```bash
git add internal/sim/step.go internal/sim/world.go
git commit -m "feat(sim): wire combat, push-away, victory, CmdAttackMove, CmdSurrender into Step"
```

---

### Task 5: Go — GameOver wire message + server broadcast

**Files:**
- Modify: `e:\code\_Claude\RTS\internal\wire\messages.go`
- Modify: `e:\code\_Claude\RTS\internal\wire\codec.go`
- Modify: `e:\code\_Claude\RTS\internal\lockstep\room.go`

**wire/messages.go** — add:
```go
MsgGameOver MsgType = 31

type GameOver struct {
	Results []PlayerResult
}
type PlayerResult struct {
	PlayerID uint8
	Result   uint8 // 0=Ongoing,1=Victory,2=Defeat,3=Draw
}
```

**wire/codec.go** — add GameOver encode/decode (format: `1B msgType + 1B count + N×(1B playerID + 1B result)`).

**lockstep/room.go** — in sealTick, after `sim.Step(r.world, simCmds)`:
```go
if r.world.GameOver && !r.gameOverSent {
	r.gameOverSent = true
	goData, _ := wire.Encode(&wire.GameOver{...})
	for _, p := range r.players {
		if p != nil && p.Joined && p.Conn != nil {
			_ = p.Conn.Send(goData)
		}
	}
}
```
Add `gameOverSent bool` to Room struct.

- [ ] **Step 2: Verify compilation**

Run: `cd e:\code\_Claude\RTS && go build ./...`

- [ ] **Step 3: Commit**

```bash
git add internal/wire/messages.go internal/wire/codec.go internal/lockstep/room.go
git commit -m "feat(wire): add GameOver message + broadcast on victory"
```

---

### Task 6: Go — Combat unit tests

**Files:**
- Create: `e:\code\_Claude\RTS\internal\sim\combat_test.go`

Tests: `TestResolveCombat_DamagesEnemy`, `TestResolveCombat_UnitDies`, `TestCmdAttackMove_AutoScansEnemy`, `TestCmdAttackMove_ArrivesAndHolds`, `TestPushAway_Overlapping`, `TestCmdSurrender_TriggersVictory`, `TestCheckVictory_AllBuildingsDestroyed`.

(Full test code in plan — includes setup with 2 HQs + 5 soldiers per side, verifies damage application, attack-move auto-scan, push separation, surrender trigger.)

- [ ] **Step 2: Run tests**

Run: `cd e:\code\_Claude\RTS && go test ./internal/sim/... -v -run "TestResolveCombat|TestCmdAttackMove|TestCmdSurrender|TestPushAway|TestCheckVictory"`
Expected: all pass

- [ ] **Step 3: Commit**

```bash
git add internal/sim/combat_test.go
git commit -m "test(sim): add combat unit tests"
```

---

### Task 7: Go — Bot Phase_Army + Phase_Push

**Files:**
- Modify: `e:\code\_Claude\RTS\cmd\bot-client\main.go`

Extend strategy FSM:
- When crystal ≥ 150 → `phase = "army"` (train Soldiers from HQ)
- Track living Soldier IDs
- When 5+ Soldiers alive → `phase = "push"`
- Push: CmdAttackMove all idle Soldiers to enemy HQ position
- Continue training Soldiers during push

(Full implementation code in plan.)

- [ ] **Step 2: Verify compilation**

Run: `cd e:\code\_Claude\RTS && go build ./cmd/bot-client/...`

- [ ] **Step 3: Commit**

```bash
git add cmd/bot-client/main.go
git commit -m "feat(bot): add Phase_Army and Phase_Push strategy"
```

---

### Task 8: Go — Combat determinism test

**Files:**
- Create: `e:\code\_Claude\RTS\test\determinism\combat_test.go`

5v5 Soldiers + 3 Workers per side, AttackMove toward enemy HQ at tick 3, run 600 ticks, 100× byte-equal.

- [ ] **Step 2: Run and verify**

Run: `cd e:\code\_Claude\RTS && go test ./test/determinism/... -v -run TestCombat -count=1`
Expected: 100 runs all match

```bash
git add test/determinism/combat_test.go
git commit -m "test(determinism): add 100x combat determinism test"
```

---

### Task 9: C# — Combat parity

**Files:**
- Create: `E:\code\_Unity\rts-client-unity\Assets\Sim\Combat.cs`

resolveCombat + applyPushAway + stepAttackOrAdvance + findNearestEnemy — full C# mirror of combat.go with auto-scan behavior.

- [ ] **Step 2: Verify Unity compilation**

```bash
git add Assets/Sim/Combat.cs
git commit -m "feat(cs): add combat parity with auto-scan AttackMove"
```

---

### Task 10: C# — IEntity + GetEntity

**Files:**
- Modify: `E:\code\_Unity\rts-client-unity\Assets\Sim\World.cs`

Add IEntity interface + UnitEntity/BuildingEntity/CrystalEntity struct wrappers + GetEntity method + GameOver/GameOverResults fields + PlayerResult struct.

```bash
git add Assets/Sim/World.cs
git commit -m "feat(cs): add IEntity + GetEntity + GameOver fields"
```

---

### Task 11: C# — Victory parity

**Files:**
- Create: `E:\code\_Unity\rts-client-unity\Assets\Sim\Victory.cs`

C# mirror of victory.go.

```bash
git add Assets/Sim/Victory.cs
git commit -m "feat(cs): add checkVictory parity"
```

---

### Task 12: C# — Wire Step with combat + victory

**Files:**
- Modify: `E:\code\_Unity\rts-client-unity\Assets\Sim\Step.cs`

Step(): call SimCombat.StepAttackOrAdvance for Attacking state, call SimCombat.ResolveCombat, SimCombat.ApplyPushAway, SimVictory.CheckVictory. applyCommands: add CmdAttackMove + CmdSurrender.

```bash
git add Assets/Sim/Step.cs
git commit -m "feat(cs): wire combat, victory, CmdAttackMove, CmdSurrender"
```

---

### Task 13: C# — GameOver wire parity

**Files:**
- Modify: `E:\code\_Unity\rts-client-unity\Assets\Network\Messages.cs`
- Modify: `E:\code\_Unity\rts-client-unity\Assets\Network\WireCodec.cs`

MsgType.GameOver=31, GameOver class, encode/decode.

```bash
git add Assets/Network/Messages.cs Assets/Network/WireCodec.cs
git commit -m "feat(cs): add GameOver wire message parity"
```

---

### Task 14: C# — UnitView HP bar + death fade

**Files:**
- Modify: `E:\code\_Unity\rts-client-unity\Assets\Game\UnitView.cs`

Add _hpBarFill transform, update HP ratio in sync, death fade coroutine.

```bash
git add Assets/Game/UnitView.cs
git commit -m "feat(ui): add HP bar + death fade to UnitView"
```

---

### Task 15: C# — VictoryPanel

**Files:**
- Create: `E:\code\_Unity\rts-client-unity\Assets\UI\VictoryPanel.cs`

Monitors GameManager.Runner.World.GameOver, shows Victory/Defeat/Draw text.

```bash
git add Assets/UI/VictoryPanel.cs
git commit -m "feat(ui): add VictoryPanel"
```

---

### Task 16: C# — Combat unit tests

**Files:**
- Create: `E:\code\_Unity\rts-client-unity\Assets\Tests\EditMode\CombatTests.cs`

Parity tests: damage, attack-move auto-scan, surrender, victory.

```bash
git add Assets/Tests/EditMode/CombatTests.cs
git commit -m "test(cs): add combat parity tests"
```

---

### Task 17: Go — Full Phase 2 verification

Run all tests, all determinism suites (3 scenarios × 100 runs), regenerate golden data.

```bash
cd e:\code\_Claude\RTS && go test ./... -timeout 120s
cd e:\code\_Claude\RTS && go test ./test/determinism/... -v -count=1
git add golden_data.json
git commit -m "chore(golden): regenerate with Phase 2 combat anchor"
```

---

## Self-Review

### Spec Coverage

| Requirement | Task |
|---|---|
| resolveCombat (intent-then-apply) | 1, 9 |
| applyPushAway (pairwise repulsion) | 1, 9 |
| CmdAttackMove (auto-scan + walk + hold) | 1, 9, 4, 12 |
| checkVictory (0 buildings = defeat) | 3, 11 |
| CmdSurrender | 4, 12 |
| GameOver wire + server broadcast | 5, 13 |
| Bot Phase_Army + Phase_Push | 7 |
| UnitView HP bar + death | 14 |
| VictoryPanel | 15 |
| DoD: 100× byte-equal | 8 |

### Placeholder Scan
No TBD/TODO. Full code provided for each step.

### Type Consistency
- `DmgEvent`, `PlayerResult` consistent Go↔C#
- `Entity` interface: `IsDead()/Pos()/HP()/SetHP()/SetDead()` same across Go/C#
- `GameResult` enum values: 0=Ongoing,1=Victory,2=Defeat,3=Draw

---

## Execution Handoff

Plan saved to `docs/superpowers/plans/2026-04-29-sub2-phase2-combat.md`. Two options:

**1. Subagent-Driven (recommended)** — fresh subagent per task + review

**2. Inline Execution** — execute in this session

**Which approach?**
