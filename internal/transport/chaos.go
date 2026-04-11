package transport

import (
	"math/rand"
	"time"
)

// ChaosHook allows injecting network chaos (packet loss, delay, reorder)
// for testing. In production, use nil — the transport layer checks for nil
// before calling any hook method.
type ChaosHook interface {
	// ShouldDrop returns true if this packet should be silently dropped.
	ShouldDrop() bool
	// Delay returns additional one-way delay to add before delivery.
	Delay() time.Duration
	// ShouldDuplicate returns true if this packet should be sent twice.
	ShouldDuplicate() bool
}

// ChaosConfig configures a chaos hook for testing.
type ChaosConfig struct {
	DropRate     float64       // probability [0, 1) of dropping a packet
	MinDelay     time.Duration // minimum added delay
	MaxDelay     time.Duration // maximum added delay
	DuplicateRate float64      // probability [0, 1) of duplicating a packet
}

// chaosHookImpl implements ChaosHook.
type chaosHookImpl struct {
	cfg ChaosConfig
	rng *rand.Rand
}

// NewChaosHook creates a ChaosHook from the given config.
func NewChaosHook(cfg ChaosConfig) ChaosHook {
	return &chaosHookImpl{
		cfg: cfg,
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (c *chaosHookImpl) ShouldDrop() bool {
	return c.rng.Float64() < c.cfg.DropRate
}

func (c *chaosHookImpl) Delay() time.Duration {
	if c.cfg.MaxDelay <= c.cfg.MinDelay {
		return c.cfg.MinDelay
	}
	jitter := time.Duration(c.rng.Int63n(int64(c.cfg.MaxDelay - c.cfg.MinDelay)))
	return c.cfg.MinDelay + jitter
}

func (c *chaosHookImpl) ShouldDuplicate() bool {
	return c.rng.Float64() < c.cfg.DuplicateRate
}
