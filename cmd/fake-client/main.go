package main

import (
	"context"
	"flag"
	"fmt"
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
	playerName := flag.String("name", "bot", "player name")
	roomID := flag.String("room", "test-room", "room ID to join")
	duration := flag.Duration("duration", 60*time.Second, "game duration")
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

	// Start read/retx loops after handshake completes.
	go client.RunReadLoop()
	go client.RunRetxLoop()

	mapW, mapH := client.MapSize()
	world := sim.NewWorld(client.Seed(), mapW, mapH)

	// Spawn initial units (must match server).
	playerCount := 2 // hardcoded for now, will be in JoinAck later
	unitsPerPlayer := 5
	for pid := 0; pid < playerCount; pid++ {
		for i := 0; i < unitsPerPlayer; i++ {
			x := fixed.FromInt(int32(10 + pid*80))
			y := fixed.FromInt(int32(10 + i*3))
			world.SpawnUnit(uint8(pid), fixed.V(x, y),
				fixed.FromInt(10), fixed.FromFloat64(0.5))
		}
	}

	myID := client.PlayerID()
	var lastTick uint32
	desyncCount := 0

	// Handle frames.
	client.OnFrame = func(fb *wire.FrameBundle) {
		// Execute all ticks up to this frame's tick.
		for lastTick < fb.Tick {
			lastTick++
			var cmds []sim.Cmd
			if lastTick == fb.Tick {
				cmds = wireCmdsToSimCmds(fb.Cmds)
			}
			sim.Step(world, cmds)
		}

		// Hash and report.
		hash := sim.Hash(world)
		if err := client.SendHashAck(fb.Tick, hash); err != nil {
			log.Warn("send hash", "err", err)
		}

		if fb.Tick%100 == 0 {
			log.Info("tick", "tick", fb.Tick, "hash", fmt.Sprintf("%016x", hash), "units", len(world.Units))
		}
	}

	go client.RunDispatchLoop()

	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Send some commands periodically.
	cmdTicker := time.NewTicker(500 * time.Millisecond)
	defer cmdTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info("game ended", "ticks", lastTick, "desyncs", desyncCount)
			return
		case <-sigCh:
			log.Info("interrupted", "ticks", lastTick)
			return
		case <-cmdTicker.C:
			// Send a random move command for one of our units.
			if lastTick > 0 && len(world.Units) > 0 {
				for _, u := range world.Units {
					if u.Owner == myID && u.State != sim.UnitDead {
						cmd := &wire.Cmd{
							Tick:    lastTick + uint32(3), // N=3
							Player:  myID,
							Op:      uint8(sim.CmdMove),
							UnitID:  u.ID,
							TargetX: fixed.FromInt(int32(world.Rand.Intn(int(mapW)))).Raw(),
							TargetY: fixed.FromInt(int32(world.Rand.Intn(int(mapH)))).Raw(),
						}
						client.SendCmd(cmd)
						break
					}
				}
			}
		}
	}
}

func wireCmdsToSimCmds(cmds []wire.Cmd) []sim.Cmd {
	if len(cmds) == 0 {
		return nil
	}
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
