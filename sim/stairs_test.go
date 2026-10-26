package sim

import (
	"strings"
	"testing"

	"github.com/memmaker/voxellib/voxel/collide"
)

// stairWorld is a fakeWorld whose _stairs blocks collide as east-climbing
// stairs (sim.Shaper): a bottom slab and the east half above it.
type stairWorld struct{ *fakeWorld }

func (w stairWorld) Shaped(x, y, z int) bool { return strings.HasSuffix(w.Block(x, y, z), "_stairs") }

func (w stairWorld) Boxes(area collide.AABB, out []collide.AABB) []collide.AABB {
	area.Grow(0, 0.5, 0).Blocks(func(x, y, z int) {
		if !w.Shaped(x, y, z) {
			return
		}
		fx, fy, fz := float64(x), float64(y), float64(z)
		for _, b := range []collide.AABB{
			{MinX: fx, MinY: fy, MinZ: fz, MaxX: fx + 1, MaxY: fy + 0.5, MaxZ: fz + 1},
			{MinX: fx + 0.5, MinY: fy + 0.5, MinZ: fz, MaxX: fx + 1, MaxY: fy + 1, MaxZ: fz + 1},
		} {
			if b.Intersects(area) {
				out = append(out, b)
			}
		}
	})
	return out
}

// An NPC walks a patrol up a flight of stairs and back down.
func TestNPCWalksStairs(t *testing.T) {
	w := &fakeWorld{b: map[Cell]string{}}
	for x := -1; x <= 12; x++ {
		for z := -1; z <= 5; z++ {
			w.b[Cell{x, 0, z}] = "minecraft:stone"
			w.b[Cell{x, 9, z}] = "minecraft:stone"
			for y := 1; y <= 8; y++ {
				if x == -1 || x == 12 || z == -1 || z == 5 {
					w.b[Cell{x, y, z}] = "minecraft:stone"
				}
			}
			if x >= 6 && z >= 0 && z <= 4 && x < 12 {
				w.b[Cell{x, 1, z}], w.b[Cell{x, 2, z}], w.b[Cell{x, 3, z}] = "minecraft:stone", "minecraft:stone", "minecraft:stone"
			}
		}
	}
	for i := 0; i < 3; i++ {
		for z := 2; z <= 3; z++ {
			for y := 1; y <= i; y++ {
				w.b[Cell{3 + i, y, z}] = "minecraft:stone"
			}
			w.b[Cell{3 + i, 1 + i, z}] = "minecraft:stone_stairs"
		}
	}
	up, down := Vec3{9.5, 4, 2.5}, Vec3{1.5, 1, 2.5}
	m := &Mission{NPCs: []*NPCDef{{ID: "w", Kind: "servant", Pos: down, Patrol: []Waypoint{{Pos: down}, {Pos: up}}}}}
	s := New(stairWorld{w}, m)
	s.Player.Pos = Vec3{1.5, 1, 0.5}
	top, back := false, false
	for range 40 * TicksPerSecond {
		s.Tick()
		p := s.NPCs[0].pos()
		top = top || p.Dist(up) < 1
		back = back || top && p.Dist(down) < 1
	}
	if !top || !back {
		t.Fatalf("up %v, back down %v; at %+v", top, back, s.NPCs[0].pos())
	}
}

func (w stairWorld) StairOrSlab(x, y, z int) bool { return w.Shaped(x, y, z) }
func (w stairWorld) Air(x, y, z int) bool         { return w.Block(x, y, z) == BlockAir }
