// Bot client binary for the RTS lockstep server.
// Connects, joins a room, runs a local deterministic sim, and executes a
// Phase_Economy strategy: assigns idle Workers to nearest unsaturated
// crystals and trains new Workers from HQ when affordable.
package main

import (
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"rts/internal/sim"
	"rts/internal/sim/fixed"
	"rts/internal/wire"
	"rts/pkg/rtsclient"
	"syscall"
	"time"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:9000", "server address")
	playerName := flag.String("name", "Bot", "player name")
	roomID := flag.String("room", "bot-room", "room ID to join")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	client, err := rtsclient.New(rtsclient.Config{
		ServerAddr: *addr,
		Logger:     log,
	})
	if err != nil {
		log.Error("create client", "err", err)
		os.Exit(1)
	}
	defer client.Close()

	if err := client.Connect(*playerName, *roomID); err != nil {
		log.Error("connect", "err", err)
		os.Exit(1)
	}

	// Start transport goroutines.
	go client.RunReadLoop()
	go client.RunRetxLoop()

	mapW, mapH := client.MapSize()
	seed := client.Seed()
	myID := client.PlayerID()

	log.Info("bot joined", "player_id", myID, "seed", seed, "map_w", mapW, "map_h", mapH)

	// Build the deterministic initial world state (must match server SpawnInitialWorld exactly).
	world := spawnInitialWorld(seed, mapW, mapH)

	// Tick state (read/written only by OnFrame, i.e. sequentially within dispatch goroutine).
	var lastTick uint32
	var lastTrainTick uint32
	var phase = "economy"
	var soldierIDs []uint32

	// OnFrame processes each FrameBundle from the server: step sim, hash, and run bot logic.
	client.OnFrame = func(fb *wire.FrameBundle) {
		// Step forward one tick at a time.  Intermediate ticks (no bundled commands)
		// execute without input.
		for lastTick < fb.Tick {
			lastTick++
			var cmds []sim.Cmd
			if lastTick == fb.Tick {
				cmds = wireCmdsToSimCmds(fb.Cmds)
			}
			sim.Step(world, cmds)
		}

		// Report hash so the server can detect desyncs.
		hash := sim.Hash(world)
		if err := client.SendHashAck(fb.Tick, hash); err != nil {
			log.Warn("send hash", "err", err)
		}

		// Every 100 ticks log a summary line for observability.
		if fb.Tick%100 == 0 {
			var workerCount int
			for _, u := range world.Units {
				if u.Owner == myID && u.Type == sim.UnitWorker && u.State != sim.UnitDead {
					workerCount++
				}
			}
			crystal := fixed.Zero
			if int(myID) < len(world.Players) {
				crystal = world.Players[myID].Crystal
			}
			log.Info("tick", "tick", fb.Tick, "hash", hash,
				"units", len(world.Units), "workers", workerCount,
				"crystal", crystal.ToInt())
		}

		botThink(world, myID, fb.Tick, &lastTrainTick, &phase, &soldierIDs, client)
	}

	go client.RunDispatchLoop()

	// Main goroutine: keep-alive ticker.  All game logic runs inside OnFrame above.
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case <-sigCh:
			log.Info("bot shutting down", "total_ticks", lastTick)
			return
		case <-ticker.C:
			// keep the process alive; OnFrame drives the sim and strategy.
		}
	}
}

// spawnInitialWorld creates a world that exactly mirrors the server's
// SpawnInitialWorld: 2 players, each with 1 HQ, 3 Workers, 8 Crystals,
// and 200 starting crystal.
func spawnInitialWorld(seed uint64, mapW, mapH int32) *sim.World {
	w := sim.NewWorld(seed, mapW, mapH)

	const playerCount = 2
	for pid := 0; pid < playerCount; pid++ {
		playerID := uint8(pid)
		hqX := int32(10)
		if pid == 1 {
			hqX = 90
		}
		hqPos := fixed.VInt(hqX, 45)
		w.SpawnBuilding(playerID, sim.BldHQ, hqPos)

		stats := sim.UnitStatTable[sim.UnitWorker]
		for wk := int32(0); wk < 3; wk++ {
			workerX := hqX + 3 + wk
			workerY := int32(45) + wk
			w.SpawnUnit(playerID,
				fixed.VInt(workerX, workerY),
				stats.MaxHP, stats.Speed)
			w.Units[len(w.Units)-1].Type = sim.UnitWorker
		}

		for _, pos := range sim.CrystalPositions(playerID) {
			w.SpawnCrystal(pos)
		}
	}

	w.Players = make([]sim.Player, playerCount)
	for i := range w.Players {
		w.Players[i] = sim.Player{ID: uint8(i)}
	}
	w.Players[0].Crystal = fixed.FromInt(200)
	w.Players[1].Crystal = fixed.FromInt(200)

	return w
}

// botThink implements the Phase_Economy strategy:
//  1. Assign every idle Worker to the nearest crystal that still has resources
//     and fewer than 2 workers already assigned.
//  2. Train a new Worker from HQ when the player has enough crystal and the
//     training queue is not full.
func botThink(w *sim.World, myID uint8, tick uint32, lastTrainTick *uint32, phase *string, soldierIDs *[]uint32, client *rtsclient.Client) {
	futureTick := tick + 3
	if int(myID) >= len(w.Players) {
		return
	}

	// --- Phase progression ---
	if tick%500 == 0 {
		slog.Info("botThink", "tick", tick, "phase", *phase, "crystal", w.Players[myID].Crystal.ToInt())
	}
	if *phase == "economy" && w.Players[myID].Crystal >= fixed.FromInt(150) {
		*phase = "army"
		slog.Info("phase -> army", "crystal", w.Players[myID].Crystal.ToInt())
	}
	if *phase == "army" {
		// Collect soldier IDs and count alive.
		*soldierIDs = (*soldierIDs)[:0]
		for i := range w.Units {
			u := &w.Units[i]
			if u.Owner == myID && u.Type == sim.UnitSoldier && u.State != sim.UnitDead {
				*soldierIDs = append(*soldierIDs, u.ID)
			}
		}
		if len(*soldierIDs) >= 5 {
			*phase = "push"
		}
	}

	// --- Always assign idle workers to crystals (all phases) ---
	for i := range w.Units {
		u := &w.Units[i]
		if u.Owner != myID || u.State != sim.UnitIdle || u.Type != sim.UnitWorker {
			continue
		}
		best := findBestCrystal(w, u.Pos)
		if best == nil {
			continue
		}
		_ = client.SendCmd(&wire.Cmd{
			Tick: futureTick, Player: myID, Op: uint8(sim.CmdMove),
			UnitID: u.ID, TargetX: best.Pos.X.Raw(), TargetY: best.Pos.Y.Raw(),
		})
	}

	// --- Train units from HQ ---
	trainType := sim.UnitWorker
	var cost fixed.Fix32
	if *phase == "economy" {
		trainType = sim.UnitWorker
		cost = sim.UnitStatTable[sim.UnitWorker].Cost
	} else {
		trainType = sim.UnitSoldier
		cost = sim.UnitStatTable[sim.UnitSoldier].Cost
	}

	if w.Players[myID].Crystal >= cost &&
		(tick-*lastTrainTick >= 30 || *lastTrainTick == 0) {
		hq := findPlayerHQ(w, myID)
		if hq != nil && len(hq.ProductionQueue) < sim.MaxQueueLength {
			*lastTrainTick = tick
			_ = client.SendCmd(&wire.Cmd{
				Tick: futureTick, Player: myID, Op: uint8(sim.CmdTrain),
				UnitID: hq.ID, TargetID: uint32(trainType),
			})
		}
	}

	// --- Phase_push: AttackMove soldiers to enemy HQ ---
	if *phase == "push" {
		enemyHQ := findEnemyHQ(w, myID)
		if enemyHQ != nil {
			for _, sid := range *soldierIDs {
				u := w.FindUnit(sid)
				if u != nil && u.State == sim.UnitIdle {
					_ = client.SendCmd(&wire.Cmd{
						Tick: futureTick, Player: myID, Op: uint8(sim.CmdAttackMove),
						UnitID: sid, TargetX: enemyHQ.Pos.X.Raw(), TargetY: enemyHQ.Pos.Y.Raw(),
					})
				}
			}
		}
	}
}

// findBestCrystal returns the nearest crystal with Remaining > 0 that has
// fewer than 2 workers already assigned.  Returns nil if no suitable crystal.
func findBestCrystal(w *sim.World, pos fixed.Vec2) *sim.Crystal {
	var best *sim.Crystal
	var bestDistSq fixed.Fix32

	for i := range w.Crystals {
		c := &w.Crystals[i]
		if c.Remaining <= 0 {
			continue
		}
		if workersAssigned(w, c.ID) >= 2 {
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

// workersAssigned counts non-dead Workers whose TargetID points to the given crystal.
func workersAssigned(w *sim.World, crystalID uint32) int {
	count := 0
	for i := range w.Units {
		u := &w.Units[i]
		if u.State == sim.UnitDead {
			continue
		}
		if u.Type != sim.UnitWorker {
			continue
		}
		if u.TargetID == crystalID {
			count++
		}
	}
	return count
}

// findPlayerHQ returns the first Ready HQ building owned by the given player.
func findPlayerHQ(w *sim.World, owner uint8) *sim.Building {
	for i := range w.Buildings {
		b := &w.Buildings[i]
		if b.Owner == owner && b.Type == sim.BldHQ && b.State == sim.BldReady {
			return b
		}
	}
	return nil
}

// findEnemyHQ returns the nearest ready HQ owned by a different player.
func findEnemyHQ(w *sim.World, myID uint8) *sim.Building {
	for i := range w.Buildings {
		b := &w.Buildings[i]
		if b.Owner != myID && b.Type == sim.BldHQ && b.State == sim.BldReady {
			return b
		}
	}
	return nil
}

// wireCmdsToSimCmds converts wire-layer commands to sim-layer commands.
func wireCmdsToSimCmds(wireCmds []wire.Cmd) []sim.Cmd {
	if len(wireCmds) == 0 {
		return nil
	}
	result := make([]sim.Cmd, len(wireCmds))
	for i, c := range wireCmds {
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
