package sim

import "rts/internal/sim/fixed"

// UnitStats holds per-unit-type parameters. All values Q16.16 fixed-point.
type UnitStats struct {
	MaxHP       fixed.Fix32
	Speed       fixed.Fix32
	Range       fixed.Fix32
	Damage      fixed.Fix32
	VisionRange fixed.Fix32
	Cost        fixed.Fix32
	TrainTicks  uint32
}

var UnitStatTable = map[UnitType]UnitStats{
	UnitWorker: {
		MaxHP:       fixed.FromInt(20),
		Speed:       fixed.FromFloat64(0.8),
		Range:       fixed.Zero,
		Damage:      fixed.Zero,
		VisionRange: fixed.FromInt(6),
		Cost:        fixed.FromInt(50),
		TrainTicks:  10,
	},
	UnitSoldier: {
		MaxHP:       fixed.FromInt(60),
		Speed:       fixed.FromFloat64(0.5),
		Range:       fixed.FromFloat64(1.5),
		Damage:      fixed.One,
		VisionRange: fixed.FromInt(8),
		Cost:        fixed.FromInt(80),
		TrainTicks:  15,
	},
	UnitArcher: {
		MaxHP:       fixed.FromInt(40),
		Speed:       fixed.FromFloat64(0.5),
		Range:       fixed.FromInt(6),
		Damage:      fixed.FromFloat64(0.6),
		VisionRange: fixed.FromInt(12),
		Cost:        fixed.FromInt(120),
		TrainTicks:  20,
	},
	UnitCavalry: {
		MaxHP:       fixed.FromInt(100),
		Speed:       fixed.FromFloat64(1.2),
		Range:       fixed.FromFloat64(1.5),
		Damage:      fixed.FromFloat64(1.5),
		VisionRange: fixed.FromInt(10),
		Cost:        fixed.FromInt(200),
		TrainTicks:  30,
	},
}

// BuildingStats holds per-building-type parameters.
type BuildingStats struct {
	MaxHP       fixed.Fix32
	VisionRange fixed.Fix32
	Cost        fixed.Fix32
	BuildTicks  uint32
	SizeCells   uint8
	Trains      []UnitType
}

var BuildingStatTable = map[BuildingType]BuildingStats{
	BldHQ: {
		MaxHP:       fixed.FromInt(800),
		VisionRange: fixed.FromInt(12),
		Cost:        fixed.Zero,
		BuildTicks:  0,
		SizeCells:   4,
		Trains:      []UnitType{UnitWorker, UnitSoldier},
	},
	BldBarracks: {
		MaxHP:       fixed.FromInt(400),
		VisionRange: fixed.FromInt(8),
		Cost:        fixed.FromInt(150),
		BuildTicks:  60,
		SizeCells:   3,
		Trains:      []UnitType{UnitSoldier},
	},
	BldArchery: {
		MaxHP:       fixed.FromInt(400),
		VisionRange: fixed.FromInt(10),
		Cost:        fixed.FromInt(150),
		BuildTicks:  60,
		SizeCells:   3,
		Trains:      []UnitType{UnitArcher},
	},
	BldStable: {
		MaxHP:       fixed.FromInt(400),
		VisionRange: fixed.FromInt(8),
		Cost:        fixed.FromInt(150),
		BuildTicks:  60,
		SizeCells:   3,
		Trains:      []UnitType{UnitCavalry},
	},
}

const (
	CarryCapacity      = 5
	MiningTicksPerTrip = 30
	MaxQueueLength     = 5
	CrystalsPerPlayer  = 8
	CrystalStartValue  = 1500
)

// CrystalPositions returns fixed crystal spawn positions for a 100x100 map.
func CrystalPositions(playerID uint8) []fixed.Vec2 {
	var baseX int32 = 5
	if playerID == 1 {
		baseX = 75
	}
	return []fixed.Vec2{
		fixed.VInt(baseX, 10),
		fixed.VInt(baseX+5, 25),
		fixed.VInt(baseX+10, 40),
		fixed.VInt(baseX+15, 55),
		fixed.VInt(baseX+20, 70),
		fixed.VInt(baseX+5, 80),
		fixed.VInt(baseX+10, 15),
		fixed.VInt(baseX+15, 65),
	}
}
