// Package rtsclient provides a Go client SDK for connecting to the RTS server.
// Used by fake-client, load-test, e2e tests, and (later) real game clients.
package rtsclient

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"rts/internal/transport"
	"rts/internal/wire"
	"sync"
	"time"
)

// Client is a lockstep RTS client that connects to a server via reliable UDP.
type Client struct {
	conn       *transport.Conn
	udpConn    *net.UDPConn
	serverAddr *net.UDPAddr
	log        *slog.Logger

	// Session state
	mu         sync.Mutex
	playerID   uint8
	roomID     string
	seed       uint64
	mapW, mapH int32
	joined     bool
	connID     uint16

	// Callbacks (set before Connect)
	OnFrame    func(fb *wire.FrameBundle)
	OnNPub     func(np *wire.NPub)
	OnHint     func(fh *wire.FeedbackHint)

	closed chan struct{}
}

// Config holds client configuration.
type Config struct {
	ServerAddr string
	PlayerName string
	RoomID     string // empty = create new room
	Logger     *slog.Logger
	Chaos      transport.ChaosHook
}

// New creates a client (does not connect yet).
func New(cfg Config) (*Client, error) {
	serverAddr, err := net.ResolveUDPAddr("udp", cfg.ServerAddr)
	if err != nil {
		return nil, fmt.Errorf("resolve: %w", err)
	}
	udpConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	c := &Client{
		serverAddr: serverAddr,
		udpConn:    udpConn,
		log:        cfg.Logger,
		closed:     make(chan struct{}),
	}

	c.conn = transport.NewConn(transport.ConnConfig{
		ConnID:     0, // will be assigned by server
		RemoteAddr: serverAddr,
		SendFunc: func(data []byte, addr *net.UDPAddr) error {
			_, err := udpConn.WriteToUDP(data, addr)
			return err
		},
		Chaos:     cfg.Chaos,
		InboxSize: 512,
	})

	return c, nil
}

// Connect performs the handshake: SYN → SYN-ACK → Hello → HelloAck → JoinRoom → JoinAck.
func (c *Client) Connect(playerName, roomID string) error {
	// Send SYN.
	synPkt := &transport.Packet{
		Flags: transport.FlagSYN,
	}
	if err := c.rawSend(synPkt.Encode()); err != nil {
		return fmt.Errorf("send SYN: %w", err)
	}

	// Wait for SYN-ACK.
	synAck, err := c.readPacketWithTimeout(2 * time.Second)
	if err != nil {
		return fmt.Errorf("SYN-ACK: %w", err)
	}
	if !synAck.IsSYN() || !synAck.IsACK() {
		return errors.New("expected SYN-ACK")
	}
	c.connID = synAck.ConnID
	c.conn.ConnID = synAck.ConnID
	c.conn.SetState(transport.StateEstablished)

	// Send Hello.
	hello := &wire.Hello{ProtocolVersion: wire.ProtocolVersion, PlayerName: playerName}
	helloData, _ := wire.Encode(hello)
	if err := c.conn.Send(helloData); err != nil {
		return fmt.Errorf("send Hello: %w", err)
	}

	// Read HelloAck.
	helloAckData, err := c.readInboxWithTimeout(2 * time.Second)
	if err != nil {
		return fmt.Errorf("HelloAck: %w", err)
	}
	_, helloAckMsg, err := wire.Decode(helloAckData)
	if err != nil {
		return fmt.Errorf("decode HelloAck: %w", err)
	}
	helloAck, ok := helloAckMsg.(*wire.HelloAck)
	if !ok || !helloAck.Accepted {
		return errors.New("HelloAck rejected")
	}

	// Send JoinRoom.
	join := &wire.JoinRoom{RoomID: roomID}
	joinData, _ := wire.Encode(join)
	if err := c.conn.Send(joinData); err != nil {
		return fmt.Errorf("send JoinRoom: %w", err)
	}

	// Read JoinAck.
	joinAckData, err := c.readInboxWithTimeout(2 * time.Second)
	if err != nil {
		return fmt.Errorf("JoinAck: %w", err)
	}
	_, joinAckMsg, err := wire.Decode(joinAckData)
	if err != nil {
		return fmt.Errorf("decode JoinAck: %w", err)
	}
	joinAck, ok := joinAckMsg.(*wire.JoinAck)
	if !ok || !joinAck.Accepted {
		return errors.New("JoinAck rejected")
	}

	c.mu.Lock()
	c.playerID = joinAck.PlayerID
	c.roomID = joinAck.RoomID
	c.seed = joinAck.Seed
	c.mapW = joinAck.MapW
	c.mapH = joinAck.MapH
	c.joined = true
	c.mu.Unlock()

	c.log.Info("connected", "conn_id", c.connID, "player_id", c.playerID, "room", c.roomID)
	return nil
}

// PlayerID returns the assigned player ID.
func (c *Client) PlayerID() uint8 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.playerID
}

// Seed returns the room seed.
func (c *Client) Seed() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.seed
}

// MapSize returns the map dimensions.
func (c *Client) MapSize() (int32, int32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.mapW, c.mapH
}

// SendCmd sends a game command to the server.
func (c *Client) SendCmd(cmd *wire.Cmd) error {
	data, err := wire.Encode(cmd)
	if err != nil {
		return err
	}
	return c.conn.Send(data)
}

// SendHashAck sends a state hash acknowledgment.
func (c *Client) SendHashAck(tick uint32, hash uint64) error {
	data, _ := wire.Encode(&wire.HashAck{Tick: tick, Hash: hash})
	return c.conn.Send(data)
}

// SendRTTReport sends an RTT report.
func (c *Client) SendRTTReport(samples [3]uint16) error {
	data, _ := wire.Encode(&wire.RTTReport{Samples: samples})
	return c.conn.SendUnreliable(data)
}

// RunReadLoop starts the read loop (blocks). Call in a goroutine.
func (c *Client) RunReadLoop() {
	buf := make([]byte, 2048)
	for {
		select {
		case <-c.closed:
			return
		default:
		}
		c.udpConn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		n, _, err := c.udpConn.ReadFromUDP(buf)
		if err != nil {
			continue
		}
		pkt, err := transport.DecodePacket(buf[:n])
		if err != nil {
			continue
		}
		c.conn.HandleReceive(pkt)
	}
}

// RunDispatchLoop reads from conn.Inbox and dispatches to callbacks (blocks).
func (c *Client) RunDispatchLoop() {
	for {
		select {
		case <-c.closed:
			return
		case data := <-c.conn.Inbox:
			c.dispatch(data)
		}
	}
}

// RunRetxLoop periodically ticks retransmission (blocks).
func (c *Client) RunRetxLoop() {
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-c.closed:
			return
		case <-ticker.C:
			c.conn.Tick(time.Now())
		}
	}
}

// Close shuts down the client.
func (c *Client) Close() {
	select {
	case <-c.closed:
	default:
		close(c.closed)
	}
	c.udpConn.Close()
}

func (c *Client) dispatch(data []byte) {
	msgType, msg, err := wire.Decode(data)
	if err != nil {
		c.log.Warn("decode error", "err", err)
		return
	}
	switch msgType {
	case wire.MsgFrameBundle:
		if c.OnFrame != nil {
			c.OnFrame(msg.(*wire.FrameBundle))
		}
	case wire.MsgNPub:
		if c.OnNPub != nil {
			c.OnNPub(msg.(*wire.NPub))
		}
	case wire.MsgFeedbackHint:
		if c.OnHint != nil {
			c.OnHint(msg.(*wire.FeedbackHint))
		}
	}
}

func (c *Client) rawSend(data []byte) error {
	_, err := c.udpConn.WriteToUDP(data, c.serverAddr)
	return err
}

func (c *Client) readPacketWithTimeout(timeout time.Duration) (*transport.Packet, error) {
	buf := make([]byte, 2048)
	c.udpConn.SetReadDeadline(time.Now().Add(timeout))
	n, _, err := c.udpConn.ReadFromUDP(buf)
	if err != nil {
		return nil, err
	}
	return transport.DecodePacket(buf[:n])
}

func (c *Client) readInboxWithTimeout(timeout time.Duration) ([]byte, error) {
	// Pump the UDP socket until we get something in the inbox.
	deadline := time.Now().Add(timeout)
	buf := make([]byte, 2048)
	for time.Now().Before(deadline) {
		c.udpConn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
		n, _, err := c.udpConn.ReadFromUDP(buf)
		if err != nil {
			continue
		}
		pkt, err := transport.DecodePacket(buf[:n])
		if err != nil {
			continue
		}
		c.conn.HandleReceive(pkt)

		select {
		case data := <-c.conn.Inbox:
			return data, nil
		default:
		}
	}
	return nil, errors.New("timeout waiting for inbox message")
}
