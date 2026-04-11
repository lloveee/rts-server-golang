package lockstep

import (
	"rts/internal/sim"
	"rts/internal/transport"
	"rts/internal/wire"
	"time"
)

const (
	// LimboTimeout is how long a disconnected player's slot is preserved.
	LimboTimeout = 30 * time.Second
	// MaxIncrementalFrames is the threshold beyond which we send a full snapshot.
	MaxIncrementalFrames = 100
)

// limboSlot tracks a disconnected player awaiting reconnect.
type limboSlot struct {
	playerID    uint8
	disconnAt   time.Time
	resumeToken [16]byte
}

// HandleDisconnect puts a player into limbo state.
// The room continues ticking; the player's slot is held for LimboTimeout.
func (r *Room) HandleDisconnect(playerID uint8) {
	for i, p := range r.players {
		if p != nil && p.PlayerID == playerID {
			r.players[i].Conn = nil
			r.players[i].Joined = false

			r.limbo = append(r.limbo, limboSlot{
				playerID:  playerID,
				disconnAt: time.Now(),
			})

			r.log.Info("player disconnected, entering limbo",
				"player_id", playerID,
				"limbo_timeout", LimboTimeout)
			return
		}
	}
}

// HandleResume attempts to reconnect a player.
// Returns the Resync message to send, or nil if resume is rejected.
func (r *Room) HandleResume(resume *wire.Resume, conn *transport.Conn) *wire.Resync {
	// Find limbo slot.
	limboIdx := -1
	for i, l := range r.limbo {
		if l.playerID == uint8(resume.ConnID) { // match by player ID encoded in ConnID
			limboIdx = i
			break
		}
	}

	if limboIdx == -1 {
		return nil // no limbo slot found
	}

	ls := r.limbo[limboIdx]
	// Remove from limbo.
	r.limbo = append(r.limbo[:limboIdx], r.limbo[limboIdx+1:]...)

	// Restore connection.
	for i, p := range r.players {
		if p != nil && p.PlayerID == ls.playerID {
			r.players[i].Conn = conn
			r.players[i].Joined = true
			break
		}
	}

	// Determine how far behind.
	lastExec := resume.LastExecutedTick
	behind := int(r.tick - lastExec)

	resync := &wire.Resync{}

	if behind > MaxIncrementalFrames || behind < 0 {
		// Too far behind: send full snapshot + recent frames.
		resync.HasSnapshot = true
		resync.Snapshot = sim.Marshal(r.world)
		// No frames needed after snapshot — client starts from current state.
	} else {
		// Incremental: send missed frames.
		resync.HasSnapshot = false
		resync.Frames = r.FrameHistory(lastExec)
	}

	r.log.Info("player resumed",
		"player_id", ls.playerID,
		"behind_ticks", behind,
		"snapshot", resync.HasSnapshot,
		"frames", len(resync.Frames))

	return resync
}

// CleanupLimbo removes expired limbo slots.
// Called periodically from the tick loop.
func (r *Room) CleanupLimbo() {
	now := time.Now()
	alive := r.limbo[:0]
	for _, l := range r.limbo {
		if now.Sub(l.disconnAt) < LimboTimeout {
			alive = append(alive, l)
		} else {
			r.log.Info("limbo expired", "player_id", l.playerID)
		}
	}
	r.limbo = alive
}
