package sim

import (
	"math"
	"os"
	"testing"
)

// fakeWorld is a map of blocks with naive light: block light falls off by
// one per block (Manhattan) from light blocks within 14, no walls.
type fakeWorld struct{ b map[Cell]string }

func (w *fakeWorld) Block(x, y, z int) string {
	if n, ok := w.b[Cell{x, y, z}]; ok {
		return n
	}
	return BlockAir
}
func (w *fakeWorld) Solid(x, y, z int) bool {
	n := w.Block(x, y, z)
	return n != BlockAir && n != BlockLight
}
func (w *fakeWorld) SetBlock(x, y, z int, n string) {
	if n == BlockAir {
		delete(w.b, Cell{x, y, z})
		return
	}
	w.b[Cell{x, y, z}] = n
}
func (w *fakeWorld) Light(x, y, z int) (uint8, uint8) {
	best := 0
	for c, n := range w.b {
		if n == BlockLight {
			d := abs(c.X-x) + abs(c.Y-y) + abs(c.Z-z)
			best = max(best, 14-d)
		}
	}
	return 0, uint8(best)
}
func abs(a int) int { return max(a, -a) }

// twoRooms builds a stone floor at y=0 (x,z in 0..20), room A x 0..9 and
// room B x 11..20 split by a wall at x=10 with a door at z=5.
func twoRooms() (*fakeWorld, *Mission) {
	w := &fakeWorld{b: map[Cell]string{}}
	for x := -1; x <= 21; x++ {
		for z := -1; z <= 11; z++ {
			w.b[Cell{x, 0, z}] = "minecraft:stone"
			w.b[Cell{x, 4, z}] = "minecraft:stone"
			for y := 1; y <= 3; y++ {
				if x == -1 || x == 21 || z == -1 || z == 11 || x == 10 {
					w.b[Cell{x, y, z}] = "minecraft:stone_bricks"
				}
			}
		}
	}
	delete(w.b, Cell{10, 1, 5})
	delete(w.b, Cell{10, 2, 5})
	m := &Mission{TimeScale: 1, StartTime: 0, Start: Vec3{2.5, 1, 2.5}, Ammo: map[string]int{AmmoWater: 3, AmmoExplosive: 1},
		Rooms: []*Room{
			{ID: 1, Name: "A", Boxes: []Box{{Cell{0, 1, 0}, Cell{9, 3, 10}}}},
			{ID: 2, Name: "B", Boxes: []Box{{Cell{11, 1, 0}, Cell{20, 3, 10}}}},
		},
		Objects: []*Object{{ID: "door", Kind: KindDoor, Pos: Vec3{10.5, 1, 5.5}}},
	}
	return w, m
}

func run(s *Sim, ticks int) {
	for i := 0; i < ticks; i++ {
		s.Tick()
	}
}

func TestSoundThroughDoor(t *testing.T) {
	w, m := twoRooms()
	s := New(w, m)
	if len(s.Rooms.Portals) != 1 {
		t.Fatalf("portals = %d, want 1", len(s.Rooms.Portals))
	}
	src, at := Vec3{5, 1.5, 5}, Vec3{15, 1.5, 5}
	closed, _ := s.Rooms.heard(s.Rooms.spread(src, s.doorClosed), 40, at)
	s.setDoor(0, true)
	open, _ := s.Rooms.heard(s.Rooms.spread(src, s.doorClosed), 40, at)
	if !(open > closed+10) {
		t.Fatalf("open door %.1f should be much louder than closed %.1f", open, closed)
	}
}

func TestVisibilityAndWaterArrow(t *testing.T) {
	w, m := twoRooms()
	m.Objects = append(m.Objects, &Object{ID: "t", Kind: KindTorch, Pos: Vec3{3.5, 2, 2.5}, Lit: true})
	s := New(w, m)
	s.Tick()
	lit := s.Visibility
	s.ArrowHit(AmmoWater, Vec3{3.5, 2.5, 2.5}, -1, false)
	s.Tick()
	if !(lit > 0.5 && s.Visibility < 0.05) {
		t.Fatalf("visibility lit %.2f, doused %.2f", lit, s.Visibility)
	}
}

func guard(pos Vec3, yaw float64) *NPCDef {
	return &NPCDef{ID: "g", Kind: "guard", Pos: pos, Yaw: yaw, Patrol: []Waypoint{{Pos: pos, Wait: 1000, Yaw: yaw, Face: true}}}
}

func TestGuardSeesLitPlayerNotDark(t *testing.T) {
	for _, lit := range []bool{true, false} {
		w, m := twoRooms()
		m.NPCs = []*NPCDef{guard(Vec3{2.5, 1, 9.5}, 0)} // facing -Z, toward the player
		m.Objects = append(m.Objects, &Object{ID: "t", Kind: KindTorch, Pos: Vec3{2.5, 2, 1.5}, Lit: lit})
		s := New(w, m)
		s.Player.OnGround = true
		run(s, 60)
		st := s.NPCs[0].State
		if lit && st != StateCombat {
			t.Fatalf("lit: guard state %v, want combat", st)
		}
		if !lit && st != StatePatrol {
			t.Fatalf("dark: guard state %v (suspicion %.1f), want patrol", st, s.NPCs[0].Suspicion)
		}
	}
}

func TestNoisemakerDrawsGuard(t *testing.T) {
	w, m := twoRooms()
	m.NPCs = []*NPCDef{guard(Vec3{15.5, 1, 8.5}, 0)}
	s := New(w, m)
	s.Player.Pos = Vec3{1.5, 1, 9.5} // dark corner of room A
	s.setDoor(0, true)
	s.ArrowHit(AmmoNoise, Vec3{7.5, 1, 5.5}, -1, false)
	run(s, 12*TicksPerSecond)
	g := s.NPCs[0]
	if g.State != StateInvestigate && g.State != StateSuspicious || g.pos().X > 11 {
		t.Fatalf("guard %v at %.1f,%.1f; want investigating in room A", g.State, g.pos().X, g.pos().Z)
	}
}

func TestLampInPuddleAccident(t *testing.T) {
	w, m := twoRooms()
	m.NPCs = []*NPCDef{{ID: "lord", Kind: "target", Pos: Vec3{5.5, 1, 5.5}, Yaw: math.Pi}}
	m.Objects = append(m.Objects,
		&Object{ID: "f", Kind: KindFaucet, Pos: Vec3{5.5, 1, 6.5}, Running: true},
		&Object{ID: "l", Kind: KindLamp, Pos: Vec3{7.5, 1, 5.5}, Lit: true})
	m.Objectives = []Objective{{Kind: "accident", Target: "lord"}}
	s := New(w, m)
	s.Player.Pos = Vec3{8.5, 1, 5.5} // knock the lamp toward -X, into the water
	s.Player.Ammo = map[string]int{}
	run(s, 15*TicksPerSecond)
	if _, wet := s.Puddles[Cell{5, 1, 5}]; !wet {
		t.Fatal("no puddle under the target")
	}
	if _, wet := s.Puddles[Cell{6, 1, 5}]; !wet {
		s.Puddles[Cell{6, 1, 5}] = 0 // make sure the lamp lands in water
	}
	s.Frob(Target{Obj: 2, NPC: -1}, false)
	run(s, 5)
	if s.NPCs[0].State != StateDead || s.NPCs[0].Death != "accident" || !s.Done[0] {
		t.Fatalf("target %v (%s), objective %v", s.NPCs[0].State, s.NPCs[0].Death, s.Done[0])
	}
}

func TestTorchOnRugFire(t *testing.T) {
	w, m := twoRooms()
	m.NPCs = []*NPCDef{guard(Vec3{3.5, 1, 9.5}, 0)}
	m.Objects = append(m.Objects,
		&Object{ID: "t", Kind: KindTorch, Pos: Vec3{3.5, 2, 3.5}, Lit: true},
		&Object{ID: "rug", Kind: KindFlammable, Pos: Vec3{3.5, 1, 3.5}})
	s := New(w, m)
	s.Player.Pos = Vec3{6.5, 1, 3.5}
	s.Frob(Target{Obj: 1, NPC: -1}, false) // take it: its light comes along
	run(s, 2)
	if o := s.Objects[1]; !o.Held || o.LightCell != (Cell{6, 2, 3}) {
		t.Fatalf("torch not carried: held %v light %v", o.Held, o.LightCell)
	}
	s.ThrowTorch(s.Player.Eye(), Vec3{-1, 0, 0}, 3) // onto the rug
	s.Player.Pos = Vec3{18.5, 1, 9.5}
	run(s, 2)
	if !s.Objects[2].Burning() {
		t.Fatal("rug not burning")
	}
	run(s, 3*TicksPerSecond)
	if g := s.NPCs[0]; g.State != StateInvestigate {
		t.Fatalf("guard %v; want investigating the fire", g.State)
	}
}

func TestBlackjackAndBodyFound(t *testing.T) {
	w, m := twoRooms()
	a := guard(Vec3{5.5, 1, 5.5}, 0)
	b := guard(Vec3{5.5, 1, 9.5}, math.Pi) // facing +Z, away from a
	b.ID = "g2"
	m.NPCs = []*NPCDef{a, b}
	m.Objects = append(m.Objects, &Object{ID: "t", Kind: KindTorch, Pos: Vec3{6.5, 2, 6.5}, Lit: true})
	s := New(w, m)
	s.Player.Pos, s.Player.Yaw = Vec3{5.5, 1, 7}, 0 // behind a, facing -Z
	if !s.Melee("blackjack") || s.NPCs[0].State != StateUnconscious {
		t.Fatalf("knockout failed: %v", s.NPCs[0].State)
	}
	s.Player.Pos = Vec3{18.5, 1, 1.5}
	s.NPCs[1].Yaw = 0 // turns around
	run(s, 10)
	if s.NPCs[1].State != StateSearch || s.Stats.BodiesFound != 1 {
		t.Fatalf("guard 2 %v, bodies %d", s.NPCs[1].State, s.Stats.BodiesFound)
	}
}

func TestExplosionBreaksWall(t *testing.T) {
	w, m := twoRooms()
	s := New(w, m)
	s.Explode(Vec3{10, 1.5, 2.5})
	if w.Solid(10, 1, 2) {
		t.Fatal("wall still there")
	}
	if w.Block(5, 0, 5) == BlockAir {
		t.Fatal("blast too big")
	}
}

func TestMissionRoundTrip(t *testing.T) {
	_, m := twoRooms()
	m.Title = "Test"
	m.NPCs = []*NPCDef{guard(Vec3{1, 2, 3}, 0.5)}
	m.Objectives = []Objective{{Kind: "kill", Target: "g", Text: "Kill him"}}
	dir := t.TempDir()
	if err := m.Save(dir); err != nil {
		t.Fatal(err)
	}
	got, err := LoadMission(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Test" || len(got.NPCs) != 1 || len(got.Rooms) != 2 || len(got.Objects) != 1 || got.Objectives[0].Target != "g" {
		t.Fatalf("round trip lost data: %+v", got)
	}
	_ = os.Remove
}
