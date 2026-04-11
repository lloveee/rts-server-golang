package main

import (
	"flag"
	"fmt"
	"os"
	"rts/internal/replay"
	"rts/internal/sim"
	"rts/internal/sim/fixed"
	"rts/internal/wire"
)

func main() {
	replayFile := flag.String("file", "", "path to replay file (required)")
	verify := flag.Bool("verify", true, "verify hashes against recorded values")
	flag.Parse()

	if *replayFile == "" {
		fmt.Fprintln(os.Stderr, "usage: replay-play -file <replay.rply> [-verify=true]")
		os.Exit(2)
	}

	f, err := os.Open(*replayFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	reader, err := replay.NewReaderFromIO(f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read header: %v\n", err)
		os.Exit(1)
	}

	h := reader.Header
	fmt.Printf("replay: seed=%d tick_rate=%d players=%d map=%dx%d snapshot=%d bytes\n",
		h.Seed, h.TickRate, h.PlayerCount, h.MapW, h.MapH, len(h.Snapshot))

	// Restore world from initial snapshot.
	world, err := sim.Unmarshal(h.Snapshot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unmarshal snapshot: %v\n", err)
		os.Exit(1)
	}

	ticks, err := reader.ReadAll()
	if err != nil {
		fmt.Fprintf(os.Stderr, "read ticks: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("replaying %d ticks...\n", len(ticks))

	desyncCount := 0
	for _, tr := range ticks {
		cmds := wireCmdsToSimCmds(tr.Cmds)
		sim.Step(world, cmds)

		if *verify {
			hash := sim.Hash(world)
			if hash != tr.Hash {
				desyncCount++
				if desyncCount <= 10 {
					fmt.Printf("DESYNC at tick %d: replay=%016x computed=%016x\n",
						tr.Tick, tr.Hash, hash)
				}
			}
		}

		if tr.Tick%1000 == 0 {
			fmt.Printf("  tick %d...\n", tr.Tick)
		}
	}

	if desyncCount > 0 {
		fmt.Printf("FAILED: %d desyncs in %d ticks\n", desyncCount, len(ticks))
		os.Exit(1)
	}

	lastHash := uint64(0)
	if len(ticks) > 0 {
		lastHash = ticks[len(ticks)-1].Hash
	}
	fmt.Printf("OK: %d ticks replayed, 0 desyncs, final hash: %016x\n", len(ticks), lastHash)
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
