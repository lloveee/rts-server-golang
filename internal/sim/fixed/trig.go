package fixed

// Trig functions using a lookup table for sin.
// Table has 1024 entries covering [0, π/2).
// cos/atan2 derived from sin table.

const sinTableSize = 1024

// sinTable covers [0, π/2) in 1024 steps.
// Each entry = sin(i * π/2 / 1024) * 65536, stored as int32.
var sinTable [sinTableSize]int32

func init() {
	// Build table at startup using float64 — this is the ONLY place floats
	// are allowed, and it runs once deterministically at process start.
	// The table is identical on every platform because it's computed from
	// the same Go math package and stored as int32.
	//
	// We inline a minimal sine computation here to avoid importing math
	// (which would tempt usage elsewhere). Taylor series with enough terms
	// for <1 ULP error in Q16.16 range.
	for i := 0; i < sinTableSize; i++ {
		// angle in radians: i * (π/2) / 1024
		// π/2 ≈ 1.5707963267948966
		x := float64(i) * 1.5707963267948966 / float64(sinTableSize)
		// Taylor: sin(x) = x - x³/6 + x⁵/120 - x⁷/5040 + x⁹/362880
		x2 := x * x
		x3 := x2 * x
		x5 := x3 * x2
		x7 := x5 * x2
		x9 := x7 * x2
		x11 := x9 * x2
		s := x - x3/6.0 + x5/120.0 - x7/5040.0 + x9/362880.0 - x11/39916800.0
		sinTable[i] = int32(s*65536.0 + 0.5)
	}
}

// Sin returns sin(angle) where angle is in Fix32 radians.
func Sin(angle Fix32) Fix32 {
	// Normalize angle to [0, 2π)
	a := int32(angle) % int32(TwoPi)
	if a < 0 {
		a += int32(TwoPi)
	}

	halfPi := int32(TwoPi) / 4 // π/2 in raw
	pi := int32(TwoPi) / 2     // π in raw

	// Determine quadrant and index into table
	var idx int
	var neg bool

	switch {
	case a < halfPi:
		idx = int(int64(a) * int64(sinTableSize) / int64(halfPi))
		neg = false
	case a < pi:
		idx = int(int64(pi-a) * int64(sinTableSize) / int64(halfPi))
		neg = false
	case a < pi+halfPi:
		idx = int(int64(a-pi) * int64(sinTableSize) / int64(halfPi))
		neg = true
	default:
		idx = int(int64(int32(TwoPi)-a) * int64(sinTableSize) / int64(halfPi))
		neg = true
	}

	if idx >= sinTableSize {
		idx = sinTableSize - 1
	}
	if idx < 0 {
		idx = 0
	}

	v := Fix32(sinTable[idx])
	if neg {
		return -v
	}
	return v
}

// Cos returns cos(angle) where angle is in Fix32 radians.
func Cos(angle Fix32) Fix32 {
	// cos(x) = sin(x + π/2)
	halfPi := Fix32(int32(TwoPi) / 4)
	return Sin(angle + halfPi)
}

// Atan2 returns the angle in radians of the vector (x, y).
// Uses a CORDIC-style approximation.
func Atan2(y, x Fix32) Fix32 {
	if x == 0 && y == 0 {
		return 0
	}

	halfPi := Fix32(int32(TwoPi) / 4)
	pi := Fix32(int32(TwoPi) / 2)

	// Handle special axes
	if x == 0 {
		if y > 0 {
			return halfPi
		}
		return -halfPi
	}

	// Use |y|/|x| ratio with polynomial approximation
	ax := x.Abs()
	ay := y.Abs()

	var angle Fix32
	if ax >= ay {
		// ratio = ay/ax, in [0, 1]
		ratio := ay.Div(ax)
		// atan(r) ≈ r * (π/4) for small r (first-order)
		// Better: atan(r) ≈ r * 0.9817 - r³ * 0.1963 (max error ~0.004 rad)
		r2 := ratio.Mul(ratio)
		// 0.9817 ≈ 64337 in Q16.16; 0.1963 ≈ 12864 in Q16.16
		angle = ratio.Mul(Fix32(64337)).Sub(r2.Mul(ratio).Mul(Fix32(12864)))
	} else {
		ratio := ax.Div(ay)
		r2 := ratio.Mul(ratio)
		angle = halfPi - ratio.Mul(Fix32(64337)).Sub(r2.Mul(ratio).Mul(Fix32(12864)))
	}

	// Map back to correct quadrant
	if x < 0 {
		angle = pi - angle
	}
	if y < 0 {
		angle = -angle
	}
	return angle
}
