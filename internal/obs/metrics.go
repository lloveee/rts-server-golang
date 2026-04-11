// Package obs provides observability primitives: metrics, structured logging, and tracing.
// All hot-path operations are zero-allocation.
package obs

import (
	"fmt"
	"io"
	"sort"
	"sync"
	"sync/atomic"
)

// Counter is a monotonically increasing uint64 counter. Zero-alloc increment.
type Counter struct {
	val uint64
}

func (c *Counter) Inc()            { atomic.AddUint64(&c.val, 1) }
func (c *Counter) Add(n uint64)    { atomic.AddUint64(&c.val, n) }
func (c *Counter) Value() uint64   { return atomic.LoadUint64(&c.val) }

// Gauge is an int64 value that can go up and down. Zero-alloc set/inc/dec.
type Gauge struct {
	val int64
}

func (g *Gauge) Set(v int64)      { atomic.StoreInt64(&g.val, v) }
func (g *Gauge) Inc()             { atomic.AddInt64(&g.val, 1) }
func (g *Gauge) Dec()             { atomic.AddInt64(&g.val, -1) }
func (g *Gauge) Add(n int64)      { atomic.AddInt64(&g.val, n) }
func (g *Gauge) Value() int64     { return atomic.LoadInt64(&g.val) }

// Histogram tracks a distribution via a fixed set of buckets.
// Not a full histogram — stores min/max/sum/count for lightweight monitoring.
type Histogram struct {
	mu    sync.Mutex
	count uint64
	sum   float64
	min   float64
	max   float64
}

func (h *Histogram) Observe(v float64) {
	h.mu.Lock()
	h.count++
	h.sum += v
	if h.count == 1 || v < h.min {
		h.min = v
	}
	if v > h.max {
		h.max = v
	}
	h.mu.Unlock()
}

// Snapshot returns count, sum, min, max.
func (h *Histogram) Snapshot() (count uint64, sum, min, max float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.count, h.sum, h.min, h.max
}

// Registry holds all named metrics.
type Registry struct {
	mu         sync.RWMutex
	counters   map[string]*Counter
	gauges     map[string]*Gauge
	histograms map[string]*Histogram
}

// NewRegistry creates a metrics registry.
func NewRegistry() *Registry {
	return &Registry{
		counters:   make(map[string]*Counter),
		gauges:     make(map[string]*Gauge),
		histograms: make(map[string]*Histogram),
	}
}

// Counter returns a named counter, creating it if needed.
func (r *Registry) Counter(name string) *Counter {
	r.mu.RLock()
	c, ok := r.counters[name]
	r.mu.RUnlock()
	if ok {
		return c
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.counters[name]; ok {
		return c
	}
	c = &Counter{}
	r.counters[name] = c
	return c
}

// Gauge returns a named gauge, creating it if needed.
func (r *Registry) Gauge(name string) *Gauge {
	r.mu.RLock()
	g, ok := r.gauges[name]
	r.mu.RUnlock()
	if ok {
		return g
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if g, ok := r.gauges[name]; ok {
		return g
	}
	g = &Gauge{}
	r.gauges[name] = g
	return g
}

// Histogram returns a named histogram, creating it if needed.
func (r *Registry) Histogram(name string) *Histogram {
	r.mu.RLock()
	h, ok := r.histograms[name]
	r.mu.RUnlock()
	if ok {
		return h
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if h, ok := r.histograms[name]; ok {
		return h
	}
	h = &Histogram{}
	r.histograms[name] = h
	return h
}

// WriteTo writes all metrics in expvar-like text format.
func (r *Registry) WriteTo(w io.Writer) (int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var total int64

	// Sort keys for deterministic output.
	counterNames := sortedKeys(r.counters)
	gaugeNames := sortedKeys(r.gauges)
	histNames := sortedKeys(r.histograms)

	for _, name := range counterNames {
		n, _ := fmt.Fprintf(w, "%s %d\n", name, r.counters[name].Value())
		total += int64(n)
	}
	for _, name := range gaugeNames {
		n, _ := fmt.Fprintf(w, "%s %d\n", name, r.gauges[name].Value())
		total += int64(n)
	}
	for _, name := range histNames {
		h := r.histograms[name]
		count, sum, min, max := h.Snapshot()
		avg := float64(0)
		if count > 0 {
			avg = sum / float64(count)
		}
		n, _ := fmt.Fprintf(w, "%s_count %d\n%s_sum %.3f\n%s_min %.3f\n%s_max %.3f\n%s_avg %.3f\n",
			name, count, name, sum, name, min, name, max, name, avg)
		total += int64(n)
	}

	return total, nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// DefaultRegistry is the global metrics registry.
var DefaultRegistry = NewRegistry()
