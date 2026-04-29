package sim

import (
	"container/heap"
	"rts/internal/sim/fixed"
)

const (
	diagCost     = fixed.Fix32(92682) // sqrt(2) ≈ 1.4142 Q16.16
	straightCost = fixed.Fix32(65536) // 1.0
)

type aStarNode struct {
	x, y   int32
	g, f   fixed.Fix32
	parent *aStarNode
}

type aStarHeap []*aStarNode

func (h aStarHeap) Len() int { return len(h) }
func (h aStarHeap) Less(i, j int) bool {
	if h[i].f != h[j].f {
		return h[i].f < h[j].f
	}
	if h[i].y != h[j].y {
		return h[i].y < h[j].y
	}
	return h[i].x < h[j].x
}
func (h aStarHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *aStarHeap) Push(x interface{})  { *h = append(*h, x.(*aStarNode)) }
func (h *aStarHeap) Pop() interface{} {
	old := *h; n := len(old); x := old[n-1]; *h = old[:n-1]; return x
}

func octileDist(x1, y1, x2, y2 int32) fixed.Fix32 {
	dx := fixed.FromInt(x1 - x2).Abs()
	dy := fixed.FromInt(y1 - y2).Abs()
	if dx < dy { dx, dy = dy, dx }
	return dy.Mul(diagCost).Add(dx.Sub(dy).Mul(straightCost))
}

var aStarDirs = [][2]int32{
	{1, 0}, {-1, 0}, {0, 1}, {0, -1},
	{1, 1}, {-1, -1}, {1, -1}, {-1, 1},
}

func FindPath(w *World, start, goal fixed.Vec2) []fixed.Vec2 {
	sx, sy := start.X.ToInt(), start.Y.ToInt()
	gx, gy := goal.X.ToInt(), goal.Y.ToInt()
	if sx == gx && sy == gy { return nil }

	W, H := w.NavGrid.W, w.NavGrid.H
	if sx < 0 || sy < 0 || sx >= W || sy >= H { return nil }
	if gx < 0 || gy < 0 || gx >= W || gy >= H { return nil }
	if isBlockedCell(w, gx, gy, 0) { return nil }

	startNode := &aStarNode{x: sx, y: sy, f: octileDist(sx, sy, gx, gy)}
	open := &aStarHeap{startNode}
	heap.Init(open)
	closed := make(map[[2]int32]bool)

	for open.Len() > 0 {
		cur := heap.Pop(open).(*aStarNode)
		key := [2]int32{cur.x, cur.y}
		if closed[key] { continue }
		closed[key] = true
		if cur.x == gx && cur.y == gy { return buildPath(cur) }

		for _, d := range aStarDirs {
			nx, ny := cur.x+d[0], cur.y+d[1]
			if nx < 0 || ny < 0 || nx >= W || ny >= H { continue }
			if closed[[2]int32{nx, ny}] { continue }
			if isBlockedCell(w, nx, ny, 0) { continue }

			moveCost := straightCost
			if d[0] != 0 && d[1] != 0 {
				if isBlockedCell(w, cur.x+d[0], cur.y, 0) || isBlockedCell(w, cur.x, cur.y+d[1], 0) {
					continue
				}
				moveCost = diagCost
			}
			ng := cur.g.Add(moveCost)
			heap.Push(open, &aStarNode{x: nx, y: ny, g: ng, f: ng.Add(octileDist(nx, ny, gx, gy)), parent: cur})
		}
	}
	return nil
}

func buildPath(node *aStarNode) []fixed.Vec2 {
	var waypoints []fixed.Vec2
	path := []fixed.Vec2{fixed.VInt(node.x, node.y)}
	for node.parent != nil { node = node.parent; path = append(path, fixed.VInt(node.x, node.y)) }
	for i := len(path) - 2; i >= 0; i-- { waypoints = append(waypoints, path[i]) }
	return waypoints
}

func isBlockedCell(w *World, cx, cy int32, excludeID uint32) bool {
	for i := range w.Buildings {
		b := &w.Buildings[i]
		if b.State == BldDead || b.ID == excludeID { continue }
		bx, by, bs := b.Pos.X.ToInt(), b.Pos.Y.ToInt(), int32(b.SizeCells)
		if cx >= bx && cx < bx+bs && cy >= by && cy < by+bs { return true }
	}
	return false
}

func stepMovePath(w *World, u *Unit) {
	if len(u.Path) == 0 { u.State = UnitIdle; return }
	wp := u.Path[0]
	if u.Pos.DistSq(wp) <= fixed.FromFloat64(4.0) {
		u.Path = u.Path[1:]
		if len(u.Path) == 0 { u.State = UnitIdle; return }
		wp = u.Path[0]
	}
	newPos := fixed.MoveToward(u.Pos, wp, u.Speed)
	newPos.X = newPos.X.Clamp(0, w.MapSizeX)
	newPos.Y = newPos.Y.Clamp(0, w.MapSizeY)
	u.Pos = newPos
}
