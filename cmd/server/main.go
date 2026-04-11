package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"rts/internal/lockstep"
	"rts/internal/obs"
	"rts/internal/room"
	"rts/internal/transport"
	"rts/internal/wire"
	"syscall"
	"time"
)

func main() {
	addr := flag.String("addr", ":9000", "UDP listen address")
	httpAddr := flag.String("http", ":9001", "HTTP address for /metrics and /trace")
	tickRate := flag.Int("tick-rate", 20, "simulation tick rate in Hz")
	initialN := flag.Int("initial-n", 3, "initial input delay N")
	playerCount := flag.Int("players", 2, "players per room")
	seed := flag.Uint64("seed", 42, "world seed for new rooms")
	mapW := flag.Int("map-w", 100, "map width")
	mapH := flag.Int("map-h", 100, "map height")
	logLevel := flag.String("log-level", "info", "log level: debug/info/warn/error")
	flag.Parse()

	// Setup logger.
	var level slog.Level
	switch *logLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))

	// Metrics registry.
	metrics := obs.DefaultRegistry

	// Pre-register all planned metrics.
	metrics.Counter("rts_udp_packets_in_total")
	metrics.Counter("rts_udp_packets_out_total")
	metrics.Counter("rts_udp_retransmit_total")
	metrics.Histogram("rts_udp_rtt_ms")
	metrics.Histogram("rts_tick_duration_ms")
	metrics.Counter("rts_tick_late_seal_total")
	metrics.Gauge("rts_input_delay_n")
	metrics.Gauge("rts_hash_pending_ticks")
	metrics.Counter("rts_desync_total")
	metrics.Gauge("rts_rooms_active")
	metrics.Counter("rts_room_reconnects_total")

	// Trace ring buffer.
	trace := obs.NewTraceRing(obs.DefaultTraceSize)

	// Room defaults.
	defaults := lockstep.RoomConfig{
		Seed:        *seed,
		MapW:        int32(*mapW),
		MapH:        int32(*mapH),
		TickRate:    *tickRate,
		InitialN:    uint8(*initialN),
		MinN:        2,
		MaxN:        6,
		PlayerCount: *playerCount,
	}

	registry := room.NewRegistry(defaults, log)

	// Start UDP listener.
	listener, err := transport.Listen(transport.ListenerConfig{
		Addr:   *addr,
		Logger: log,
	})
	if err != nil {
		log.Error("listen failed", "err", err)
		os.Exit(1)
	}
	defer listener.Close()

	log.Info("rts-server started", "addr", listener.LocalAddr(), "tick_rate", *tickRate, "http", *httpAddr)

	// Handle new connections.
	listener.OnNewConn = func(conn *transport.Conn) {
		metrics.Counter("rts_udp_packets_in_total").Inc()
		go handleConnection(conn, registry, defaults, log)
	}

	// Start retransmission ticker for all connections.
	go func() {
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			for _, c := range listener.Conns() {
				c.Tick(time.Now())
			}
		}
	}()

	// HTTP endpoints for observability.
	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		// Update dynamic gauges.
		metrics.Gauge("rts_rooms_active").Set(int64(registry.Count()))
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		metrics.WriteTo(w)
	})
	mux.HandleFunc("/trace", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		trace.WriteTo(w)
	})
	go func() {
		if err := http.ListenAndServe(*httpAddr, mux); err != nil {
			log.Error("http server error", "err", err)
		}
	}()

	// Graceful shutdown.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := listener.Serve(); err != nil {
			log.Error("serve error", "err", err)
		}
	}()

	<-sigCh
	fmt.Println("\nshutting down...")

	// Suppress unused variable warnings for trace (used by HTTP handler).
	_ = trace
}

func handleConnection(conn *transport.Conn, registry *room.Registry, defaults lockstep.RoomConfig, log *slog.Logger) {
	// Read Hello.
	helloData, ok := readWithTimeout(conn, 5*time.Second)
	if !ok {
		return
	}
	msgType, msg, err := wire.Decode(helloData)
	if err != nil || msgType != wire.MsgHello {
		return
	}
	hello := msg.(*wire.Hello)
	log.Debug("hello received", "player", hello.PlayerName, "proto", hello.ProtocolVersion)

	// Send HelloAck.
	ack := &wire.HelloAck{
		ProtocolVersion: wire.ProtocolVersion,
		ServerTickRate:  uint16(defaults.TickRate),
		Accepted:        hello.ProtocolVersion == wire.ProtocolVersion,
	}
	ackData, _ := wire.Encode(ack)
	conn.Send(ackData)

	if !ack.Accepted {
		return
	}

	// Read JoinRoom.
	joinData, ok := readWithTimeout(conn, 5*time.Second)
	if !ok {
		return
	}
	msgType, msg, err = wire.Decode(joinData)
	if err != nil || msgType != wire.MsgJoinRoom {
		return
	}
	joinRoom := msg.(*wire.JoinRoom)

	roomID := joinRoom.RoomID
	if roomID == "" {
		roomID = fmt.Sprintf("room-%d", time.Now().UnixNano()%10000)
	}

	rm := registry.GetOrCreate(roomID)
	playerID, ok := rm.AddPlayer(conn)
	if !ok {
		joinAck := &wire.JoinAck{RoomID: roomID, Accepted: false}
		data, _ := wire.Encode(joinAck)
		conn.Send(data)
		return
	}

	joinAck := &wire.JoinAck{
		RoomID:   roomID,
		PlayerID: playerID,
		Seed:     defaults.Seed,
		MapW:     defaults.MapW,
		MapH:     defaults.MapH,
		Accepted: true,
	}
	joinAckData, _ := wire.Encode(joinAck)
	conn.Send(joinAckData)

	log.Info("player joined", "player_id", playerID, "room", roomID, "conn_id", conn.ConnID)

	// Dispatch loop: read from conn.Inbox and forward to room.
	for {
		select {
		case data := <-conn.Inbox:
			msgType, msg, err := wire.Decode(data)
			if err != nil {
				continue
			}
			rm.Inbox <- lockstep.RoomMsg{
				PlayerID: playerID,
				Type:     msgType,
				Data:     msg,
			}
		}
	}
}

func readWithTimeout(conn *transport.Conn, timeout time.Duration) ([]byte, bool) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case data := <-conn.Inbox:
		return data, true
	case <-timer.C:
		return nil, false
	}
}
