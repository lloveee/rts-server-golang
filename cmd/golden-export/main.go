// cmd/golden-export/main.go
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"rts/internal/sim"
	"rts/internal/sim/fixed"
)

type FixedArithTest struct {
	Op     string `json:"op"`
	A      int32  `json:"a"`
	B      int32  `json:"b"`
	Expect int32  `json:"expect"`
}

type CmdJSON struct {
	Tick     uint32 `json:"tick"`
	Player   uint8  `json:"player"`
	Op       uint8  `json:"op"`
	UnitID   uint32 `json:"unitID"`
	TargetX  int32  `json:"targetX"`
	TargetY  int32  `json:"targetY"`
	TargetID uint32 `json:"targetID"`
}

type GoldenData struct {
	Seed             uint64           `json:"seed"`
	MapW             int32            `json:"mapW"`
	MapH             int32            `json:"mapH"`
	Players          int              `json:"players"`
	UnitsPerPlayer   int              `json:"unitsPerPlayer"`
	Ticks            int              `json:"ticks"`
	Commands         []CmdJSON        `json:"commands"`
	TickHashes       []string         `json:"tickHashes"`
	FixedArithTests  []FixedArithTest `json:"fixedArithTests"`
	SinTableRaw      []int32          `json:"sinTableRaw"`
	SplitMixSequence []uint64         `json:"splitMixSequence"`
	Vec2Tests        []Vec2Test       `json:"vec2Tests"`
	Atan2Tests       []Atan2Test      `json:"atan2Tests"`
}

type Vec2Test struct {
	Op      string `json:"op"`
	AX      int32  `json:"ax"`
	AY      int32  `json:"ay"`
	BX      int32  `json:"bx"`
	BY      int32  `json:"by"`
	ExpectX int32  `json:"expectX"`
	ExpectY int32  `json:"expectY"`
}

type Atan2Test struct {
	Y      int32 `json:"y"`
	X      int32 `json:"x"`
	Expect int32 `json:"expect"`
}

func main() {
	g := GoldenData{
		Seed:           42,
		MapW:           100,
		MapH:           100,
		Players:        2,
		UnitsPerPlayer: 5,
		Ticks:          300,
	}

	// Fixed arithmetic tests
	one := fixed.One
	two := fixed.FromInt(2)
	three := fixed.FromInt(3)
	half := fixed.Half
	g.FixedArithTests = []FixedArithTest{
		{"add", int32(one), int32(two), int32(one.Add(two))},
		{"sub", int32(three), int32(one), int32(three.Sub(one))},
		{"mul", int32(two), int32(three), int32(two.Mul(three))},
		{"mul", int32(half), int32(half), int32(half.Mul(half))},
		{"div", int32(three), int32(two), int32(three.Div(two))},
		{"div", int32(one), int32(three), int32(one.Div(three))},
		{"sqrt", int32(fixed.FromInt(4)), 0, int32(fixed.Sqrt(fixed.FromInt(4)))},
		{"sqrt", int32(fixed.FromInt(9)), 0, int32(fixed.Sqrt(fixed.FromInt(9)))},
		{"sqrt", int32(two), 0, int32(fixed.Sqrt(two))},
		{"neg", int32(three), 0, int32(three.Neg())},
		{"abs", int32(three.Neg()), 0, int32(three.Neg().Abs())},
		{"floor", int32(fixed.FromFloat64(2.7)), 0, int32(fixed.FromFloat64(2.7).Floor())},
		{"ceil", int32(fixed.FromFloat64(2.3)), 0, int32(fixed.FromFloat64(2.3).Ceil())},
	}

	// Sin table (all 1024 values)
	g.SinTableRaw = fixed.ExportSinTable()

	// SplitMix64 first 100 values from seed=42
	rng := sim.NewRand(42)
	g.SplitMixSequence = make([]uint64, 100)
	for i := range g.SplitMixSequence {
		g.SplitMixSequence[i] = rng.Next()
	}

	// Vec2 tests
	v1 := fixed.V(one, two)
	v2 := fixed.V(three, one)
	g.Vec2Tests = []Vec2Test{
		{"add", int32(v1.X), int32(v1.Y), int32(v2.X), int32(v2.Y), int32(v1.Add(v2).X), int32(v1.Add(v2).Y)},
		{"sub", int32(v1.X), int32(v1.Y), int32(v2.X), int32(v2.Y), int32(v1.Sub(v2).X), int32(v1.Sub(v2).Y)},
		{"dist", int32(v1.X), int32(v1.Y), int32(v2.X), int32(v2.Y), int32(v1.Dist(v2)), 0},
		{"normalize", int32(fixed.FromInt(3)), int32(fixed.FromInt(4)), 0, 0,
			int32(fixed.V(fixed.FromInt(3), fixed.FromInt(4)).Normalize().X),
			int32(fixed.V(fixed.FromInt(3), fixed.FromInt(4)).Normalize().Y)},
	}

	// Atan2 tests
	g.Atan2Tests = []Atan2Test{
		{int32(one), int32(one), int32(fixed.Atan2(one, one))},
		{int32(one), int32(one.Neg()), int32(fixed.Atan2(one, one.Neg()))},
		{int32(one.Neg()), int32(one), int32(fixed.Atan2(one.Neg(), one))},
		{int32(0), int32(one), int32(fixed.Atan2(0, one))},
		{int32(one), int32(0), int32(fixed.Atan2(one, 0))},
	}

	// Build world and run simulation
	w := sim.NewWorld(g.Seed, g.MapW, g.MapH)

	// Spawn units: player 0 on left, player 1 on right
	for i := 0; i < g.UnitsPerPlayer; i++ {
		sim.SpawnForGolden(w, 0, fixed.V(fixed.FromInt(int32(10+i*5)), fixed.FromInt(int32(50))),
			fixed.One, fixed.FromFloat64(0.5))
		sim.SpawnForGolden(w, 1, fixed.V(fixed.FromInt(int32(90-i*5)), fixed.FromInt(int32(50))),
			fixed.One, fixed.FromFloat64(0.5))
	}

	// Pre-generate commands: move player 0's units right, player 1's units left
	cmdsPerTick := make(map[uint32][]sim.Cmd)
	// At tick 1: move all player 0 units to (50,50)
	for i := uint32(1); i <= uint32(g.UnitsPerPlayer); i++ {
		cmd := sim.Cmd{
			Player:    0,
			Op:        sim.CmdMove,
			UnitID:    i,
			TargetPos: fixed.V(fixed.FromInt(50), fixed.FromInt(50)),
		}
		cmdsPerTick[1] = append(cmdsPerTick[1], cmd)
		g.Commands = append(g.Commands, CmdJSON{
			Tick:    1,
			Player:  0,
			Op:      uint8(sim.CmdMove),
			UnitID:  i,
			TargetX: int32(fixed.FromInt(50)),
			TargetY: int32(fixed.FromInt(50)),
		})
	}
	// At tick 50: move player 1 units to (50,50)
	for i := uint32(g.UnitsPerPlayer + 1); i <= uint32(g.UnitsPerPlayer*2); i++ {
		cmd := sim.Cmd{
			Player:    1,
			Op:        sim.CmdMove,
			UnitID:    i,
			TargetPos: fixed.V(fixed.FromInt(50), fixed.FromInt(50)),
		}
		cmdsPerTick[50] = append(cmdsPerTick[50], cmd)
		g.Commands = append(g.Commands, CmdJSON{
			Tick:    50,
			Player:  1,
			Op:      uint8(sim.CmdMove),
			UnitID:  i,
			TargetX: int32(fixed.FromInt(50)),
			TargetY: int32(fixed.FromInt(50)),
		})
	}
	// At tick 100: stop all player 0 units
	for i := uint32(1); i <= uint32(g.UnitsPerPlayer); i++ {
		cmd := sim.Cmd{
			Player: 0,
			Op:     sim.CmdStop,
			UnitID: i,
		}
		cmdsPerTick[100] = append(cmdsPerTick[100], cmd)
		g.Commands = append(g.Commands, CmdJSON{
			Tick:   100,
			Player: 0,
			Op:     uint8(sim.CmdStop),
			UnitID: i,
		})
	}

	// Run simulation
	g.TickHashes = make([]string, g.Ticks)
	for tick := 0; tick < g.Ticks; tick++ {
		cmds := cmdsPerTick[uint32(tick+1)] // Step increments tick first
		sim.Step(w, cmds)
		h := sim.Hash(w)
		g.TickHashes[tick] = fmt.Sprintf("%016x", h)
	}

	// Write JSON
	data, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "json error: %v\n", err)
		os.Exit(1)
	}

	outPath := "golden_data.json"
	if len(os.Args) > 1 {
		outPath = os.Args[1]
	}
	if err := os.WriteFile(outPath, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "write error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Wrote %d bytes to %s\n", len(data), outPath)
}
