package transport

import (
	"fmt"
	"net"
	"sync"
	"testing"
	"time"
)

// memPipe simulates a UDP link between two endpoints with optional chaos.
type memPipe struct {
	mu     sync.Mutex
	aToB   chan []byte
	bToA   chan []byte
}

func newMemPipe() *memPipe {
	return &memPipe{
		aToB: make(chan []byte, 1024),
		bToA: make(chan []byte, 1024),
	}
}

func TestPacketEncodeDecode(t *testing.T) {
	pkt := &Packet{
		Flags:      FlagReliable | FlagACK,
		ConnID:     42,
		Seq:        100,
		Ack:        99,
		AckBitmask: 0xFFFFFFFF,
		Payload:    []byte("hello lockstep"),
	}
	data := pkt.Encode()

	got, err := DecodePacket(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Flags != pkt.Flags || got.ConnID != pkt.ConnID || got.Seq != pkt.Seq ||
		got.Ack != pkt.Ack || got.AckBitmask != pkt.AckBitmask || string(got.Payload) != "hello lockstep" {
		t.Fatalf("roundtrip mismatch: got %+v", got)
	}
}

func TestReorderBuffer(t *testing.T) {
	rb := newReorderBuffer(1, 64)

	// Send packets out of order: 3, 1, 2
	d3 := rb.Insert(3, []byte("three"))
	if d3 != nil {
		t.Error("seq 3 should not deliver yet")
	}
	d1 := rb.Insert(1, []byte("one"))
	if len(d1) != 1 || string(d1[0]) != "one" {
		t.Error("seq 1 should deliver immediately")
	}
	d2 := rb.Insert(2, []byte("two"))
	if len(d2) != 2 || string(d2[0]) != "two" || string(d2[1]) != "three" {
		t.Errorf("seq 2 should flush 2 and 3, got %d items", len(d2))
	}
	if rb.NextExpected() != 4 {
		t.Errorf("next expected = %d, want 4", rb.NextExpected())
	}
}

func TestReorderDuplicate(t *testing.T) {
	rb := newReorderBuffer(1, 64)
	rb.Insert(1, []byte("one"))
	d := rb.Insert(1, []byte("one again"))
	if d != nil {
		t.Error("duplicate should be dropped")
	}
}

func TestRetxQueue(t *testing.T) {
	q := newRetxQueue()
	now := time.Now()

	pkt := &Packet{Seq: 1, Flags: FlagReliable}
	q.Add(pkt, now, 100*time.Millisecond)

	// Before RTO: nothing to retransmit (InitialRTO clamps to 200ms).
	retx, _ := q.CollectRetransmissions(now.Add(150 * time.Millisecond))
	if len(retx) != 0 {
		t.Error("should not retransmit before RTO (200ms)")
	}

	// After RTO: retransmit.
	retx, _ = q.CollectRetransmissions(now.Add(250 * time.Millisecond))
	if len(retx) != 1 {
		t.Errorf("should retransmit 1 packet, got %d", len(retx))
	}

	// Ack it.
	_, ok := q.Ack(1, now.Add(300*time.Millisecond))
	if !ok {
		t.Error("ack should succeed")
	}
	if q.PendingCount() != 0 {
		t.Error("queue should be empty after ack")
	}
}

func TestConnEndToEnd(t *testing.T) {
	// Two conns talking through real UDP loopback.
	addrA, _ := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	addrB, _ := net.ResolveUDPAddr("udp", "127.0.0.1:0")

	sockA, err := net.ListenUDP("udp", addrA)
	if err != nil {
		t.Fatal(err)
	}
	defer sockA.Close()
	sockB, err := net.ListenUDP("udp", addrB)
	if err != nil {
		t.Fatal(err)
	}
	defer sockB.Close()

	sendA := func(data []byte, addr *net.UDPAddr) error {
		_, err := sockA.WriteToUDP(data, addr)
		return err
	}
	sendB := func(data []byte, addr *net.UDPAddr) error {
		_, err := sockB.WriteToUDP(data, addr)
		return err
	}

	connA := NewConn(ConnConfig{
		ConnID:     1,
		RemoteAddr: sockB.LocalAddr().(*net.UDPAddr),
		SendFunc:   sendA,
		InboxSize:  64,
	})
	connA.SetState(StateEstablished)

	connB := NewConn(ConnConfig{
		ConnID:     1,
		RemoteAddr: sockA.LocalAddr().(*net.UDPAddr),
		SendFunc:   sendB,
		InboxSize:  64,
	})
	connB.SetState(StateEstablished)

	// Read loops (time-bounded).
	var wg sync.WaitGroup
	deadline := time.Now().Add(2 * time.Second)
	wg.Add(2)
	go func() {
		defer wg.Done()
		buf := make([]byte, 2048)
		for time.Now().Before(deadline) {
			sockA.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			n, _, err := sockA.ReadFromUDP(buf)
			if err != nil {
				continue
			}
			pkt, err := DecodePacket(buf[:n])
			if err != nil {
				continue
			}
			connA.HandleReceive(pkt)
		}
	}()
	go func() {
		defer wg.Done()
		buf := make([]byte, 2048)
		for time.Now().Before(deadline) {
			sockB.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			n, _, err := sockB.ReadFromUDP(buf)
			if err != nil {
				continue
			}
			pkt, err := DecodePacket(buf[:n])
			if err != nil {
				continue
			}
			connB.HandleReceive(pkt)
		}
	}()

	// A sends 10 messages to B.
	for i := 0; i < 10; i++ {
		msg := fmt.Sprintf("msg-%d", i)
		if err := connA.Send([]byte(msg)); err != nil {
			t.Fatalf("send: %v", err)
		}
		time.Sleep(5 * time.Millisecond)
	}

	wg.Wait()

	// Check B received all 10 in order.
	for i := 0; i < 10; i++ {
		select {
		case payload := <-connB.Inbox:
			expected := fmt.Sprintf("msg-%d", i)
			if string(payload) != expected {
				t.Errorf("msg %d: got %q, want %q", i, string(payload), expected)
			}
		default:
			t.Errorf("msg %d not received", i)
		}
	}
}

func TestConnWithChaos(t *testing.T) {
	// Same as above but with 10% packet loss.
	addrA, _ := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	addrB, _ := net.ResolveUDPAddr("udp", "127.0.0.1:0")

	sockA, _ := net.ListenUDP("udp", addrA)
	defer sockA.Close()
	sockB, _ := net.ListenUDP("udp", addrB)
	defer sockB.Close()

	chaos := NewChaosHook(ChaosConfig{DropRate: 0.10})

	sendA := func(data []byte, addr *net.UDPAddr) error {
		if chaos.ShouldDrop() {
			return nil // simulated drop
		}
		_, err := sockA.WriteToUDP(data, addr)
		return err
	}
	sendB := func(data []byte, addr *net.UDPAddr) error {
		_, err := sockB.WriteToUDP(data, addr)
		return err
	}

	connA := NewConn(ConnConfig{
		ConnID:     1,
		RemoteAddr: sockB.LocalAddr().(*net.UDPAddr),
		SendFunc:   sendA,
		InboxSize:  64,
	})
	connA.SetState(StateEstablished)

	connB := NewConn(ConnConfig{
		ConnID:     1,
		RemoteAddr: sockA.LocalAddr().(*net.UDPAddr),
		SendFunc:   sendB,
		InboxSize:  64,
	})
	connB.SetState(StateEstablished)

	var wg sync.WaitGroup
	chaosDeadline := time.Now().Add(3 * time.Second)

	// Reader for A (receives ACKs).
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 2048)
		for time.Now().Before(chaosDeadline) {
			sockA.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
			n, _, err := sockA.ReadFromUDP(buf)
			if err != nil {
				continue
			}
			pkt, _ := DecodePacket(buf[:n])
			if pkt != nil {
				connA.HandleReceive(pkt)
			}
		}
	}()

	// Reader for B (receives data).
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 2048)
		for time.Now().Before(chaosDeadline) {
			sockB.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
			n, _, err := sockB.ReadFromUDP(buf)
			if err != nil {
				continue
			}
			pkt, _ := DecodePacket(buf[:n])
			if pkt != nil {
				connB.HandleReceive(pkt)
			}
		}
	}()

	// A sends 20 messages, with retransmission ticks.
	for i := 0; i < 20; i++ {
		msg := fmt.Sprintf("msg-%d", i)
		_ = connA.Send([]byte(msg))
		time.Sleep(10 * time.Millisecond)
		connA.Tick(time.Now())
	}

	// Extra retransmission rounds.
	for i := 0; i < 5; i++ {
		time.Sleep(100 * time.Millisecond)
		connA.Tick(time.Now())
	}

	wg.Wait()

	// Drain inbox and verify order.
	received := 0
	lastIdx := -1
	for {
		select {
		case payload := <-connB.Inbox:
			var idx int
			fmt.Sscanf(string(payload), "msg-%d", &idx)
			if idx <= lastIdx {
				t.Errorf("out of order: got msg-%d after msg-%d", idx, lastIdx)
			}
			lastIdx = idx
			received++
		default:
			goto done
		}
	}
done:
	t.Logf("received %d/20 messages in order under 10%% chaos", received)
	if received < 15 {
		t.Errorf("too few messages received: %d (expected most of 20 with retransmission)", received)
	}
}
