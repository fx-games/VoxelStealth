package sim

import (
	"math"
	"testing"
)

// balcony is a plank balcony at y 3 over x 4..6, z 4..6 of room A, held
// up by two log posts at 4 and 6,1..2,7.
func balcony() (*fakeWorld, *Mission) {
	w, m := twoRooms()
	for x := 4; x <= 6; x++ {
		for z := 4; z <= 6; z++ {
			w.SetBlock(x, 3, z, "minecraft:spruce_planks")
		}
	}
	var posts []Cell
	for _, x := range []int{4, 6} {
		for y := 1; y <= 2; y++ {
			w.SetBlock(x, y, 7, "minecraft:stripped_oak_log[axis=y]")
			posts = append(posts, Cell{x, y, 7})
		}
	}
	m.Structures = []*Structure{{ID: "balcony", Supports: posts,
		Box: Box{Cell{4, 3, 4}, Cell{6, 3, 6}}}}
	return w, m
}

func kickAt(s *Sim, c Cell) {
	pr := s.playerR()
	s.Stimulate(Stimulus{Stim: StimKick, Medium: "radius", Origin: c.Center(), Range: 0.3, Intensity: 1, Source: &pr,
		Extra: map[string]any{"yaw": 0.0}})
}

func TestBalconyFallsOnGuardWhenSupportsBreak(t *testing.T) {
	w, m := balcony()
	m.NPCs = []*NPCDef{guard(Vec3{5.5, 1, 5.5}, 0)}
	s := New(w, m)
	kickAt(s, Cell{4, 1, 7})
	if s.Strain(Cell{4, 1, 7}) == 0 || w.Block(4, 1, 7) == BlockAir {
		t.Fatalf("one kick: strain %v, block %q", s.Strain(Cell{4, 1, 7}), w.Block(4, 1, 7))
	}
	kickAt(s, Cell{4, 1, 7})
	if w.Block(4, 1, 7) != BlockAir || w.Block(4, 2, 7) != BlockAir || s.Collapsed["balcony"] {
		t.Fatal("two kicks: the first post should go whole and the other still hold")
	}
	kickAt(s, Cell{6, 1, 7})
	kickAt(s, Cell{6, 1, 7})
	if !s.Collapsed["balcony"] || w.Block(5, 3, 5) != BlockAir {
		t.Fatal("balcony didn't come down with its last support")
	}
	run(s, 2*TicksPerSecond)
	if n := s.NPCs[0]; n.State() != StateDead || n.Death != "accident" {
		t.Fatalf("guard under it: %v (%s)", n.State(), n.Death)
	}
	if len(s.Falls) != 0 || w.Block(4, 1, 4) != "minecraft:spruce_planks" || w.Block(5, 1, 5) != BlockAir {
		t.Fatalf("rubble: falls %d, 4,1,4 %q, under the body %q", len(s.Falls), w.Block(4, 1, 4), w.Block(5, 1, 5))
	}
}

func TestBurntSupportBringsBalconyDown(t *testing.T) {
	w, m := balcony()
	s := New(w, m)
	if got := (simWorld{s}).ArchetypeAt(Cell{6, 2, 7}); got != "support" {
		t.Fatalf("support cell is %q", got)
	}
	kickAt(s, Cell{4, 1, 7})
	kickAt(s, Cell{4, 1, 7})
	s.W.SetBlock(6, 2, 7, BlockAir) // eaten by fire
	supportBroken(s, Cell{6, 2, 7}, Delivery{})
	if !s.Collapsed["balcony"] {
		t.Fatal("no collapse")
	}
}

func TestKickedIntoWallSpikesPinned(t *testing.T) {
	w, m := twoRooms()
	for y := 1; y <= 2; y++ { // spikes on the west wall, pointing east
		w.SetBlock(0, y, 5, "minecraft:iron_spikes[facing=east]")
	}
	m.NPCs = []*NPCDef{guard(Vec3{2.5, 1, 5.5}, math.Pi/2)}
	s := New(w, m)
	s.Player.Pos, s.Player.Yaw = Vec3{4.2, 1, 5.5}, -math.Pi/2 // facing west, at the guard
	if !s.Kick() {
		t.Fatal("kick missed")
	}
	run(s, 2*TicksPerSecond)
	n := s.NPCs[0]
	pinned, _, yaw := s.Pinned(0)
	if n.State() != StateDead || !pinned || math.Abs(yaw+math.Pi/2) > 0.01 {
		t.Fatalf("guard %v pinned %v yaw %v at %v", n.State(), pinned, yaw, n.pos())
	}
}
