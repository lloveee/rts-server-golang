package wire

import (
	"encoding/binary"
	"errors"
)

// Encode serializes a message to bytes: [type(1)] [payload...].
func Encode(msg interface{}) ([]byte, error) {
	switch m := msg.(type) {
	case *Hello:
		return encodeHello(m), nil
	case *HelloAck:
		return encodeHelloAck(m), nil
	case *JoinRoom:
		return encodeJoinRoom(m), nil
	case *JoinAck:
		return encodeJoinAck(m), nil
	case *Cmd:
		return encodeCmd(m), nil
	case *FrameBundle:
		return encodeFrameBundle(m), nil
	case *HashAck:
		return encodeHashAck(m), nil
	case *RTTReport:
		return encodeRTTReport(m), nil
	case *NPub:
		return encodeNPub(m), nil
	case *FeedbackHint:
		return encodeFeedbackHint(m), nil
	case *Resume:
		return encodeResume(m), nil
	default:
		return nil, errors.New("unknown message type")
	}
}

// Decode deserializes a message from bytes.
func Decode(data []byte) (MsgType, interface{}, error) {
	if len(data) < 1 {
		return 0, nil, errors.New("empty message")
	}
	msgType := MsgType(data[0])
	payload := data[1:]

	switch msgType {
	case MsgHello:
		m, err := decodeHello(payload)
		return msgType, m, err
	case MsgHelloAck:
		m, err := decodeHelloAck(payload)
		return msgType, m, err
	case MsgJoinRoom:
		m, err := decodeJoinRoom(payload)
		return msgType, m, err
	case MsgJoinAck:
		m, err := decodeJoinAck(payload)
		return msgType, m, err
	case MsgCmd:
		m, err := decodeCmd(payload)
		return msgType, m, err
	case MsgFrameBundle:
		m, err := decodeFrameBundle(payload)
		return msgType, m, err
	case MsgHashAck:
		m, err := decodeHashAck(payload)
		return msgType, m, err
	case MsgRTTReport:
		m, err := decodeRTTReport(payload)
		return msgType, m, err
	case MsgNPub:
		m, err := decodeNPub(payload)
		return msgType, m, err
	case MsgFeedbackHint:
		m, err := decodeFeedbackHint(payload)
		return msgType, m, err
	case MsgResume:
		m, err := decodeResume(payload)
		return msgType, m, err
	default:
		return msgType, nil, errors.New("unknown message type")
	}
}

// --- Encoders ---

func encodeHello(m *Hello) []byte {
	name := truncStr(m.PlayerName, 32)
	buf := make([]byte, 1+2+1+len(name))
	buf[0] = byte(MsgHello)
	binary.LittleEndian.PutUint16(buf[1:3], m.ProtocolVersion)
	buf[3] = byte(len(name))
	copy(buf[4:], name)
	return buf
}

func encodeHelloAck(m *HelloAck) []byte {
	buf := make([]byte, 1+2+2+1)
	buf[0] = byte(MsgHelloAck)
	binary.LittleEndian.PutUint16(buf[1:3], m.ProtocolVersion)
	binary.LittleEndian.PutUint16(buf[3:5], m.ServerTickRate)
	if m.Accepted {
		buf[5] = 1
	}
	return buf
}

func encodeJoinRoom(m *JoinRoom) []byte {
	rid := truncStr(m.RoomID, 32)
	buf := make([]byte, 1+1+len(rid))
	buf[0] = byte(MsgJoinRoom)
	buf[1] = byte(len(rid))
	copy(buf[2:], rid)
	return buf
}

func encodeJoinAck(m *JoinAck) []byte {
	rid := truncStr(m.RoomID, 32)
	buf := make([]byte, 1+1+len(rid)+1+8+4+4+1)
	off := 0
	buf[off] = byte(MsgJoinAck)
	off++
	buf[off] = byte(len(rid))
	off++
	copy(buf[off:], rid)
	off += len(rid)
	buf[off] = m.PlayerID
	off++
	binary.LittleEndian.PutUint64(buf[off:], m.Seed)
	off += 8
	binary.LittleEndian.PutUint32(buf[off:], uint32(m.MapW))
	off += 4
	binary.LittleEndian.PutUint32(buf[off:], uint32(m.MapH))
	off += 4
	if m.Accepted {
		buf[off] = 1
	}
	return buf[:off+1]
}

const cmdSize = 1 + 4 + 1 + 1 + 4 + 4 + 4 + 4 // type + tick + player + op + unitid + tx + ty + targetid = 23

func encodeCmd(m *Cmd) []byte {
	buf := make([]byte, cmdSize)
	buf[0] = byte(MsgCmd)
	binary.LittleEndian.PutUint32(buf[1:5], m.Tick)
	buf[5] = m.Player
	buf[6] = m.Op
	binary.LittleEndian.PutUint32(buf[7:11], m.UnitID)
	binary.LittleEndian.PutUint32(buf[11:15], uint32(m.TargetX))
	binary.LittleEndian.PutUint32(buf[15:19], uint32(m.TargetY))
	binary.LittleEndian.PutUint32(buf[19:23], m.TargetID)
	return buf
}

// encodeCmdPayload encodes a Cmd WITHOUT the type prefix (for embedding in FrameBundle).
func encodeCmdPayload(m *Cmd) []byte {
	buf := make([]byte, cmdSize-1) // no type byte
	binary.LittleEndian.PutUint32(buf[0:4], m.Tick)
	buf[4] = m.Player
	buf[5] = m.Op
	binary.LittleEndian.PutUint32(buf[6:10], m.UnitID)
	binary.LittleEndian.PutUint32(buf[10:14], uint32(m.TargetX))
	binary.LittleEndian.PutUint32(buf[14:18], uint32(m.TargetY))
	binary.LittleEndian.PutUint32(buf[18:22], m.TargetID)
	return buf
}

func encodeFrameBundle(m *FrameBundle) []byte {
	// type(1) + tick(4) + n_current(1) + cmd_count(2) + [cmd_count × (cmdSize-1)]
	cmdPayloadSize := cmdSize - 1
	buf := make([]byte, 1+4+1+2+len(m.Cmds)*cmdPayloadSize)
	buf[0] = byte(MsgFrameBundle)
	binary.LittleEndian.PutUint32(buf[1:5], m.Tick)
	buf[5] = m.NCurrent
	binary.LittleEndian.PutUint16(buf[6:8], uint16(len(m.Cmds)))
	off := 8
	for i := range m.Cmds {
		copy(buf[off:], encodeCmdPayload(&m.Cmds[i]))
		off += cmdPayloadSize
	}
	return buf[:off]
}

func encodeHashAck(m *HashAck) []byte {
	buf := make([]byte, 1+4+8)
	buf[0] = byte(MsgHashAck)
	binary.LittleEndian.PutUint32(buf[1:5], m.Tick)
	binary.LittleEndian.PutUint64(buf[5:13], m.Hash)
	return buf
}

func encodeRTTReport(m *RTTReport) []byte {
	buf := make([]byte, 1+6)
	buf[0] = byte(MsgRTTReport)
	for i := 0; i < 3; i++ {
		binary.LittleEndian.PutUint16(buf[1+i*2:3+i*2], m.Samples[i])
	}
	return buf
}

func encodeNPub(m *NPub) []byte {
	buf := make([]byte, 1+4+1)
	buf[0] = byte(MsgNPub)
	binary.LittleEndian.PutUint32(buf[1:5], m.EffectiveFromTick)
	buf[5] = m.N
	return buf
}

func encodeFeedbackHint(m *FeedbackHint) []byte {
	buf := make([]byte, 1+4+1+2)
	buf[0] = byte(MsgFeedbackHint)
	binary.LittleEndian.PutUint32(buf[1:5], m.Tick)
	buf[5] = m.Player
	binary.LittleEndian.PutUint16(buf[6:8], m.HintID)
	return buf
}

func encodeResume(m *Resume) []byte {
	buf := make([]byte, 1+2+4+16)
	buf[0] = byte(MsgResume)
	binary.LittleEndian.PutUint16(buf[1:3], m.ConnID)
	binary.LittleEndian.PutUint32(buf[3:7], m.LastExecutedTick)
	copy(buf[7:23], m.Token[:])
	return buf
}

// --- Decoders ---

func decodeHello(data []byte) (*Hello, error) {
	if len(data) < 3 {
		return nil, errors.New("hello too short")
	}
	m := &Hello{
		ProtocolVersion: binary.LittleEndian.Uint16(data[0:2]),
	}
	nameLen := int(data[2])
	if len(data) < 3+nameLen {
		return nil, errors.New("hello name truncated")
	}
	m.PlayerName = string(data[3 : 3+nameLen])
	return m, nil
}

func decodeHelloAck(data []byte) (*HelloAck, error) {
	if len(data) < 5 {
		return nil, errors.New("helloack too short")
	}
	return &HelloAck{
		ProtocolVersion: binary.LittleEndian.Uint16(data[0:2]),
		ServerTickRate:  binary.LittleEndian.Uint16(data[2:4]),
		Accepted:        data[4] != 0,
	}, nil
}

func decodeJoinRoom(data []byte) (*JoinRoom, error) {
	if len(data) < 1 {
		return nil, errors.New("joinroom too short")
	}
	ridLen := int(data[0])
	if len(data) < 1+ridLen {
		return nil, errors.New("joinroom rid truncated")
	}
	return &JoinRoom{RoomID: string(data[1 : 1+ridLen])}, nil
}

func decodeJoinAck(data []byte) (*JoinAck, error) {
	if len(data) < 1 {
		return nil, errors.New("joinack too short")
	}
	ridLen := int(data[0])
	off := 1 + ridLen
	if len(data) < off+1+8+4+4+1 {
		return nil, errors.New("joinack too short")
	}
	m := &JoinAck{
		RoomID:   string(data[1 : 1+ridLen]),
		PlayerID: data[off],
	}
	off++
	m.Seed = binary.LittleEndian.Uint64(data[off:])
	off += 8
	m.MapW = int32(binary.LittleEndian.Uint32(data[off:]))
	off += 4
	m.MapH = int32(binary.LittleEndian.Uint32(data[off:]))
	off += 4
	m.Accepted = data[off] != 0
	return m, nil
}

func decodeCmd(data []byte) (*Cmd, error) {
	if len(data) < cmdSize-1 {
		return nil, errors.New("cmd too short")
	}
	return decodeCmdPayload(data)
}

func decodeCmdPayload(data []byte) (*Cmd, error) {
	if len(data) < 22 {
		return nil, errors.New("cmd payload too short")
	}
	return &Cmd{
		Tick:     binary.LittleEndian.Uint32(data[0:4]),
		Player:   data[4],
		Op:       data[5],
		UnitID:   binary.LittleEndian.Uint32(data[6:10]),
		TargetX:  int32(binary.LittleEndian.Uint32(data[10:14])),
		TargetY:  int32(binary.LittleEndian.Uint32(data[14:18])),
		TargetID: binary.LittleEndian.Uint32(data[18:22]),
	}, nil
}

func decodeFrameBundle(data []byte) (*FrameBundle, error) {
	if len(data) < 7 {
		return nil, errors.New("framebundle too short")
	}
	m := &FrameBundle{
		Tick:     binary.LittleEndian.Uint32(data[0:4]),
		NCurrent: data[4],
	}
	cmdCount := binary.LittleEndian.Uint16(data[5:7])
	cmdPayloadSize := cmdSize - 1
	off := 7
	m.Cmds = make([]Cmd, cmdCount)
	for i := 0; i < int(cmdCount); i++ {
		if off+cmdPayloadSize > len(data) {
			return nil, errors.New("framebundle cmds truncated")
		}
		cmd, err := decodeCmdPayload(data[off:])
		if err != nil {
			return nil, err
		}
		m.Cmds[i] = *cmd
		off += cmdPayloadSize
	}
	return m, nil
}

func decodeHashAck(data []byte) (*HashAck, error) {
	if len(data) < 12 {
		return nil, errors.New("hashack too short")
	}
	return &HashAck{
		Tick: binary.LittleEndian.Uint32(data[0:4]),
		Hash: binary.LittleEndian.Uint64(data[4:12]),
	}, nil
}

func decodeRTTReport(data []byte) (*RTTReport, error) {
	if len(data) < 6 {
		return nil, errors.New("rttreport too short")
	}
	m := &RTTReport{}
	for i := 0; i < 3; i++ {
		m.Samples[i] = binary.LittleEndian.Uint16(data[i*2 : i*2+2])
	}
	return m, nil
}

func decodeNPub(data []byte) (*NPub, error) {
	if len(data) < 5 {
		return nil, errors.New("npub too short")
	}
	return &NPub{
		EffectiveFromTick: binary.LittleEndian.Uint32(data[0:4]),
		N:                 data[4],
	}, nil
}

func decodeFeedbackHint(data []byte) (*FeedbackHint, error) {
	if len(data) < 7 {
		return nil, errors.New("feedbackhint too short")
	}
	return &FeedbackHint{
		Tick:   binary.LittleEndian.Uint32(data[0:4]),
		Player: data[4],
		HintID: binary.LittleEndian.Uint16(data[5:7]),
	}, nil
}

func decodeResume(data []byte) (*Resume, error) {
	if len(data) < 22 {
		return nil, errors.New("resume too short")
	}
	m := &Resume{
		ConnID:           binary.LittleEndian.Uint16(data[0:2]),
		LastExecutedTick: binary.LittleEndian.Uint32(data[2:6]),
	}
	copy(m.Token[:], data[6:22])
	return m, nil
}

func truncStr(s string, max int) string {
	if len(s) > max {
		return s[:max]
	}
	return s
}
