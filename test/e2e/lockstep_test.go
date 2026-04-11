package e2e

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"rts/internal/lockstep"
	"rts/internal/replay"
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
	testUnitsPerPlayer = 5
)

func startServer(t *testing.T) (*transport.Listener, *room.Registry, func()) {
	t.Helper()
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))

	defaults := lockstep.RoomConfig{
		Seed:        testSeed,
		MapW:        testMapW,
		MapH:        testMapH,
		TickRate:    testTickRate,
		InitialN:    3,
		MinN:        2,
		MaxN:        6,
		PlayerCount: testPlayerCount,
	}

	registry := room.NewRegistry(defaults, log)

	listener, err := transport.Listen(transport.ListenerConfig{
		Addr:   "127.0.0.1:0",
		Logger: log,
	})
	if err != nil {
		t.Fatal(err)
	}

	listener.OnNewConn = func(conn *transport.Conn) {
		go serverHandleConn(conn, registry, defaults, log)
	}

	go listener.Serve()

	// Retx ticker.
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

	cleanup := func() {
		cancel()
		listener.Close()
	}
	return listener, registry, cleanup
}

func serverHandleConn(conn *transport.Conn, registry *room.Registry, defaults lockstep.RoomConfig, log *slog.Logger) {
	helloData, ok := readConnTimeout(conn, 5*time.Second)
	if !ok {
		return
	}
	msgType, msg, err := wire.Decode(helloData)
	if err != nil || msgType != wire.MsgHello {
		return
	}
	hello := msg.(*wire.Hello)

	ack := &wire.HelloAck{
		ProtocolVersion: wire.ProtocolVersion,
		ServerTickRate:  uint16(defaults.TickRate),
		Accepted:        hello.ProtocolVersion == wire.ProtocolVersion,
	}
	ackData, _ := wire.Encode(ack)
	conn.Send(ackData)

	joinData, ok := readConnTimeout(conn, 5*time.Second)
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

func readConnTimeout(conn *transport.Conn, timeout time.Duration) ([]byte, bool) {
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

func connectClient(t *testing.T, serverAddr, name, roomID string) *clientState {
	t.Helper()
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))

	c, err := rtsclient.New(rtsclient.Config{
		ServerAddr: serverAddr,
		Logger:     log,
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
		for i := 0; i < testUnitsPerPlayer; i++ {
			x := fixed.FromInt(int32(10 + pid*80))
			y := fixed.FromInt(int32(10 + i*3))
			w.SpawnUnit(uint8(pid), fixed.V(x, y), fixed.FromInt(10), fixed.FromFloat64(0.5))
		}
	}

	cs := &clientState{client: c, world: w}
	return cs
}

func TestLockstep2Players60s(t *testing.T) {
	listener, _, cleanup := startServer(t)
	defer cleanup()

	serverAddr := listener.LocalAddr().String()
	roomID := "test-room-1"

	// Connect 2 clients.
	cs1 := connectClient(t, serverAddr, "p0", roomID)
	defer cs1.client.Close()
	cs2 := connectClient(t, serverAddr, "p1", roomID)
	defer cs2.client.Close()

	// Setup frame handlers that step sim and collect hashes.
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

	// Let it run for 3 seconds (60 ticks at 20Hz).
	duration := 3 * time.Second
	t.Logf("running lockstep for %v...", duration)

	// Send some commands from each client.
	go func() {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for i := 0; i < 15; i++ {
			<-ticker.C
			cs1.mu.Lock()
			tick := cs1.world.Tick
			cs1.mu.Unlock()
			if tick > 0 {
				cmd := &wire.Cmd{
					Tick:    tick + 3,
					Player:  cs1.client.PlayerID(),
					Op:      uint8(sim.CmdMove),
					UnitID:  1,
					TargetX: fixed.FromInt(int32(50)).Raw(),
					TargetY: fixed.FromInt(int32(50)).Raw(),
				}
				cs1.client.SendCmd(cmd)
			}
		}
	}()
	go func() {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for i := 0; i < 15; i++ {
			<-ticker.C
			cs2.mu.Lock()
			tick := cs2.world.Tick
			cs2.mu.Unlock()
			if tick > 0 {
				cmd := &wire.Cmd{
					Tick:    tick + 3,
					Player:  cs2.client.PlayerID(),
					Op:      uint8(sim.CmdMove),
					UnitID:  6,
					TargetX: fixed.FromInt(int32(50)).Raw(),
					TargetY: fixed.FromInt(int32(50)).Raw(),
				}
				cs2.client.SendCmd(cmd)
			}
		}
	}()

	time.Sleep(duration)

	// Compare hashes.
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
		t.Fatalf("too few ticks executed: %d (expected at least 10)", minLen)
	}

	desyncCount := 0
	for i := 0; i < minLen; i++ {
		if h1[i] != h2[i] {
			desyncCount++
			if desyncCount <= 5 {
				t.Errorf("DESYNC at tick %d: client1=%016x client2=%016x", i+1, h1[i], h2[i])
			}
		}
	}

	if desyncCount > 0 {
		t.Fatalf("total desyncs: %d / %d ticks", desyncCount, minLen)
	}

	t.Logf("SUCCESS: %d ticks, 0 desyncs, final hash: %016x",
		minLen, fmt.Sprintf("%016x", h1[minLen-1]))
}

func TestReplayRecordAndVerify(t *testing.T) {
	// Start server.
	listener, registry, cleanup := startServer(t)
	defer cleanup()

	serverAddr := listener.LocalAddr().String()
	roomID := "replay-test-room"

	// Connect 2 clients.
	cs1 := connectClient(t, serverAddr, "p0", roomID)
	defer cs1.client.Close()
	cs2 := connectClient(t, serverAddr, "p1", roomID)
	defer cs2.client.Close()

	// Attach replay writer to the room.
	rm := registry.Get(roomID)
	if rm == nil {
		t.Fatal("room not found")
	}

	var replayBuf bytes.Buffer
	rw := replay.NewWriter(&replayBuf)

	// Write header with initial snapshot.
	snapshot := sim.Marshal(rm.World())
	header := &replay.Header{
		ProtoVersion: wire.ProtocolVersion,
		Seed:         testSeed,
		TickRate:     uint16(testTickRate),
		StartTS:      time.Now().Unix(),
		PlayerCount:  uint8(testPlayerCount),
		MapW:         testMapW,
		MapH:         testMapH,
		Snapshot:     snapshot,
	}
	if err := rw.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	rm.SetReplayWriter(rw)

	// Setup frame handlers.
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

	// Send some commands.
	go func() {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for i := 0; i < 10; i++ {
			<-ticker.C
			cs1.mu.Lock()
			tick := cs1.world.Tick
			cs1.mu.Unlock()
			if tick > 0 {
				cs1.client.SendCmd(&wire.Cmd{
					Tick: tick + 3, Player: cs1.client.PlayerID(),
					Op: uint8(sim.CmdMove), UnitID: 1,
					TargetX: fixed.FromInt(60).Raw(), TargetY: fixed.FromInt(60).Raw(),
				})
			}
		}
	}()

	time.Sleep(3 * time.Second)

	// Now replay the recording.
	t.Log("replaying recorded session...")
	replayData := replayBuf.Bytes()
	reader, err := replay.NewReader(replayData)
	if err != nil {
		t.Fatal(err)
	}

	// Restore world from initial snapshot.
	replayWorld, err := sim.Unmarshal(reader.Header.Snapshot)
	if err != nil {
		t.Fatal(err)
	}

	ticks, err := reader.ReadAll()
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("replay: %d ticks recorded", len(ticks))
	if len(ticks) < 10 {
		t.Fatalf("too few ticks recorded: %d", len(ticks))
	}

	desyncCount := 0
	for _, tr := range ticks {
		cmds := make([]sim.Cmd, len(tr.Cmds))
		for i, c := range tr.Cmds {
			cmds[i] = sim.Cmd{
				Player:    c.Player,
				Op:        sim.CmdOp(c.Op),
				UnitID:    c.UnitID,
				TargetPos: fixed.V(fixed.FromRaw(c.TargetX), fixed.FromRaw(c.TargetY)),
				TargetID:  c.TargetID,
			}
		}
		sim.Step(replayWorld, cmds)
		hash := sim.Hash(replayWorld)
		if hash != tr.Hash {
			desyncCount++
			if desyncCount <= 5 {
				t.Errorf("replay DESYNC at tick %d: recorded=%016x computed=%016x", tr.Tick, tr.Hash, hash)
			}
		}
	}

	if desyncCount > 0 {
		t.Fatalf("replay desyncs: %d / %d ticks", desyncCount, len(ticks))
	}

	t.Logf("REPLAY OK: %d ticks, 0 desyncs", len(ticks))
}
