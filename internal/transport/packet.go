// Package transport implements a minimal Reliable UDP layer for lockstep RTS.
//
// Features: sequence numbers, selective ACK via bitmask, retransmission,
// in-order delivery, connection lifecycle (SYN/FIN), NAT keepalive (PING).
// Explicitly NOT implemented: congestion control (RTS traffic is small and predictable).
package transport

import (
	"encoding/binary"
	"errors"
)

// Magic bytes identifying our protocol: 'R' '1'.
const (
	MagicByte0 byte = 0x52 // 'R'
	MagicByte1 byte = 0x31 // '1'
)

// Flag bits in the flags byte.
const (
	FlagSYN      byte = 1 << 0
	FlagACK      byte = 1 << 1
	FlagFIN      byte = 1 << 2
	FlagReliable byte = 1 << 3
	FlagPING     byte = 1 << 4
)

// HeaderSize is the fixed-size header before payload.
// magic(2) + flags(1) + conn_id(2) + seq(4) + ack(4) + ack_bitmask(4) + payload_len(2) = 19
const HeaderSize = 19

// MaxPayloadSize is the maximum payload in a single packet (stay under typical MTU).
const MaxPayloadSize = 1200

// Packet is the wire-level unit of the reliable UDP protocol.
type Packet struct {
	Flags      byte
	ConnID     uint16
	Seq        uint32
	Ack        uint32
	AckBitmask uint32
	Payload    []byte
}

// Encode serializes a packet into a byte slice.
func (p *Packet) Encode() []byte {
	buf := make([]byte, HeaderSize+len(p.Payload))
	buf[0] = MagicByte0
	buf[1] = MagicByte1
	buf[2] = p.Flags
	binary.LittleEndian.PutUint16(buf[3:5], p.ConnID)
	binary.LittleEndian.PutUint32(buf[5:9], p.Seq)
	binary.LittleEndian.PutUint32(buf[9:13], p.Ack)
	binary.LittleEndian.PutUint32(buf[13:17], p.AckBitmask)
	binary.LittleEndian.PutUint16(buf[17:19], uint16(len(p.Payload)))
	copy(buf[19:], p.Payload)
	return buf
}

// DecodePacket deserializes a packet from bytes.
func DecodePacket(data []byte) (*Packet, error) {
	if len(data) < HeaderSize {
		return nil, errors.New("packet too short")
	}
	if data[0] != MagicByte0 || data[1] != MagicByte1 {
		return nil, errors.New("bad magic")
	}

	payloadLen := binary.LittleEndian.Uint16(data[17:19])
	if int(payloadLen) > len(data)-HeaderSize {
		return nil, errors.New("payload length exceeds packet size")
	}

	p := &Packet{
		Flags:      data[2],
		ConnID:     binary.LittleEndian.Uint16(data[3:5]),
		Seq:        binary.LittleEndian.Uint32(data[5:9]),
		Ack:        binary.LittleEndian.Uint32(data[9:13]),
		AckBitmask: binary.LittleEndian.Uint32(data[13:17]),
	}
	if payloadLen > 0 {
		p.Payload = make([]byte, payloadLen)
		copy(p.Payload, data[HeaderSize:HeaderSize+int(payloadLen)])
	}
	return p, nil
}

// IsSYN returns true if the SYN flag is set.
func (p *Packet) IsSYN() bool { return p.Flags&FlagSYN != 0 }

// IsACK returns true if the ACK flag is set.
func (p *Packet) IsACK() bool { return p.Flags&FlagACK != 0 }

// IsFIN returns true if the FIN flag is set.
func (p *Packet) IsFIN() bool { return p.Flags&FlagFIN != 0 }

// IsReliable returns true if the Reliable flag is set.
func (p *Packet) IsReliable() bool { return p.Flags&FlagReliable != 0 }

// IsPING returns true if the PING flag is set.
func (p *Packet) IsPING() bool { return p.Flags&FlagPING != 0 }
