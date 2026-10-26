package sim

import (
	"container/heap"
	"math"
)

// Rooms and portals, after the Dark Engine's room database (concept only,
// see docs/references.md): designers place room boxes; sound travels from
// room to room through the openings between them and loses loudness with
// path length, and closed doors in an opening muffle it. Everything not in
// a room is room 0, "outdoors".

// Box is an inclusive cell box.
type Box struct{ Min, Max Cell }

func (b Box) Contains(c Cell) bool {
	return c.X >= b.Min.X && c.X <= b.Max.X && c.Y >= b.Min.Y && c.Y <= b.Max.Y && c.Z >= b.Min.Z && c.Z <= b.Max.Z
}

// Room is a named set of boxes. IDs start at 1.
type Room struct {
	ID    int
	Name  string
	Boxes []Box
}

// Portal connects two rooms. Door is the index of the door object in the
// opening, or -1.
type Portal struct {
	A, B int
	Pos  Vec3
	Door int
	n    int // cells averaged into Pos
}

// RoomMap answers "which room is this cell in" and holds the portal graph.
type RoomMap struct {
	Rooms   []*Room
	Portals []*Portal
	cells   map[Cell]int
	adj     map[int][]int // room → portal indices
}

// Sound units lost per block of path, and extra loss through a closed door.
const (
	SoundFalloff = 1.6
	DoorMuffle   = 18.0
)

// BuildRooms indexes the room boxes and finds the portals: neighbouring open
// cells in different rooms, or open cells on both sides of a door.
func BuildRooms(w World, rooms []*Room, doors map[Cell]int) *RoomMap {
	rm := &RoomMap{Rooms: rooms, cells: map[Cell]int{}, adj: map[int][]int{}}
	for _, r := range rooms {
		for _, b := range r.Boxes {
			for x := b.Min.X; x <= b.Max.X; x++ {
				for y := b.Min.Y; y <= b.Max.Y; y++ {
					for z := b.Min.Z; z <= b.Max.Z; z++ {
						rm.cells[Cell{x, y, z}] = r.ID
					}
				}
			}
		}
	}
	type key struct{ a, b, door int }
	found := map[key]*Portal{}
	open := func(c Cell) bool { _, d := doors[c]; return !d && !w.Solid(c.X, c.Y, c.Z) }
	dirs := []Cell{{1, 0, 0}, {-1, 0, 0}, {0, 1, 0}, {0, -1, 0}, {0, 0, 1}, {0, 0, -1}}
	for c, a := range rm.cells {
		if !open(c) {
			continue
		}
		for _, d := range dirs {
			n := c.Add(d.X, d.Y, d.Z)
			door := -1
			if di, ok := doors[n]; ok { // look through the door
				door = di
				n = n.Add(d.X, d.Y, d.Z)
			}
			if !open(n) {
				continue
			}
			b := rm.cells[n]
			if b == a {
				continue // pairs seen from both sides just average in twice
			}
			k := key{min(a, b), max(a, b), door}
			p := found[k]
			if p == nil {
				p = &Portal{A: k.a, B: k.b, Door: door}
				found[k] = p
			}
			mid := c.Center().Add(n.Center()).Scale(0.5)
			p.Pos = p.Pos.Scale(float64(p.n)).Add(mid).Scale(1 / float64(p.n+1))
			p.n++
		}
	}
	for _, p := range found {
		i := len(rm.Portals)
		rm.Portals = append(rm.Portals, p)
		rm.adj[p.A] = append(rm.adj[p.A], i)
		rm.adj[p.B] = append(rm.adj[p.B], i)
	}
	return rm
}

// RoomAt returns the room id of a cell (0 = outdoors).
func (rm *RoomMap) RoomAt(c Cell) int { return rm.cells[c] }

// Name returns a room's name.
func (rm *RoomMap) Name(id int) string {
	for _, r := range rm.Rooms {
		if r.ID == id {
			return r.Name
		}
	}
	return "outdoors"
}

// soundField is a sound spread over the room graph: per room, the path
// length to the nearest entry point and that point.
type soundField struct {
	dist  map[int]float64
	entry map[int]Vec3
}

type pqItem struct {
	room int
	d    float64
}
type pq []pqItem

func (q pq) Len() int           { return len(q) }
func (q pq) Less(i, j int) bool { return q[i].d < q[j].d }
func (q pq) Swap(i, j int)      { q[i], q[j] = q[j], q[i] }
func (q *pq) Push(x any)        { *q = append(*q, x.(pqItem)) }
func (q *pq) Pop() any          { o := *q; it := o[len(o)-1]; *q = o[:len(o)-1]; return it }

// spread runs Dijkstra from the source over rooms. closed reports whether
// a door is shut.
func (rm *RoomMap) spread(src Vec3, closed func(door int) bool) soundField {
	f := soundField{dist: map[int]float64{}, entry: map[int]Vec3{}}
	r0 := rm.RoomAt(src.Cell())
	f.dist[r0], f.entry[r0] = 0, src
	q := &pq{{r0, 0}}
	for q.Len() > 0 {
		it := heap.Pop(q).(pqItem)
		if it.d > f.dist[it.room] {
			continue
		}
		from := f.entry[it.room]
		for _, pi := range rm.adj[it.room] {
			p := rm.Portals[pi]
			other := p.A
			if other == it.room {
				other = p.B
			}
			d := it.d + from.Dist(p.Pos)
			if p.Door >= 0 && closed(p.Door) {
				d += DoorMuffle / SoundFalloff
			}
			if old, ok := f.dist[other]; !ok || d < old {
				f.dist[other], f.entry[other] = d, p.Pos
				heap.Push(q, pqItem{other, d})
			}
		}
	}
	return f
}

// pathLen is how far a sound travels to at, closed doors counted as the
// blocks of path they muffle as much as (+Inf: it doesn't get there), and
// where it seems to come from: the source in the same room, else the
// opening it came in by.
func (rm *RoomMap) pathLen(f soundField, at Vec3) (float64, Vec3) {
	r := rm.RoomAt(at.Cell())
	d, ok := f.dist[r]
	if !ok {
		return math.Inf(1), Vec3{}
	}
	return d + f.entry[r].Dist(at), f.entry[r]
}

// heard returns the loudness reaching a listener and where it seems to
// come from (the source if in the same room, else the last opening).
func (rm *RoomMap) heard(f soundField, loud float64, at Vec3) (float64, Vec3) {
	r := rm.RoomAt(at.Cell())
	d, ok := f.dist[r]
	if !ok {
		return math.Inf(-1), Vec3{}
	}
	e := f.entry[r]
	return loud - SoundFalloff*(d+e.Dist(at)), e
}

// Hear returns how loud a sound of loudness loud at src is at the listener
// and where it seems to come from, travelling through rooms and doors as
// the guards hear it. The game plays the player's sounds the same way.
func (s *Sim) Hear(src Vec3, loud float64, at Vec3) (float64, Vec3) {
	return s.Rooms.heard(s.Rooms.spread(src, s.doorClosed), loud, at)
}
