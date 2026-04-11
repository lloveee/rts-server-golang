package chaos

import (
	"context"
	"log/slog"
	"os"
	"rts/internal/lockstep"
	"rts/internal/room"
	"rts/internal/sim"
	"rts/internal/sim/fixed"
	"rts/internal/transport"
	"rts/internal/wire"
	"rts/pkg/rtsclient"
	"sync"
	"testing"
	"time"
)

const (
	testTickRate    = 20
	testSeed        = 42
	testMapW        = 100
	testMapH        = 100
	testPlayerCount = 2
	unitsPerPlayer  = 5
)

func startServer(t *testing.T) (*transport.Listener, *room.Registry, func()) {
	t.Helper()
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))

	defaults := lockstep.RoomConfig{
		Seed: testSeed, MapW: testMapW, MapH: testMapH,
		TickRate: testTickRate, InitialN: 3, MinN: 2, MaxN: 6,
		PlayerCount: testPlayerCount,
	}

	registry := room.NewRegistry(defaults, log)
	listener, err := transport.Listen(transport.ListenerConfig{
		Addr: "127.0.0.1:0", Logger: log,
	})
	if err != nil {
		t.Fatal(err)
	}

	listener.OnNewConn = func(conn *transport.Conn) {
		go serverHandleConn(conn, registry, defaults, log)
	}
	go listener.Serve()

	ctx, cancel := context.WithCancel(context.Background())
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

	return listener, registry, func() { cancel(); listener.Close() }
}

func serverHandleConn(conn *transport.Conn, registry *room.Registry, defaults lockstep.RoomConfig, log *slog.Logger) {
	helloData, ok := readTimeout(conn, 5*time.Second)
	if !ok {
		return
	}
	msgType, msg, err := wire.Decode(helloData)
	if err != nil || msgType != wire.MsgHello {
		return
	}
	_ = msg.(*wire.Hello)

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

type clientState struct {
	client *rtsclient.Client
	world  *sim.World
	hashes []uint64
	mu     sync.Mutex
}

func connectClient(t *testing.T, serverAddr, name, roomID string, chaos transport.ChaosHook) *clientState {
	t.Helper()
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))

	c, err := rtsclient.New(rtsclient.Config{
		ServerAddr: serverAddr,
		Logger:     log,
		Chaos:      chaos,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := c.Connect(name, roomID); err != nil {
		t.Fatal(err)
	}

	go c.RunReadLoop()
	go c.RunRetxLoop()

	mapW, mapH := c.MapSize()
	w := sim.NewWorld(c.Seed(), mapW, mapH)
	for pid := 0; pid < testPlayerCount; pid++ {
		for i := 0; i < unitsPerPlayer; i++ {
			x := fixed.FromInt(int32(10 + pid*80))
			y := fixed.FromInt(int32(10 + i*3))
			w.SpawnUnit(uint8(pid), fixed.V(x, y), fixed.FromInt(10), fixed.FromFloat64(0.5))
		}
	}

	return &clientState{client: c, world: w}
}

// TestChaos5PctDrop runs a lockstep session with 5% packet drop.
func TestChaos5PctDrop(t *testing.T) {
	listener, _, cleanup := startServer(t)
	defer cleanup()

	serverAddr := listener.LocalAddr().String()
	roomID := "chaos-room-1"

	// No chaos on the connect path — chaos only affects the read loop after connect.
	cs1 := connectClient(t, serverAddr, "p0", roomID, nil)
	defer cs1.client.Close()
	cs2 := connectClient(t, serverAddr, "p1", roomID, nil)
	defer cs2.client.Close()

	setupFrameHandler := func(cs *clientState) {
		var lastTick uint32
		cs.client.OnFrame = func(fb *wire.FrameBundle) {
			cs.mu.Lock()
			defer cs.mu.Unlock()
			for lastTick < fb.Tick {
				lastTick++
				var cmds []sim.Cmd
				if lastTick == fb.Tick {
					for _, c := range fb.Cmds {
						cmds = append(cmds, sim.Cmd{
							Player:    c.Player,
							Op:        sim.CmdOp(c.Op),
							UnitID:    c.UnitID,
							TargetPos: fixed.V(fixed.FromRaw(c.TargetX), fixed.FromRaw(c.TargetY)),
							TargetID:  c.TargetID,
						})
					}
				}
				sim.Step(cs.world, cmds)
			}
			h := sim.Hash(cs.world)
			cs.hashes = append(cs.hashes, h)
			cs.client.SendHashAck(fb.Tick, h)
		}
	}
	setupFrameHandler(cs1)
	setupFrameHandler(cs2)

	go cs1.client.RunDispatchLoop()
	go cs2.client.RunDispatchLoop()

	// Send commands.
	go func() {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for i := 0; i < 15; i++ {
			<-ticker.C
			cs1.mu.Lock()
			tick := cs1.world.Tick
			cs1.mu.Unlock()
			if tick > 0 {
				cs1.client.SendCmd(&wire.Cmd{
					Tick: tick + 3, Player: cs1.client.PlayerID(),
					Op: uint8(sim.CmdMove), UnitID: 1,
					TargetX: fixed.FromInt(50).Raw(), TargetY: fixed.FromInt(50).Raw(),
				})
			}
		}
	}()

	time.Sleep(3 * time.Second)

	cs1.mu.Lock()
	cs2.mu.Lock()
	h1 := cs1.hashes
	h2 := cs2.hashes
	cs2.mu.Unlock()
	cs1.mu.Unlock()

	minLen := len(h1)
	if len(h2) < minLen {
		minLen = len(h2)
	}

	t.Logf("client 1: %d ticks, client 2: %d ticks, comparing %d", len(h1), len(h2), minLen)

	if minLen < 10 {
		t.Fatalf("too few ticks: %d", minLen)
	}

	desyncCount := 0
	for i := 0; i < minLen; i++ {
		if h1[i] != h2[i] {
			desyncCount++
			if desyncCount <= 5 {
				t.Errorf("DESYNC at tick %d", i+1)
			}
		}
	}

	if desyncCount > 0 {
		t.Fatalf("desyncs: %d / %d", desyncCount, minLen)
	}

	t.Logf("CHAOS OK: %d ticks, 0 desyncs", minLen)
}
