package obs

import (
	"bytes"
	"strings"
	"testing"
)

func TestCounter(t *testing.T) {
	c := &Counter{}
	c.Inc()
	c.Inc()
	c.Add(3)
	if c.Value() != 5 {
		t.Errorf("got %d want 5", c.Value())
	}
}

func TestGauge(t *testing.T) {
	g := &Gauge{}
	g.Set(10)
	g.Inc()
	g.Dec()
	g.Dec()
	if g.Value() != 9 {
		t.Errorf("got %d want 9", g.Value())
	}
}

func TestHistogram(t *testing.T) {
	h := &Histogram{}
	h.Observe(1.0)
	h.Observe(3.0)
	h.Observe(5.0)
	count, sum, min, max := h.Snapshot()
	if count != 3 {
		t.Errorf("count: got %d want 3", count)
	}
	if sum != 9.0 {
		t.Errorf("sum: got %f want 9", sum)
	}
	if min != 1.0 {
		t.Errorf("min: got %f want 1", min)
	}
	if max != 5.0 {
		t.Errorf("max: got %f want 5", max)
	}
}

func TestRegistryWriteTo(t *testing.T) {
	r := NewRegistry()
	r.Counter("rts_packets_in_total").Add(100)
	r.Counter("rts_packets_out_total").Add(200)
	r.Gauge("rts_rooms_active").Set(3)
	r.Histogram("rts_tick_duration_ms").Observe(2.5)
	r.Histogram("rts_tick_duration_ms").Observe(3.5)

	var buf bytes.Buffer
	r.WriteTo(&buf)
	out := buf.String()

	if !strings.Contains(out, "rts_packets_in_total 100") {
		t.Error("missing packets_in")
	}
	if !strings.Contains(out, "rts_rooms_active 3") {
		t.Error("missing rooms_active")
	}
	if !strings.Contains(out, "rts_tick_duration_ms_count 2") {
		t.Error("missing tick_duration count")
	}
}

func TestTraceRing(t *testing.T) {
	tr := NewTraceRing(4)

	for i := uint32(1); i <= 6; i++ {
		tr.Record(TraceEntry{Tick: i, CmdCount: int(i)})
	}

	entries := tr.Snapshot()
	if len(entries) != 4 {
		t.Fatalf("got %d entries, want 4", len(entries))
	}
	// Should have ticks 3,4,5,6 (oldest dropped).
	if entries[0].Tick != 3 {
		t.Errorf("first tick: got %d want 3", entries[0].Tick)
	}
	if entries[3].Tick != 6 {
		t.Errorf("last tick: got %d want 6", entries[3].Tick)
	}

	var buf bytes.Buffer
	tr.WriteTo(&buf)
	if !strings.Contains(buf.String(), "tick\t") {
		t.Error("missing header in trace dump")
	}
}
