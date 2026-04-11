package fixed

// Vec2 is a 2D vector in fixed-point.
type Vec2 struct {
	X, Y Fix32
}

// V creates a Vec2 from two Fix32 values.
func V(x, y Fix32) Vec2 {
	return Vec2{X: x, Y: y}
}

// VInt creates a Vec2 from two integers.
func VInt(x, y int32) Vec2 {
	return Vec2{X: FromInt(x), Y: FromInt(y)}
}

// Add returns a + b.
func (a Vec2) Add(b Vec2) Vec2 {
	return Vec2{X: a.X + b.X, Y: a.Y + b.Y}
}

// Sub returns a - b.
func (a Vec2) Sub(b Vec2) Vec2 {
	return Vec2{X: a.X - b.X, Y: a.Y - b.Y}
}

// Scale returns a * s.
func (a Vec2) Scale(s Fix32) Vec2 {
	return Vec2{X: a.X.Mul(s), Y: a.Y.Mul(s)}
}

// Dot returns a · b.
func (a Vec2) Dot(b Vec2) Fix32 {
	return a.X.Mul(b.X).Add(a.Y.Mul(b.Y))
}

// LenSq returns |a|² (avoids sqrt).
func (a Vec2) LenSq() Fix32 {
	return a.Dot(a)
}

// Len returns |a|.
func (a Vec2) Len() Fix32 {
	return Sqrt(a.LenSq())
}

// DistSq returns |a - b|².
func (a Vec2) DistSq(b Vec2) Fix32 {
	return a.Sub(b).LenSq()
}

// Dist returns |a - b|.
func (a Vec2) Dist(b Vec2) Fix32 {
	return Sqrt(a.DistSq(b))
}

// Normalize returns a unit vector in the same direction.
// Returns zero vector if a is zero.
func (a Vec2) Normalize() Vec2 {
	l := a.Len()
	if l == 0 {
		return Vec2{}
	}
	return Vec2{X: a.X.Div(l), Y: a.Y.Div(l)}
}

// MoveToward returns a point moved from `from` toward `target` by `maxDist`.
// If already within maxDist, returns target.
func MoveToward(from, target Vec2, maxDist Fix32) Vec2 {
	diff := target.Sub(from)
	dSq := diff.LenSq()
	if dSq <= maxDist.Mul(maxDist) {
		return target
	}
	d := Sqrt(dSq)
	return from.Add(diff.Scale(maxDist.Div(d)))
}
