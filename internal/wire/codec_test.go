package wire

import (
	"testing"
)

func TestCmdRoundtrip(t *testing.T) {
	orig := &Cmd{Tick: 100, Player: 1, Op: 2, UnitID: 42, TargetX: -5000, TargetY: 3000, TargetID: 7}
	data, err := Encode(orig)
	if err != nil {
		t.Fatal(err)
	}
	msgType, decoded, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if msgType != MsgCmd {
		t.Fatalf("type = %d, want %d", msgType, MsgCmd)
	}
	got := decoded.(*Cmd)
	if *got != *orig {
		t.Fatalf("mismatch: got %+v, want %+v", got, orig)
	}
}

func TestFrameBundleRoundtrip(t *testing.T) {
	orig := &FrameBundle{
		Tick:     200,
		NCurrent: 3,
		Cmds: []Cmd{
			{Tick: 200, Player: 0, Op: 1, UnitID: 1, TargetX: 100, TargetY: 200},
			{Tick: 200, Player: 1, Op: 2, UnitID: 5, TargetID: 1},
		},
	}
	data, err := Encode(orig)
	if err != nil {
		t.Fatal(err)
	}
	msgType, decoded, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if msgType != MsgFrameBundle {
		t.Fatalf("type = %d, want %d", msgType, MsgFrameBundle)
	}
	got := decoded.(*FrameBundle)
	if got.Tick != orig.Tick || got.NCurrent != orig.NCurrent || len(got.Cmds) != len(orig.Cmds) {
		t.Fatalf("header mismatch: got %+v", got)
	}
	for i := range got.Cmds {
		if got.Cmds[i] != orig.Cmds[i] {
			t.Fatalf("cmd %d mismatch: got %+v, want %+v", i, got.Cmds[i], orig.Cmds[i])
		}
	}
}

func TestHelloRoundtrip(t *testing.T) {
	orig := &Hello{ProtocolVersion: 1, PlayerName: "TestPlayer"}
	data, _ := Encode(orig)
	_, decoded, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	got := decoded.(*Hello)
	if got.ProtocolVersion != orig.ProtocolVersion || got.PlayerName != orig.PlayerName {
		t.Fatalf("mismatch: got %+v", got)
	}
}

func TestHashAckRoundtrip(t *testing.T) {
	orig := &HashAck{Tick: 500, Hash: 0xDEADBEEFCAFEBABE}
	data, _ := Encode(orig)
	_, decoded, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	got := decoded.(*HashAck)
	if *got != *orig {
		t.Fatalf("mismatch: got %+v, want %+v", got, orig)
	}
}

func TestJoinAckRoundtrip(t *testing.T) {
	orig := &JoinAck{
		RoomID: "room-42", PlayerID: 1, Seed: 12345, MapW: 100, MapH: 100, Accepted: true,
	}
	data, _ := Encode(orig)
	_, decoded, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	got := decoded.(*JoinAck)
	if got.RoomID != orig.RoomID || got.PlayerID != orig.PlayerID || got.Seed != orig.Seed ||
		got.MapW != orig.MapW || got.MapH != orig.MapH || got.Accepted != orig.Accepted {
		t.Fatalf("mismatch: got %+v, want %+v", got, orig)
	}
}

func TestAllMessageTypes(t *testing.T) {
	messages := []interface{}{
		&Hello{ProtocolVersion: 1, PlayerName: "P"},
		&HelloAck{ProtocolVersion: 1, ServerTickRate: 20, Accepted: true},
		&JoinRoom{RoomID: "r1"},
		&JoinAck{RoomID: "r1", PlayerID: 0, Seed: 1, MapW: 50, MapH: 50, Accepted: true},
		&Cmd{Tick: 10, Player: 0, Op: 1, UnitID: 1},
		&FrameBundle{Tick: 10, NCurrent: 3, Cmds: []Cmd{{Tick: 10, Player: 0, Op: 1, UnitID: 1}}},
		&HashAck{Tick: 10, Hash: 42},
		&RTTReport{Samples: [3]uint16{50, 55, 48}},
		&NPub{EffectiveFromTick: 100, N: 2},
		&FeedbackHint{Tick: 10, Player: 0, HintID: 7},
		&Resume{ConnID: 5, LastExecutedTick: 100, Token: [16]byte{1, 2, 3}},
	}
	for _, msg := range messages {
		data, err := Encode(msg)
		if err != nil {
			t.Fatalf("encode %T: %v", msg, err)
		}
		_, _, err = Decode(data)
		if err != nil {
			t.Fatalf("decode %T: %v (data len=%d)", msg, err, len(data))
		}
	}
}
