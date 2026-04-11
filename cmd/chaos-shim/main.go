// chaos-shim is a standalone UDP proxy that injects packet loss, delay, and reordering
// between a client and server for testing network resilience.
package main

import (
	"flag"
	"fmt"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

func main() {
	listen := flag.String("listen", ":9002", "local UDP listen address")
	forward := flag.String("forward", "127.0.0.1:9000", "remote server address")
	dropPct := flag.Float64("drop", 0.05, "packet drop probability [0.0-1.0]")
	delayMs := flag.Int("delay", 50, "added one-way delay in ms")
	jitterMs := flag.Int("jitter", 20, "delay jitter in ms")
	flag.Parse()

	fmt.Printf("chaos-shim %s → %s (drop=%.1f%% delay=%d±%dms)\n",
		*listen, *forward, *dropPct*100, *delayMs, *jitterMs)

	listenAddr, err := net.ResolveUDPAddr("udp", *listen)
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve listen: %v\n", err)
		os.Exit(1)
	}
	serverAddr, err := net.ResolveUDPAddr("udp", *forward)
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve forward: %v\n", err)
		os.Exit(1)
	}

	conn, err := net.ListenUDP("udp", listenAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "listen: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	// Track client addresses to forward responses back.
	var mu sync.RWMutex
	clients := make(map[string]*net.UDPAddr) // server-side key → client addr

	var dropped, forwarded uint64

	// Forward function with chaos injection.
	chaosForward := func(data []byte, dst *net.UDPAddr) {
		if rng.Float64() < *dropPct {
			dropped++
			return
		}
		delay := time.Duration(*delayMs) * time.Millisecond
		if *jitterMs > 0 {
			delay += time.Duration(rng.Intn(*jitterMs*2)-*jitterMs) * time.Millisecond
			if delay < 0 {
				delay = 0
			}
		}
		if delay > 0 {
			pkt := make([]byte, len(data))
			copy(pkt, data)
			go func() {
				time.Sleep(delay)
				conn.WriteToUDP(pkt, dst)
			}()
		} else {
			conn.WriteToUDP(data, dst)
		}
		forwarded++
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		fmt.Printf("\nchaos-shim stats: forwarded=%d dropped=%d\n", forwarded, dropped)
		os.Exit(0)
	}()

	buf := make([]byte, 2048)
	for {
		n, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			continue
		}
		data := make([]byte, n)
		copy(data, buf[:n])

		if remoteAddr.String() == serverAddr.String() {
			// Response from server → find client to forward to.
			mu.RLock()
			// For simplicity, forward to all known clients (the server response
			// contains conn_id so the client layer filters correctly).
			for _, clientAddr := range clients {
				chaosForward(data, clientAddr)
			}
			mu.RUnlock()
		} else {
			// Packet from client → forward to server.
			mu.Lock()
			clients[remoteAddr.String()] = remoteAddr
			mu.Unlock()
			chaosForward(data, serverAddr)
		}
	}
}
