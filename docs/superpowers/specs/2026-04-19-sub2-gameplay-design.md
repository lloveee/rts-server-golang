# Sub-2 Gameplay Layer · Design Spec

**Date:** 2026-04-19
**Status:** Approved for planning
**Scope:** Build a complete classic RTS game loop (economy → production → combat → victory) on top of Sub-1's validated deterministic lockstep + reliable UDP foundation, in order to validate and iterate the "production-grade RTS network architecture" under realistic gameplay load.
**Est. effort:** ~7 weeks full-time

---

## 0. Context & Positioning

Sub-1 (completed 2026-04-19) proved the network layer — deterministic sim (Q16.16 fixed-point, sorted iteration, canonical hash), reliable UDP with SYN/ACK handshake, wire protocol, MonoBehaviour scaffolding, hash reconciliation — all interop-verified bit-identical between Unity client and Go server.

Sub-2's core purpose, as refined during brainstorming: **build a complete RTS game to validate and iterate the production-grade network architecture**. The game itself is a first-class goal, not just a test vehicle. Each vertical slice produces a new traffic pattern and state-churn pattern for the network layer to be stressed against.

Explicitly out of scope for Sub-2 (pushed to Sub-3+):
- Pathfinding better than A* + push-away ("SC1-level")
- Second resource type, tech tree, advanced unit roster
- High-ground vision, stealth, terrain types
- AI opponent (Sub-2 ships a scripted bot only)
- Audio, advanced animations, cinematic polish
- Chat, spectator, save-game

---

## 1. Locked-In Scope Decisions

| # | Axis | Decision | Alternatives considered |
|---|---|---|---|
| 1 | Goal | Complete game + network validation | Pure network stress test |
| 2 | Template | **T2** — 4 units, 4 buildings, 1 resource, worker-built construction | T1 (base only), T3 (tech tree) |
| 3 | Pathfinding | **P1** — A\* on 1×1 grid + unit push-away (SC1-level) | P2 (flow field + ORCA), P3 (direct-line only) |
| 4 | Fog of War | **F2** — classic two-layer (unseen/explored/visible) + building memory, client-rendering only | F1 (none), F3 (tactical with high-ground/stealth) |
| 5 | Test harness | **M2** — manual Unity-vs-Unity + scripted Go bot for CI determinism/chaos | M1 (manual only), M3 (+reactive AI) |
| 6 | Delivery | **A** — Vertical slices (each slice is end-to-end playable) | B (horizontal layers), C (risk-first) |

---

## 2. Gameplay Rules

### 2.1 Match flow

```
JoinRoom (2 players) → server auto SpawnInitial → game starts immediately (no Ready ceremony)
  → both sides mine/produce/fight
  → when one side loses ALL buildings → GameOver broadcast
  → clients display Victory/Defeat panel
```

- **No ready ceremony**: Nth joined player's JoinAck triggers first tick (same as Sub-1)
- **Victory**: Destroy all of opponent's buildings (any of the 4 types)
- **Surrender**: `CmdSurrender` — server treats as "all this player's buildings HP = 0"
- **Draw**: Mutual annihilation on same tick → "double defeat" result
- **Tiebreak on same-tick mutual annihilation**: the side whose last building reaches HP=0 first in iteration order (sorted by building ID) loses; if truly simultaneous → draw
- **No pause / no in-match timer / no max duration**
- **Reconnect**: inherits Sub-1's Resume flow (limbo + snapshot fallback)
- **Fog of war semantics**: see §5

### 2.2 Entity spec (placeholder values, tunable in constants file)

| Entity | HP | Speed | Range | Dmg | Vision | Cost | Build/Train | Size |
|---|---|---|---|---|---|---|---|---|
| Worker    | 20  | 0.8  | 0   | 0   | 6  | 50   | 10 ticks | 1×1 |
| Soldier   | 60  | 0.5  | 1.5 | 1.0 | 8  | 80   | 15 ticks | 1×1 |
| Archer    | 40  | 0.5  | 6   | 0.6 | 12 | 120  | 20 ticks | 1×1 |
| Cavalry   | 100 | 1.2  | 1.5 | 1.5 | 10 | 200  | 30 ticks | 1×1 |
| HQ        | 800 | 0    | 0   | 0   | 12 | —    | —        | 4×4 |
| Barracks  | 400 | 0    | 0   | 0   | 8  | 150  | 60 ticks | 3×3 |
| Archery   | 400 | 0    | 0   | 0   | 10 | 150  | 60 ticks | 3×3 |
| Stable    | 400 | 0    | 0   | 0   | 8  | 150  | 60 ticks | 3×3 |

All values Q16.16 fixed-point constants. Workers do not attack (Range=0).

### 2.3 Resource

- Single resource: **Crystal**, ~8 piles scattered at fixed per-player spawn positions
- Each pile starts at 1500; worker carries 5 per trip; trip takes ~30 ticks at pile + round-trip travel
- Crystal belongs to the **Player** (shared across a player's HQs; Sub-2 only has 1 HQ anyway)
- UI: top-right corner `💎 1250` counter

### 2.4 Command set (wire layer)

Existing (Sub-1): `CmdMove(1)`, `CmdAttack(2)`, `CmdStop(3)`.

New in Sub-2:
- `CmdAttackMove(4)` — attack-move to ground point
- `CmdBuild(5)` — worker places a building (type + ground cell)
- `CmdTrain(6)` — building queues a unit (type; server validates building-unit type match)
- `CmdSurrender(7)` — player concedes

`CmdPatrol` and `CmdHold` are explicitly **not** implemented (YAGNI).

---

## 3. Sim Data Model

### 3.1 Structure — 3 separate lists + shared ID namespace

Sub-1's `World.Units []Unit` cannot host buildings/crystals because `Unit`'s movement fields (Speed/MoveTo/TargetID) are meaningless for those. Instead:

```go
type World struct {
    Tick      uint32
    Seed      uint64
    Rand      *SplitMix64
    MapSizeX  fixed.Fix32
    MapSizeY  fixed.Fix32
    NextID    uint32             // shared ID pool across all entities

    Units     []Unit              // Worker/Soldier/Archer/Cavalry
    Buildings []Building          // HQ/Barracks/Archery/Stable
    Crystals  []Crystal           // resource piles
    Players   []Player            // crystal pool + surrender state

    NavGrid   *NavGrid            // A* collision bitmap, updated on building add/remove
}
```

- Commands carry a single `TargetID uint32` — `World.FindEntity(id)` scans all three lists (O(N+B+C); <1000 total entities → negligible)
- Sub-1's sorted-iteration rule extends: `sortUnitsByID`, `sortBuildingsByID`, `sortCrystalsByID` each tick
- **Not an ECS**: keeps Sub-1's flat-array style

### 3.2 Struct definitions (delta from Sub-1)

```go
type UnitType uint8
const (UnitWorker=1; UnitSoldier=2; UnitArcher=3; UnitCavalry=4)

type UnitState uint8
const (StIdle=0; StMoving=1; StAttacking=2; StMining=3; StReturning=4; StBuilding=5; StDead=6)

type Unit struct {
    ID, Owner          uint32, uint8
    Type               UnitType
    Pos, MoveTo        Vec2
    HP, MaxHP          Fix32
    Speed              Fix32
    Range, Damage      Fix32             // 0 for Worker
    VisionRange        Fix32             // client FOW only; server does not read during sim
    State              UnitState
    TargetID           uint32            // semantic depends on State
    CarryAmount        Fix32             // Worker only
    Path               []Vec2            // A* result, consumed segment by segment
    AttackMoveTarget   Vec2              // StAttacking with TargetID=0: keep walking here
}

type BuildingType uint8
const (BldHQ=1; BldBarracks=2; BldArchery=3; BldStable=4)

type BuildingState uint8
const (BldConstructing=0; BldReady=1; BldDead=2)

type Building struct {
    ID, Owner          uint32, uint8
    Type               BuildingType
    Pos                Vec2             // AABB lower-left
    SizeCells          uint8            // 3 or 4
    HP, MaxHP          Fix32
    State              BuildingState
    ConstructProgress  Fix32            // 0..1, incremented per tick while StConstructing
    ProductionQueue    []QueueItem      // FIFO
    RallyPoint         Vec2             // trained units auto-move here
}

type QueueItem struct {
    UnitType    UnitType
    TicksLeft   uint32
    StartTick   uint32  // for UI progress bar
}

type Crystal struct {
    ID        uint32
    Pos       Vec2
    Remaining Fix32
}

type Player struct {
    ID          uint8
    Crystal     Fix32
    Surrendered bool
}
```

### 3.3 Command dispatch

```
CmdMove(unit, pos)        → Path = A*(pos); State = StMoving
CmdAttack(unit, target)   → TargetID = target; State = StAttacking (ignored if Range=0)
CmdAttackMove(unit, pos)  → AttackMoveTarget = pos; Path = A*(pos); State = StAttacking, TargetID = 0
CmdStop(unit)             → State = StIdle; clear Path/TargetID
CmdBuild(worker, bldT, pos):
  1. Validate: player resource sufficient; cells AABB-free; within map
  2. Deduct resource; create Building{State=StConstructing, HP=1, ConstructProgress=0}
  3. worker.State=StBuilding; TargetID=newBuildingID; Path=A*(→building.Pos)
CmdTrain(building, unitT):
  1. Validate: unit type matches building; queue length < 5; resource sufficient
  2. Deduct resource; append QueueItem
CmdSurrender(player)      → Player.Surrendered = true (checkVictory reads it)
```

Invalid commands are **silently dropped by sim** (no error path). Clients do UI-level validation to prevent them; if a malicious client sends an invalid command, sim's idempotent rejection keeps everyone in sync.

### 3.4 Tick sequence (expansion of Sub-1's `sim.Step`)

```go
func Step(w *World, cmds []Cmd) {
    w.Tick++
    applyCommands(w, cmds)
    sortAllByID(w)                    // Unit, Building, Crystal

    for each unit (sorted by ID):
        switch State:
            Moving    → stepMovePath
            Mining    → stepMining
            Returning → stepReturning
            Building  → stepBuilding
            Attacking → stepAttackOrAdvance
            Idle      → nothing (no auto-acquire — YAGNI)

    for each building (sorted by ID):
        if Constructing → tick ConstructProgress; complete → StReady, HP=MaxHP
        if Ready        → tick queue head; expire → spawn unit at edge cell

    resolveCombat(w)          // intent-then-apply, see §4.2
    applyPushAway(w)          // pairwise repulsion, see §4.1
    removeDead(w)              // prune HP<=0 Units and Buildings
    checkVictory(w)            // if any player has 0 buildings or Surrendered → set GameOver
}
```

### 3.5 FOW lives entirely client-side

- Server `FrameBundle` broadcasts all commands unmodified (same as Sub-1)
- All clients run identical sim with identical state
- FOW is a per-client render filter (see §5)
- **Sim never consults FOW**. Any command (even "attack invisible enemy") is processed identically by all sims. Client UI is responsible for preventing such commands at input time.

### 3.6 Determinism rules (Sub-1's 6 + 2 new)

Sub-1 rules unchanged: no float, no map iteration, no time.Now/rand, no goroutine-concurrent sim state, no reflection, determinism suite per Go version.

**New in Sub-2:**
7. **A\* tiebreak must be stable**: nodes with equal f-score are popped in (y, x) lex order; never rely on Go map iteration
8. **Push-away direction tiebreak must be stable**: overlapping units resolved by lower-ID-moves rule; move direction is (smaller.Pos - bigger.Pos).Normalize() — no randomness

---

## 4. Hard Subsystems

### 4.1 Pathfinding (P1: A\* + push-away)

**NavGrid** — static data, sim state:

```go
type NavGrid struct {
    W, H     int32          // 1:1 with MapSize, 100×100 cells
    Blocked  []uint64       // bitmap; Blocked[y*W+x] bit x%64
}
```

- Building placement/destruction calls `grid.UpdateBuilding(b)` to toggle the AABB cells
- **Units do not enter the blocked bitmap** — unit-unit collision is handled by push-away (avoids high-frequency bitmap churn)

**A\* algorithm:**
- 8-connectivity; diagonal cost ≈ 1.41 as Q16.16 constant
- Heuristic: octile distance, fixed-point
- Open list: min-heap keyed on (f, x, y); equal f tie-broken by (y, x) lex order
- Closed set: temporary bool matrix indexed by cell coords (no map)
- Unit.Path stored as `[]Vec2` world-coord waypoints (pre-converted)

**Re-planning triggers:**
- New move command → plan once
- Path blocked (next cell now occupied by a new building) → re-plan
- Unit death → path discarded
- **Not triggered by**: enemy unit movement (units not in bitmap)

**CPU budget:** 300 units × one re-plan per ~5 seconds ≈ 3 A\* calls per tick on average; far under the 1ms/tick budget.

**Push-away (applyPushAway), once per tick at end:**
```
for each pair (i,j) of units sorted by ID:
    if |u_i.Pos - u_j.Pos| < 2*unitRadius:
        smaller = min-ID unit
        dir = (smaller.Pos - other.Pos).Normalize()
        smaller.Pos += dir * kPushStep  // kPushStep = 0.1
```
- O(N²) for N≈300 → 90K distance checks × 20Hz → ~2M ops/sec, <1% tick budget
- Future optimization: spatial hash bucketing (Sub-3+)
- Tie-breaks entirely deterministic

### 4.2 Combat resolution — intent-then-apply

```go
type DmgEvent struct { Target uint32; Dmg Fix32 }

func resolveCombat(w *World) {
    events := []DmgEvent{}

    for each unit u sorted by ID:
        if u.State != StAttacking: continue
        target := w.FindEntity(u.TargetID)
        if target == nil || target.IsDead():
            if u.AttackMoveTarget set:
                // Scan radius 1.5*Range for nearest enemy entity
                u.TargetID = findNearestEnemy(w, u); continue
            else:
                u.State = StIdle; continue
        if dist(u, target) <= u.Range:
            events = append(events, DmgEvent{target.ID, u.Damage})
        else:
            stepPathToward(u, target.Pos)

    // Apply phase — order fixed by event insertion order (which was ID-sorted)
    for e in events:
        entity := w.FindEntity(e.Target)
        entity.HP -= e.Dmg
        if entity.HP <= 0 { entity.SetDead() }
}
```

**Attack-move semantics:** keep walking toward `AttackMoveTarget`; each tick scan 1.5×Range for nearest enemy and lock TargetID; after kill, continue walking; arriving at target → State=Idle.

### 4.3 Production & construction

**Building queue** (per-building field):
- `ProductionQueue []QueueItem` stored on Building
- Each tick decrement head's `TicksLeft`
- At 0: spawn Unit at first free edge cell; if all edge cells occupied, delay 1 tick; set new unit's MoveTo=RallyPoint (if set), State=StMoving; dequeue
- Queue length cap 5; `CmdTrain` silently rejected at cap (client UI also blocks)

**Worker build flow:**
1. Client build menu → placement mode → preview ghost follows mouse
2. Left-click → `CmdBuild(worker, type, pos)` emitted
3. Server sim on CmdBuild:
   - Validate resource + AABB-free
   - Pass → deduct resource; create Building{StConstructing, HP=1, Progress=0}; worker.State=StBuilding; TargetID=newBuildingID
   - Fail → silent drop
4. Worker arrives → each tick `ConstructProgress += 1/BuildTicks`; HP scales linearly to MaxHP
5. At 1.0 → Building.State=StReady; worker.State=StIdle
6. Construction damaged: Building HP drops normally; if HP→0, Building.SetDead; **resource is NOT refunded** (RTS convention)

**Worker mining loop:**
```
StIdle → [right-click crystal / CmdMove near crystal] → pathing to crystal
  → arrive → State=StMining(crystalID); internal counter 30 ticks
  → counter expires → take = min(CarryCapacity=5, crystal.Remaining); CarryAmount=take; crystal.Remaining -= take
    (if Remaining ≤ 0 after the take → crystal marked dead → worker finds next nearest)
  → State=StReturning(nearestOwnHQ); Path=A*(→HQ)
  → arrive at HQ range → Player.Crystal += CarryAmount; CarryAmount=0
  → State=StMining (same crystal if Remaining>0, else next nearest)
```

UI: no explicit Mine button; right-clicking a crystal enters the loop (client maps crystal-target right-click to the worker-mine intent).

---

## 5. FOW (Client-Side Only)

### 5.1 Data — Unity C#

```csharp
public class FOWState {
    public int W, H;
    public byte[] Cells;          // 0=unseen, 1=explored/memory, 2=visible
    public Dictionary<uint, LastSeenBuilding> Memory;
}
```

### 5.2 Update cadence

- Recomputed every 5 ticks (~0.25s) — balances smoothness against CPU
- Step:
  1. All Visible cells decay to Explored
  2. For each owned Unit and Building: paint VisionRange disc into Visible
  3. For newly Visible cells: update Memory snapshot of any enemy building observed

### 5.3 Render filtering

- **Friendly entities**: always rendered in full fidelity
- **Enemy UnitView**: if its cell != Visible → `SetActive(false)`
- **Enemy BuildingView**:
  - Visible → live state
  - Explored + Memory exists → render memory snapshot (stale HP)
  - Unseen → hidden
- **Minimap**: cells colored by FOW state + entity dots when visible
- **Selection**: box-select excludes hidden enemies (own units always selectable)
- **Command dispatch**: right-click on any non-Visible cell (Unseen OR Explored) → CmdMove, never CmdAttack (clients cannot reliably know what's actually there)

---

## 6. Scripted Go Bot (M2)

Location: `cmd/bot-client/main.go`. Reuses Sub-1's `pkg/rtsclient` for transport + wire + receiving FrameBundle. Bot runs a local copy of `internal/sim` to know world state (where enemies are, how much crystal it has).

**Strategy FSM (hard-coded):**
```go
type Phase int
const (PhaseEconomy=1; PhaseArmy=2; PhasePush=3)

Phase_Economy:
  - All 3 starting workers → nearest 3 crystals
  - When crystal ≥ 150 → CmdBuild(Barracks, HQ.Pos + (10, 0))
  - Barracks completed → phase = PhaseArmy

Phase_Army:
  - Continue mining
  - Barracks idle + crystal ≥ 80 → CmdTrain(Soldier)
  - Army size ≥ 5 → phase = PhasePush

Phase_Push:
  - All Soldiers CmdAttackMove(enemy HQ.Pos)  // initial HQ pos is fixed per-spawn, known
  - Continue training reinforcements
```

**Properties for testing:**
- Deterministic: `rand.New(fixedSeed)` for any internal jitter; same seed → same game
- Cheats on FOW (reads full world) — irrelevant to lockstep correctness, since bot only emits commands; server and human clients still run identical sim
- Runs in CI: `./server & ./bot-client -player=0 & ./bot-client -player=1 &` → 60s later assert `rts_desync_total == 0`

---

## 7. Unity Client Architecture

Sub-1's existing components are preserved unchanged. New C# classes by assembly:

```
RTS.Sim (extended)
  + NavGrid.cs
  + Building.cs
  + Crystal.cs
  + Player.cs
  + Pathfinding.cs
  (World.cs: + Buildings, Crystals, Players, NavGrid fields)
  (Step.cs: all new state machines + resolveCombat + applyPushAway + checkVictory)

RTS.Game (extended)
  + BuildingView.cs             HP bar + construction progress + footprint tile
  + BuildingViewPool.cs
  + CrystalView.cs
  + FOWState.cs
  + FOWRenderer.cs              per-frame visibility filter across UnitView/BuildingView
  + VictoryController.cs        listens for GameOver, activates VictoryPanel

RTS.Input (extended)
  + BuildMenuController.cs      B key → type picker → placement mode → CmdBuild
  + ControlGroups.cs            Ctrl+1..9 assign; 1..9 select
  (CommandDispatcher.cs: emits CmdAttack, CmdAttackMove, CmdBuild, CmdTrain, CmdSurrender)

RTS.UI (extended)
  + ResourceBar (top-right crystal counter)
  + SelectedUnitPanel (portrait + HP + action buttons)
  + ProductionQueue (overlay on BuildingView, shows queue + progress)
  + Minimap (FOW-shaded + entity dots)
  + VictoryPanel (Victory / Defeat / Draw)

RTS.Network (extended)
  (Messages.cs: new CmdOp values, GameOver message struct)
  (WireCodec.cs: encode/decode new messages + golden parity tests)
```

## 8. Server (Go) New Additions

```
cmd/bot-client/main.go                     new scripted bot binary

internal/sim/
  + nav.go                                 NavGrid + A*
  + building.go                            Building + production + construction
  + crystal.go                             Crystal + mining mechanics
  + player.go                              Player + resource pool
  (world.go extended)
  (step.go major: new states + resolveCombat + applyPushAway + checkVictory)
  (hash.go: extend canonical hash over Buildings, Crystals, Players, NavGrid.Blocked)
  (snapshot.go: Marshal/Unmarshal all new fields)

internal/wire/
  (messages.go: new CmdOp values, GameOver message)
  (codec.go: + encode/decode extensions)

internal/lockstep/
  (room.go: checkVictory called at sealTick end; CmdSurrender plumbing)
  (reconnect.go: unchanged — snapshot/resume path already handles new fields via Marshal extension)

test/determinism/
  + economy_test.go                        100× byte-equal: 3 workers mining 60s
  + combat_test.go                         100× byte-equal: 5v5 battle
  + full_game_test.go                      100× byte-equal: bot vs bot to end

test/chaos/
  + sub2_scenarios.go                      bot vs bot under 5% loss + 100ms jitter × 60 runs, 0 desync
```

---

## 9. Vertical Slice Execution Plan

Each phase ends with: bot-vs-bot CI passes + Unity-vs-bot manual playtest OK + relevant determinism tests byte-equal + desync counter 0.

### Phase 0 · Foundation prep (~2 days)

No new gameplay. Establish shared structures.
- Go + C# synchronized extension of Unit struct, World struct (Players/Buildings/Crystals/NavGrid fields)
- Extend golden export to include Buildings + Crystals
- Marshal/Unmarshal + hash for new structs
- Assert Sub-1 tests still pass with extended (but empty) new structures
- **DoD**: hash parity across Go/C# on empty World with new fields; all Sub-1 tests green.

### Phase 1 · Economy slice (~1 week)

**DoD: Workers mine, HQ produces Soldier on cooldown, resource counter visible. No combat yet.**
- sim: Crystal + Player.Crystal + Worker mining FSM + HQ ProductionQueue + fixed rally point
- wire: CmdTrain
- server: spawn crystals at fixed positions per player's territory; SpawnInitialUnits → 1×HQ + 3×Workers
- Unity: CrystalView, Player.Crystal sync, ResourceBar, worker right-click-crystal auto-enters Mining, HQ selected + Q to CmdTrain(Soldier)
- bot: Phase_Economy strategy
- **Network stress**: high-frequency worker state transitions (Mining↔Returning) — validate hash reconciliation under dense state churn
- **DoD**: `bot_vs_bot_economy_60s.go` passes; two Unity instances mine + build

### Phase 2 · Combat slice (~1 week)

**DoD: Soldiers can kill each other; right-click to attack, A-key attack-move both work; Victory/Defeat panel appears.**
- sim: resolveCombat + applyPushAway (still no A\* — units walk in straight lines and push-collide) + CmdAttack/CmdAttackMove + checkVictory + CmdSurrender
- wire: new CmdOps + GameOver message
- server: GameOver broadcast
- Unity: CommandDispatcher emits CmdAttack (right-click enemy) + CmdAttackMove (A + left-click); UnitView HP bar + damage flash + death fade; VictoryController + VictoryPanel
- bot: Phase_Army + Phase_Push (train 5 Soldiers → AttackMove enemy HQ)
- **Network stress**: death churn — unit IDs disappearing from list; validate target-resolution when target dies mid-tick
- **DoD**: `bot_vs_bot_full_game.go` terminates with a winner; two Unity instances can play to victory

### Phase 3 · Construction slice (~1 week)

**DoD: Worker can build Barracks/Archery/Stable; all 3 new unit types spawnable; production queue UI visible.**
- sim: StConstructing logic + worker StBuilding state + 3 new BuildingTypes + Archer/Cavalry sharing stepMove/stepAttack
- wire: CmdBuild + BuildingType enum
- server: build validation + spawn
- Unity: BuildMenuController (B key → pick type → mouse preview → left-click place); BuildingView placeholders for 4 types + progress bar; ProductionQueue UI; Building selected → Q/W/E to train different units
- bot: +"if crystal ≥ 100 and no Barracks → build Barracks; if ≥ 150 and no Archery → build Archery"
- **Network stress**: entity creation + state transitions (Constructing→Ready) + parallel multi-building production — list-insertion determinism under lockstep
- **DoD**: bot demos all 4 buildings + 4 unit types; replay byte-equal 100×

### Phase 4 · Pathfinding slice (~1.5 weeks)

**DoD: Units no longer clip through buildings; workers don't walk through HQ en route to crystal.**
- sim: NavGrid + A\* fixed-point + Blocked bitmap updated on building add/remove
- sim: Unit.Path field, stepMovePath consumes it
- C# parity: NavGrid.cs + Pathfinding.cs
- **Determinism audit**: same seed + same command stream → A\* path lists byte-equal
- **Network stress**: tick CPU rises (A\* calls) — monitor `rts_tick_duration_ms` p99 stays under 10ms
- **DoD**: Unity playthrough with no clipping; replay byte-equal 100× (now including path data)

### Phase 5 · Fog of War slice (~1 week)

**DoD: enemy units only visible inside vision; minimap shaded by FOW; Memory shows last-seen enemy buildings.**
- Unity: FOWState + FOWRenderer + Minimap
- **No sim changes** (biggest advantage of F2)
- SelectionManager: box-select excludes hidden enemies
- CommandDispatcher: right-click on unexplored → fallback to CmdMove
- **No wire / server / sim changes** — pure client work; network risk zero
- **DoD**: Unity match plays with proper FOW; desync counter still 0

### Phase 6 · Polish slice (~1 week)

**DoD: handling feels right; UI doesn't look rough.**
- ControlGroups (Ctrl+1..9 / 1..9)
- SelectedUnitPanel (portrait + HP + action buttons including S to stop)
- Rally point UI (select building, right-click ground to set)
- Attack VFX: Archer arrow trajectory line, Melee unit "lunge" animation
- Audio (optional; Sub-2 can ship without): attack / death / train-complete / build-complete
- Error toasts: insufficient resource, invalid placement, wrong building type
- Full playtesting round + value tuning (HP/Damage/Speed/Cost/Time)
- **DoD**: full match feels OK; all 9 end-to-end checklist items pass

---

## 10. End-to-End Acceptance Checklist

- [ ] Two Unity clients + server: full match from 0 crystal start to one side annihilated; ≤10 min duration
- [ ] bot vs bot × 100 matches × 60s: 0 desync, 0 late-seals, 0 crashes
- [ ] Under 5% packet loss + 100ms jitter: bot vs bot 60s × 10 matches → 0 desync
- [ ] Determinism suite (economy/combat/build/pathing/full_game) — 100× byte-equal each
- [ ] Replay: record a bot-vs-bot match; replay-play reproduces byte-equal
- [ ] Reconnect: Unity disconnects mid-match, reconnects, resumes with identical building state
- [ ] `/metrics` exposes new gauges: `rts_buildings_active`, `rts_construction_in_progress`, `rts_production_queue_depth`
- [ ] tick duration p99 < 10ms during 30-units × 300-tick engagement (A\* peak load)
- [ ] README Sub-2 section: rules, keybindings, victory condition, troubleshooting

---

## 11. Effort Summary

| Phase | Duration | Cumulative | Milestone |
|---|---|---|---|
| 0 | 2 days | 2 days | Structure alignment; Sub-1 tests green |
| 1 | 1 week | 1.5 weeks | "Economy works" |
| 2 | 1 week | 2.5 weeks | "Combat + Victory works" |
| 3 | 1 week | 3.5 weeks | "4 buildings + 4 units works" |
| 4 | 1.5 weeks | 5 weeks | "Pathfinding works" |
| 5 | 1 week | 6 weeks | "FOW works" |
| 6 | 1 week | **7 weeks** | "Sub-2 polished and closed" |

Total ~7 weeks full-time. ±1 week variance expected (A\* determinism audit, worker mining UX polish, and FOW minimap shading are common overrun sources).

---

## 12. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| A\* determinism leaks (map iteration, unstable tiebreak) | High | Desync | Phase 4 ends only when 100× byte-equal passes; add explicit unit test for equal-f tiebreak |
| FOW visibility recompute (5-tick cadence) causes frame-rate spikes | Medium | UX | Profile; fall back to per-tick incremental update if batch is too heavy |
| Unit push-away O(N²) hits p99 under 300-unit engagements | Low | tick >10ms | Spatial-hash bucket scheme ready as fallback; escalate to Sub-3 if needed |
| Building placement edge cases (unit standing on target cells) | High | Gameplay annoyance | Sim rejects placement silently; client UX shows red preview on invalid cells |
| Bot strategy too rigid → gameplay gets repetitive in manual testing | Low | QA UX | Accept; Sub-2's bot purpose is determinism tests, not engaging play |
| Worker mining FSM has resume-after-disconnect bug | Medium | Correctness | Include mining state in snapshot; reconnect_mid_mining e2e scenario in Phase 1 DoD |

---

## 13. Explicit Non-Goals (Deferred to Sub-3+)

- Pathfinding beyond P1 (flow field, ORCA, smart formations)
- Second resource type
- Tech tree / unit upgrades
- Defensive structures (towers, walls)
- High ground / terrain types / stealth / detectors
- Reactive AI opponent
- Pause / in-match timer / surrender negotiation
- Save-game / spectator / replay share
- Audio / advanced animations / cinematics
- Chat / lobby / matchmaking / ranked
- Mobile / controller input

---

## 14. Next Step

After this spec is approved, the brainstorming skill hands off to `superpowers:writing-plans` to produce a detailed implementation plan (Phase 0 will be the first executable plan; later phases get their own plans as their predecessors finish).
