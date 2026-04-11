package replay

import (
	"io"
	"rts/internal/wire"
)

// Writer records a game session to an io.Writer (append-only).
type Writer struct {
	w       io.Writer
	written bool // header written?
}

// NewWriter creates a replay writer.
func NewWriter(w io.Writer) *Writer {
	return &Writer{w: w}
}

// WriteHeader writes the replay header. Must be called exactly once, before any ticks.
func (rw *Writer) WriteHeader(h *Header) error {
	data := MarshalHeader(h)
	_, err := rw.w.Write(data)
	if err == nil {
		rw.written = true
	}
	return err
}

// WriteTick appends one tick record.
func (rw *Writer) WriteTick(tick uint32, cmds []wire.Cmd, hash uint64) error {
	tr := &TickRecord{Tick: tick, Cmds: cmds, Hash: hash}
	data := MarshalTick(tr)
	_, err := rw.w.Write(data)
	return err
}
