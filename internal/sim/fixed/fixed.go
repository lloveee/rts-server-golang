// Package fixed provides Q16.16 fixed-point arithmetic.
//
// A Fix32 is a signed 32-bit integer where the lower 16 bits represent
// the fractional part and the upper 16 bits the integer part.
// Range: approximately [-32768, +32768) with precision of 1/65536 ≈ 0.0000153.
//
// All operations are deterministic and produce identical results across
// all platforms and Go versions — no float anywhere.
package fixed

const (
	Shift = 16
	One   = Fix32(1 << Shift)          // 1.0
	Half  = Fix32(1 << (Shift - 1))    // 0.5
	Max   = Fix32(0x7FFFFFFF)          // ~32767.999985
	Min   = Fix32(-0x80000000)         // -32768.0
	Pi    = Fix32(205887)              // π ≈ 3.14159
	TwoPi = Fix32(411775)             // 2π ≈ 6.28318
	Eps   = Fix32(1)                   // smallest representable positive value
)

// Fix32 is a Q16.16 fixed-point number.
type Fix32 int32

// FromInt converts an integer to Fix32.
func FromInt(v int32) Fix32 {
	return Fix32(v << Shift)
}

// FromFloat64 converts a float64 to Fix32.
// This is ONLY for initialization/test — never call at runtime in sim.
func FromFloat64(v float64) Fix32 {
	return Fix32(v * float64(One))
}

// ToInt returns the integer part (truncated toward zero).
func (a Fix32) ToInt() int32 {
	v := int64(a)
	if v >= 0 {
		return int32(v >> Shift)
	}
	return int32(-((-v) >> Shift))
}

// ToFloat64 converts to float64 for debug/display only.
func (a Fix32) ToFloat64() float64 {
	return float64(a) / float64(One)
}

// Raw returns the underlying int32 representation.
func (a Fix32) Raw() int32 {
	return int32(a)
}

// FromRaw creates a Fix32 from its raw representation.
func FromRaw(v int32) Fix32 {
	return Fix32(v)
}

// Add returns a + b.
func (a Fix32) Add(b Fix32) Fix32 {
	return a + b
}

// Sub returns a - b.
func (a Fix32) Sub(b Fix32) Fix32 {
	return a - b
}

// Mul returns a * b using 64-bit intermediate to avoid overflow.
func (a Fix32) Mul(b Fix32) Fix32 {
	return Fix32((int64(a) * int64(b)) >> Shift)
}

// Div returns a / b. Panics on division by zero.
func (a Fix32) Div(b Fix32) Fix32 {
	return Fix32((int64(a) << Shift) / int64(b))
}

// Neg returns -a.
func (a Fix32) Neg() Fix32 {
	return -a
}

// Abs returns |a|.
func (a Fix32) Abs() Fix32 {
	if a < 0 {
		return -a
	}
	return a
}

// Floor returns the largest integer <= a.
func (a Fix32) Floor() Fix32 {
	return a & ^Fix32(One-1)
}

// Ceil returns the smallest integer >= a.
func (a Fix32) Ceil() Fix32 {
	if a&Fix32(One-1) == 0 {
		return a
	}
	return (a & ^Fix32(One-1)) + One
}

// Clamp returns a clamped to [lo, hi].
func (a Fix32) Clamp(lo, hi Fix32) Fix32 {
	if a < lo {
		return lo
	}
	if a > hi {
		return hi
	}
	return a
}

// Sqrt returns the square root of a using binary search.
// Returns 0 for negative input.
func Sqrt(a Fix32) Fix32 {
	if a <= 0 {
		return 0
	}

	// Use int64 to avoid overflow during the search.
	n := int64(a) << Shift // scale up for precision
	var result int64

	// Binary search: find largest r such that r*r <= n.
	bit := int64(1) << 30 // start from high bit
	for bit > 0 {
		trial := result | bit
		if trial*trial <= n {
			result = trial
		}
		bit >>= 1
	}
	return Fix32(result)
}

// Min2 returns the smaller of a, b.
func Min2(a, b Fix32) Fix32 {
	if a < b {
		return a
	}
	return b
}

// Max2 returns the larger of a, b.
func Max2(a, b Fix32) Fix32 {
	if a > b {
		return a
	}
	return b
}
