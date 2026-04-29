// Package wire defines the L3 game wire protocol messages and their binary codec.
// All messages are identified by a 1-byte type ID and use varint/fixed encoding.
package wire

// MsgType identifies a wire protocol message.
type MsgType uint8

const (
	MsgHello        MsgType = 1
	MsgHelloAck     MsgType = 2
	MsgJoinRoom     MsgType = 3
	MsgJoinAck      MsgType = 4
	MsgCmd          MsgType = 10
	MsgFrameBundle  MsgType = 11
	MsgHashAck      MsgType = 12
	MsgRTTReport    MsgType = 13
	MsgNPub         MsgType = 14
	MsgFeedbackHint MsgType = 15
	MsgResume       MsgType = 20
	MsgResync       MsgType = 21
	MsgBye          MsgType = 30
	MsgGameOver     MsgType = 31
)

// GameOver is broadcast by the server when the match ends.
type GameOver struct {
	Results []PlayerResult
}

// PlayerResult encodes one player's outcome.
type PlayerResult struct {
	PlayerID uint8
	Result   uint8 // 0=Ongoing, 1=Victory, 2=Defeat, 3=Draw
}

// Hello is sent by the client on connection to negotiate protocol version.
type Hello struct {
	ProtocolVersion uint16
	PlayerName      string // max 32 bytes
}

// HelloAck is the server's response to Hello.
type HelloAck struct {
	ProtocolVersion uint16
	ServerTickRate  uint16
	Accepted        bool
}

// JoinRoom is sent by the client to join/create a room.
type JoinRoom struct {
	RoomID string // max 32 bytes, empty = create new
}

// JoinAck is the server's response to JoinRoom.
type JoinAck struct {
	RoomID   string
	PlayerID uint8
	Seed     uint64
	MapW     int32
	MapH     int32
	Accepted bool
}

// Cmd is a player command to be executed at a specific tick.
type Cmd struct {
	Tick      uint32
	Player    uint8
	Op        uint8  // maps to sim.CmdOp
	UnitID    uint32
	TargetX   int32 // fixed-point raw
	TargetY   int32
	TargetID  uint32
}

// FrameBundle is broadcast by the server: all commands sealed for a tick.
type FrameBundle struct {
	Tick     uint32
	NCurrent uint8 // current adaptive N
	Cmds     []Cmd
}

// HashAck is sent by the client after executing a tick.
type HashAck struct {
	Tick uint32
	Hash uint64
}

// RTTReport carries client-measured RTT samples.
type RTTReport struct {
	Samples [3]uint16 // in milliseconds
}

// NPub announces a change in input delay N.
type NPub struct {
	EffectiveFromTick uint32
	N                 uint8
}

// FeedbackHint is a non-lockstep immediate feedback forwarded by server.
type FeedbackHint struct {
	Tick   uint32
	Player uint8
	HintID uint16
}

// Resume is sent by a reconnecting client.
type Resume struct {
	ConnID           uint16
	LastExecutedTick uint32
	Token            [16]byte
}

// Resync is the server's response to Resume.
type Resync struct {
	HasSnapshot bool
	Snapshot    []byte       // only if HasSnapshot
	Frames      []FrameBundle // frames to catch up
}
