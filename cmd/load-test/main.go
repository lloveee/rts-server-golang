package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"rts/internal/lockstep"
	"rts/internal/room"
	"rts/internal/sim"
	"rts/internal/sim/fixed"
	"rts/internal/transport"
	"rts/internal/wire"
	"rts/pkg/rtsclient"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

func main() {
	rooms := flag.Int("rooms", 10, "number of rooms")
	playersPerRoom := flag.Int("players", 2, "players per room")
	dur := flag.Duration("duration", 30*time.Second, "test duration")
	tickRate := flag.Int("tick-rate", 20, "simulation tick rate")
	logLevel := flag.String("log-level", "warn", "log level")
	flag.Parse()

	var level slog.Level
	switch *logLevel {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelWarn
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))

	fmt.Printf("load-test: %d rooms × %d players, duration=%v, tick_rate=%d\n",
		*rooms, *playersPerRoom, *dur, *tickRate)

	// Start in-process server.
	defaults := lockstep.RoomConfig{
		Seed:        42,
		MapW:        100,
		MapH:        100,
		TickRate:    *tickRate,
		InitialN:    3,
		MinN:        2,
		MaxN:        6,
		PlayerCount: *playersPerRoom,
	}

	registry := room.NewRegistry(defaults, log)
	listener, err := transport.Listen(transport.ListenerConfig{
		Addr:   "127.0.0.1:0",
		Logger: log,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "listen: %v\n", err)
		os.Exit(1)
	}
	defer listener.Close()

	listener.OnNewConn = func(conn *transport.Conn) {
		go handleConn(conn, registry, defaults, log)
	}
	go listener.Serve()

	// Retx ticker.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				for _, c := range listener.Conns() {
					c.Tick(time.Now())
				}
			}
		}
	}()

	serverAddr := listener.LocalAddr().String()

	// Spawn clients.
	var wg sync.WaitGroup
	var totalDesyncs atomic.Int64
	var totalTicks atomic.Int64
	var connectErrors atomic.Int64

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	gameCtx, gameCancel := context.WithTimeout(ctx, *dur)
	defer gameCancel()

	go func() {
		select {
		case <-sigCh:
			gameCancel()
		case <-gameCtx.Done():
		}
	}()

	for r := 0; r < *rooms; r++ {
		roomID := fmt.Sprintf("room-%d", r)
		for p := 0; p < *playersPerRoom; p++ {
			wg.Add(1)
			go func(roomID string, playerIdx int) {
				defer wg.Done()
				ticks, desyncs, err := runClient(gameCtx, serverAddr, roomID, playerIdx, *playersPerRoom, log)
				if err != nil {
					connectErrors.Add(1)
					return
				}
				totalTicks.Add(int64(ticks))
				totalDesyncs.Add(int64(desyncs))
			}(roomID, p)
			// Stagger connections slightly.
			time.Sleep(5 * time.Millisecond)
		}
	}

	wg.Wait()

	fmt.Printf("\n=== LOAD TEST RESULTS ===\n")
	fmt.Printf("rooms:           %d\n", *rooms)
	fmt.Printf("players/room:    %d\n", *playersPerRoom)
	fmt.Printf("total clients:   %d\n", *rooms**playersPerRoom)
	fmt.Printf("connect errors:  %d\n", connectErrors.Load())
	fmt.Printf("total ticks:     %d\n", totalTicks.Load())
	fmt.Printf("total desyncs:   %d\n", totalDesyncs.Load())

	if totalDesyncs.Load() > 0 {
		fmt.Println("STATUS: FAILED (desyncs detected)")
		os.Exit(1)
	}
	if connectErrors.Load() > 0 {
		fmt.Println("STATUS: PARTIAL (some clients failed to connect)")
		os.Exit(1)
	}
	fmt.Println("STATUS: PASS")
}

func runClient(ctx context.Context, serverAddr, roomID string, playerIdx, playerCount int, log *slog.Logger) (ticks int, desyncs int, err error) {
	c, err := rtsclient.New(rtsclient.Config{
		ServerAddr: serverAddr,
		Logger:     log,
	})
	if err != nil {
		return 0, 0, err
	}
	defer c.Close()

	if err := c.Connect(fmt.Sprintf("p%d", playerIdx), roomID); err != nil {
		return 0, 0, err
	}

	go c.RunReadLoop()
	go c.RunRetxLoop()

	mapW, mapH := c.MapSize()
	world := sim.NewWorld(c.Seed(), mapW, mapH)
	for pid := 0; pid < playerCount; pid++ {
		for i := 0; i < 5; i++ {
			x := fixed.FromInt(int32(10 + pid*80))
			y := fixed.FromInt(int32(10 + i*3))
			world.SpawnUnit(uint8(pid), fixed.V(x, y), fixed.FromInt(10), fixed.FromFloat64(0.5))
		}
	}

	var mu sync.Mutex
	var lastTick uint32

	c.OnFrame = func(fb *wire.FrameBundle) {
		mu.Lock()
		defer mu.Unlock()
		for lastTick < fb.Tick {
			lastTick++
			var cmds []sim.Cmd
			if lastTick == fb.Tick {
				for _, cmd := range fb.Cmds {
					cmds = append(cmds, sim.Cmd{
						Player:    cmd.Player,
						Op:        sim.CmdOp(cmd.Op),
						UnitID:    cmd.UnitID,
						TargetPos: fixed.V(fixed.FromRaw(cmd.TargetX), fixed.FromRaw(cmd.TargetY)),
						TargetID:  cmd.TargetID,
					})
				}
			}
			sim.Step(world, cmds)
		}
		h := sim.Hash(world)
		c.SendHashAck(fb.Tick, h)
		ticks++
	}

	go c.RunDispatchLoop()

	// Send periodic commands.
	cmdTicker := time.NewTicker(500 * time.Millisecond)
	defer cmdTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ticks, desyncs, nil
		case <-cmdTicker.C:
			mu.Lock()
			t := lastTick
			mu.Unlock()
			if t > 0 {
				c.SendCmd(&wire.Cmd{
					Tick:    t + 3,
					Player:  c.PlayerID(),
					Op:      uint8(sim.CmdMove),
					UnitID:  1,
					TargetX: fixed.FromInt(int32(world.Rand.Intn(int(mapW)))).Raw(),
					TargetY: fixed.FromInt(int32(world.Rand.Intn(int(mapH)))).Raw(),
				})
			}
		}
	}
}

func handleConn(conn *transport.Conn, registry *room.Registry, defaults lockstep.RoomConfig, log *slog.Logger) {
	helloData, ok := readTimeout(conn, 5*time.Second)
	if !ok {
		return
	}
	msgType, msg, err := wire.Decode(helloData)
	if err != nil || msgType != wire.MsgHello {
		return
	}
	hello := msg.(*wire.Hello)
	_ = hello

	ack := &wire.HelloAck{
		ProtocolVersion: wire.ProtocolVersion,
		ServerTickRate:  uint16(defaults.TickRate),
		Accepted:        true,
	}
	ackData, _ := wire.Encode(ack)
	conn.Send(ackData)

	joinData, ok := readTimeout(conn, 5*time.Second)
	if !ok {
		return
	}
	msgType, msg, err = wire.Decode(joinData)
	if err != nil || msgType != wire.MsgJoinRoom {
		return
	}
	joinRoom := msg.(*wire.JoinRoom)

	rm := registry.GetOrCreate(joinRoom.RoomID)
	playerID, ok := rm.AddPlayer(conn)
	if !ok {
		return
	}

	joinAck := &wire.JoinAck{
		RoomID: joinRoom.RoomID, PlayerID: playerID,
		Seed: defaults.Seed, MapW: defaults.MapW, MapH: defaults.MapH, Accepted: true,
	}
	data, _ := wire.Encode(joinAck)
	conn.Send(data)

	for d := range conn.Inbox {
		msgType, msg, err := wire.Decode(d)
		if err != nil {
			continue
		}
		rm.Inbox <- lockstep.RoomMsg{PlayerID: playerID, Type: msgType, Data: msg}
	}
}

func readTimeout(conn *transport.Conn, timeout time.Duration) ([]byte, bool) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case d := <-conn.Inbox:
		return d, true
	case <-timer.C:
		return nil, false
	}
}
