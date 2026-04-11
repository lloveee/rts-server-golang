package lockstep

import (
	"context"
	"log/slog"
	"rts/internal/replay"
	"rts/internal/sim"
	"rts/internal/sim/fixed"
	"rts/internal/transport"
	"rts/internal/wire"
	"sync"
	"time"
)

// RoomConfig holds parameters for creating a room.
type RoomConfig struct {
	RoomID      string
	Seed        uint64
	MapW, MapH  int32
	TickRate    int // Hz
	InitialN    uint8
	MinN, MaxN  uint8
	PlayerCount int
	Logger      *slog.Logger
}

// PlayerSlot represents a connected player in the room.
type PlayerSlot struct {
	PlayerID uint8
	Conn     *transport.Conn
	Joined   bool
}

// Room is the lockstep room actor. All mutable state lives here.
// Exactly one goroutine runs the room — zero locks needed inside.
type Room struct {
	cfg       RoomConfig
	world     *sim.World
	inputBuf  *InputBuf
	hashAgg   *HashAgg
	adaptiveN *AdaptiveN
	log       *slog.Logger

	players   []*PlayerSlot
	tick      uint32
	tickInterval time.Duration

	// Inbox for messages from connection reader goroutines.
	Inbox chan RoomMsg

	// Frame history for reconnect (ring buffer of last N frames).
	frameHistory []wire.FrameBundle
	maxHistory   int

	// Limbo slots for disconnected players awaiting reconnect.
	limbo []limboSlot

	// Optional replay writer. If non-nil, every tick is recorded.
	replayWriter *replay.Writer

	mu sync.Mutex // only for external reads (e.g., status query)
}

// RoomMsg is a message delivered to the room actor via Inbox.
type RoomMsg struct {
	PlayerID uint8
	Type     wire.MsgType
	Data     interface{} // the decoded wire message
}

// NewRoom creates a room but does not start it.
func NewRoom(cfg RoomConfig) *Room {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	w := sim.NewWorld(cfg.Seed, cfg.MapW, cfg.MapH)

	r := &Room{
		cfg:          cfg,
		world:        w,
		inputBuf:     NewInputBuf(),
		hashAgg:      NewHashAgg(100, cfg.PlayerCount),
		adaptiveN:    NewAdaptiveN(cfg.InitialN, cfg.MinN, cfg.MaxN, cfg.TickRate),
		log:          cfg.Logger.With("room", cfg.RoomID),
		players:      make([]*PlayerSlot, cfg.PlayerCount),
		tickInterval: time.Second / time.Duration(cfg.TickRate),
		Inbox:        make(chan RoomMsg, 1024),
		maxHistory:   200,
	}
	r.frameHistory = make([]wire.FrameBundle, 0, r.maxHistory)
	return r
}

// AddPlayer assigns a connection to a player slot. Returns the player ID.
func (r *Room) AddPlayer(conn *transport.Conn) (uint8, bool) {
	for i := range r.players {
		if r.players[i] == nil {
			r.players[i] = &PlayerSlot{
				PlayerID: uint8(i),
				Conn:     conn,
				Joined:   true,
			}
			return uint8(i), true
		}
	}
	return 0, false
}

// SpawnInitialUnits creates starting units for each player.
func (r *Room) SpawnInitialUnits(unitsPerPlayer int) {
	for pid := 0; pid < r.cfg.PlayerCount; pid++ {
		for i := 0; i < unitsPerPlayer; i++ {
			x := fixed.FromInt(int32(10 + pid*80))
			y := fixed.FromInt(int32(10 + i*3))
			r.world.SpawnUnit(uint8(pid), fixed.V(x, y),
				fixed.FromInt(10), fixed.FromFloat64(0.5))
		}
	}
}

// SetReplayWriter attaches a replay writer. Must be called before Run.
// The caller is responsible for writing the header before calling this.
func (r *Room) SetReplayWriter(rw *replay.Writer) {
	r.replayWriter = rw
}

// Run starts the room tick loop. Blocks until context is cancelled.
func (r *Room) Run(ctx context.Context) {
	r.log.Info("room started", "tick_rate", r.cfg.TickRate, "seed", r.cfg.Seed)

	startTime := time.Now()

	for {
		// Calculate next tick absolute time (avoid drift).
		nextTick := startTime.Add(time.Duration(r.tick+1) * r.tickInterval)
		sleepDur := time.Until(nextTick)
		if sleepDur > 0 {
			timer := time.NewTimer(sleepDur)
			select {
			case <-ctx.Done():
				timer.Stop()
				r.log.Info("room stopped", "tick", r.tick)
				return
			case msg := <-r.Inbox:
				timer.Stop()
				r.handleMsg(msg)
				// Drain remaining messages before tick.
				r.drainInbox()
				continue
			case <-timer.C:
			}
		}

		// Drain any messages that arrived during sleep.
		r.drainInbox()

		// Seal and broadcast tick.
		r.sealTick()
	}
}

func (r *Room) drainInbox() {
	for {
		select {
		case msg := <-r.Inbox:
			r.handleMsg(msg)
		default:
			return
		}
	}
}

func (r *Room) handleMsg(msg RoomMsg) {
	switch msg.Type {
	case wire.MsgCmd:
		cmd := msg.Data.(*wire.Cmd)
		r.inputBuf.Add(*cmd)
	case wire.MsgHashAck:
		ha := msg.Data.(*wire.HashAck)
		state := r.hashAgg.Report(ha.Tick, msg.PlayerID, ha.Hash)
		if state == HashDesync {
			r.log.Error("DESYNC DETECTED", "tick", ha.Tick, "player", msg.PlayerID)
		}
	case wire.MsgResume:
		resume := msg.Data.(*wire.Resume)
		// Resume is handled specially: we need to find the conn.
		// For now, log it — the server connection handler will manage Resume.
		r.log.Info("resume request received", "player", msg.PlayerID, "last_tick", resume.LastExecutedTick)
	case wire.MsgRTTReport:
		rr := msg.Data.(*wire.RTTReport)
		// Use median of 3 samples.
		samples := rr.Samples
		if samples[0] > samples[1] {
			samples[0], samples[1] = samples[1], samples[0]
		}
		if samples[1] > samples[2] {
			samples[1], samples[2] = samples[2], samples[1]
		}
		if samples[0] > samples[1] {
			samples[0], samples[1] = samples[1], samples[0]
		}
		rtt := time.Duration(samples[1]) * time.Millisecond
		r.adaptiveN.ReportRTT(msg.PlayerID, rtt)
	}
}

func (r *Room) sealTick() {
	r.tick++

	// Seal commands for this tick.
	cmds := r.inputBuf.Seal(r.tick)

	// Create frame bundle.
	fb := wire.FrameBundle{
		Tick:     r.tick,
		NCurrent: r.adaptiveN.Current(),
		Cmds:     cmds,
	}

	// Store in history for reconnect.
	if len(r.frameHistory) >= r.maxHistory {
		// Shift left (drop oldest).
		copy(r.frameHistory, r.frameHistory[1:])
		r.frameHistory = r.frameHistory[:len(r.frameHistory)-1]
	}
	r.frameHistory = append(r.frameHistory, fb)

	// Create hash slot.
	r.hashAgg.CreateSlot(r.tick)

	// Check hash timeouts.
	r.hashAgg.CheckTimeouts(r.tick, 60) // 60 ticks = 3 seconds

	// Broadcast frame to all connected players.
	data, _ := wire.Encode(&fb)
	for _, p := range r.players {
		if p == nil || !p.Joined || p.Conn == nil {
			continue
		}
		_ = p.Conn.Send(data)
	}

	// Periodically cleanup limbo and recalculate adaptive N (every 20 ticks = 1 second).
	if r.tick%20 == 0 {
		r.CleanupLimbo()
		if newN, changed := r.adaptiveN.Recalculate(); changed {
			npub := &wire.NPub{
				EffectiveFromTick: r.tick + uint32(newN)*2,
				N:                 newN,
			}
			npubData, _ := wire.Encode(npub)
			for _, p := range r.players {
				if p == nil || !p.Joined || p.Conn == nil {
					continue
				}
				_ = p.Conn.Send(npubData)
			}
			r.log.Info("adaptive N changed", "new_n", newN, "effective_tick", npub.EffectiveFromTick)
		}
	}

	// Execute simulation step (for hash verification — server doesn't use the result
	// but we run it to validate correctness against clients).
	simCmds := wireCmdsToSimCmds(cmds)
	sim.Step(r.world, simCmds)

	// Record to replay if writer is attached.
	if r.replayWriter != nil {
		hash := sim.Hash(r.world)
		if err := r.replayWriter.WriteTick(r.tick, cmds, hash); err != nil {
			r.log.Warn("replay write failed", "tick", r.tick, "err", err)
		}
	}
}

// Tick returns the current tick number.
func (r *Room) Tick() uint32 {
	return r.tick
}

// World returns the room's world (for snapshot/reconnect).
func (r *Room) World() *sim.World {
	return r.world
}

// FrameHistory returns recent frames for reconnect.
func (r *Room) FrameHistory(fromTick uint32) []wire.FrameBundle {
	var frames []wire.FrameBundle
	for _, fb := range r.frameHistory {
		if fb.Tick > fromTick {
			frames = append(frames, fb)
		}
	}
	return frames
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
