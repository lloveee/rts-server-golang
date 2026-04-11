package fixed

import (
	"math"
	"testing"
)

func TestFromInt(t *testing.T) {
	tests := []struct{ in int32 }{
		{0}, {1}, {-1}, {100}, {-32768}, {32767},
	}
	for _, tt := range tests {
		f := FromInt(tt.in)
		got := f.ToInt()
		if got != tt.in {
			t.Errorf("FromInt(%d).ToInt() = %d", tt.in, got)
		}
	}
}

func TestMulDiv(t *testing.T) {
	a := FromFloat64(3.5)
	b := FromFloat64(2.0)

	mul := a.Mul(b)
	if got := mul.ToFloat64(); math.Abs(got-7.0) > 0.001 {
		t.Errorf("3.5 * 2.0 = %f, want 7.0", got)
	}

	div := a.Div(b)
	if got := div.ToFloat64(); math.Abs(got-1.75) > 0.001 {
		t.Errorf("3.5 / 2.0 = %f, want 1.75", got)
	}
}

func TestSqrt(t *testing.T) {
	tests := []struct {
		in   float64
		want float64
	}{
		{0, 0},
		{1, 1},
		{4, 2},
		{9, 3},
		{2, 1.4142},
		{100, 10},
		{0.25, 0.5},
	}
	for _, tt := range tests {
		got := Sqrt(FromFloat64(tt.in)).ToFloat64()
		if math.Abs(got-tt.want) > 0.01 {
			t.Errorf("Sqrt(%f) = %f, want %f", tt.in, got, tt.want)
		}
	}
}

func TestSinCos(t *testing.T) {
	tests := []struct {
		angle float64
		sinW  float64
		cosW  float64
	}{
		{0, 0, 1},
		{math.Pi / 2, 1, 0},
		{math.Pi, 0, -1},
		{3 * math.Pi / 2, -1, 0},
		{math.Pi / 4, 0.7071, 0.7071},
		{math.Pi / 6, 0.5, 0.8660},
	}
	for _, tt := range tests {
		a := FromFloat64(tt.angle)
		s := Sin(a).ToFloat64()
		c := Cos(a).ToFloat64()
		if math.Abs(s-tt.sinW) > 0.02 {
			t.Errorf("Sin(%f) = %f, want %f", tt.angle, s, tt.sinW)
		}
		if math.Abs(c-tt.cosW) > 0.02 {
			t.Errorf("Cos(%f) = %f, want %f", tt.angle, c, tt.cosW)
		}
	}
}

func TestNegativeSin(t *testing.T) {
	a := FromFloat64(-math.Pi / 2)
	s := Sin(a).ToFloat64()
	if math.Abs(s-(-1)) > 0.02 {
		t.Errorf("Sin(-π/2) = %f, want -1", s)
	}
}

func TestVec2Len(t *testing.T) {
	v := V(FromInt(3), FromInt(4))
	l := v.Len().ToFloat64()
	if math.Abs(l-5.0) > 0.01 {
		t.Errorf("|(3,4)| = %f, want 5.0", l)
	}
}

func TestVec2MoveToward(t *testing.T) {
	from := VInt(0, 0)
	target := VInt(10, 0)
	maxDist := FromInt(3)

	result := MoveToward(from, target, maxDist)
	if math.Abs(result.X.ToFloat64()-3.0) > 0.01 {
		t.Errorf("MoveToward X = %f, want 3.0", result.X.ToFloat64())
	}

	// When close enough, snap to target
	from2 := V(FromFloat64(9.5), FromInt(0))
	result2 := MoveToward(from2, target, maxDist)
	if result2.X != target.X || result2.Y != target.Y {
		t.Errorf("MoveToward should snap: got (%f,%f), want (10,0)",
			result2.X.ToFloat64(), result2.Y.ToFloat64())
	}
}

func TestDeterminism(t *testing.T) {
	// Run the same operations 100 times and verify identical results.
	var results [100]int32
	for i := range results {
		a := FromFloat64(3.14159)
		b := FromFloat64(2.71828)
		c := a.Mul(b).Add(Sqrt(a)).Sub(b.Div(a))
		s := Sin(a).Mul(Cos(b))
		results[i] = (c + s).Raw()
	}
	for i := 1; i < len(results); i++ {
		if results[i] != results[0] {
			t.Fatalf("determinism broken: run %d got %d, run 0 got %d", i, results[i], results[0])
		}
	}
}
