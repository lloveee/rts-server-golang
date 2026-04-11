package lockstep

import "rts/internal/wire"

// InputBuf stores commands grouped by their target execution tick.
type InputBuf struct {
	buckets map[uint32][]wire.Cmd
}

// NewInputBuf creates a new input buffer.
func NewInputBuf() *InputBuf {
	return &InputBuf{buckets: make(map[uint32][]wire.Cmd)}
}

// Add inserts a command into the bucket for its target tick.
func (ib *InputBuf) Add(cmd wire.Cmd) {
	ib.buckets[cmd.Tick] = append(ib.buckets[cmd.Tick], cmd)
}

// Seal removes and returns all commands for the given tick.
func (ib *InputBuf) Seal(tick uint32) []wire.Cmd {
	cmds := ib.buckets[tick]
	delete(ib.buckets, tick)
	return cmds
}

// Pending returns how many ticks have buffered commands.
func (ib *InputBuf) Pending() int {
	return len(ib.buckets)
}
