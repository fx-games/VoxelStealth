package sim

import "testing"

// ladderWorld is twoRooms with ladders (character.Climber).
type ladderWorld struct{ *fakeWorld }

func (w ladderWorld) Climbable(x, y, z int) bool { return IsLadder(w.Block(x, y, z)) }
func (w ladderWorld) Solid(x, y, z int) bool {
	return !w.Climbable(x, y, z) && w.fakeWorld.Solid(x, y, z)
}

// A guard patrols up a ladder onto a ledge and back down.
func TestNPCClimbsLadder(t *testing.T) {
	fw, m := twoRooms()
	w := ladderWorld{fw}
	for x := 3; x <= 6; x++ { // a ledge along the north wall, its top at y 4: too high to drop off
		for y := 1; y <= 4; y++ {
			for z := 0; z <= 1; z++ {
				w.SetBlock(x, y, z, "minecraft:stone")
			}
		}
	}
	for x := 2; x <= 7; x++ { // open to the sky above it
		for z := 0; z <= 3; z++ {
			if x < 3 || x > 6 || z > 1 {
				w.SetBlock(x, 4, z, BlockAir)
			}
		}
	}
	for y := 1; y <= 4; y++ {
		w.SetBlock(5, y, 2, "minecraft:ladder[facing=south]")
	}
	g := guard(Vec3{5.5, 1, 6.5}, 0)
	g.Patrol = []Waypoint{{Pos: Vec3{3.5, 5, 0.5}, Wait: 20}, {Pos: Vec3{5.5, 1, 6.5}, Wait: 20}}
	m.NPCs = []*NPCDef{g}
	s := New(w, m)
	s.Player.Pos = Vec3{18.5, 1, 9.5} // out of sight, next door
	n := s.NPCs[0]
	top, down := false, false
	for i := 0; i < 20*TicksPerSecond; i++ {
		s.Tick()
		if n.pos().DistXZ(Vec3{3.5, 5, 0.5}) < 0.6 && n.pos().Y > 4.9 {
			top = true
		}
		if top && n.pos().Cell() == (Cell{5, 3, 2}) { // on the rungs on the way down
			down = true
		}
		if down && n.PatrolI == 0 && n.pos().Y < 1.1 && n.pos().DistXZ(Vec3{5.5, 1, 6.5}) < 0.6 {
			return
		}
	}
	t.Fatalf("reached the ledge %v, climbed down %v; guard at %v", top, down, n.pos())
}
