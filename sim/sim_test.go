package sim

import (
	"math"
	"os"
	"testing"

	"github.com/memmaker/voxellib/character"
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
	return n != BlockAir && n != BlockLight && n != BlockPuddle && n != BlockOil
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

func TestGuardSeesThroughGlassAndHearsItBreak(t *testing.T) {
	w, m := twoRooms()
	for z := 1; z <= 3; z++ {
		for y := 1; y <= 3; y++ {
			w.b[Cell{10, y, z}] = "minecraft:glass"
		}
	}
	m.NPCs = []*NPCDef{guard(Vec3{15.5, 1, 2.5}, -math.Pi/2)} // facing -X, at the window
	m.Objects = append(m.Objects, &Object{ID: "t", Kind: KindTorch, Pos: Vec3{6.5, 2, 2.5}, Lit: true})
	s := New(w, m)
	s.Player.Pos, s.Player.OnGround = Vec3{7.5, 1, 2.5}, true
	run(s, 60)
	if st := s.NPCs[0].State; st != StateCombat {
		t.Fatalf("guard behind glass: %v, want combat", st)
	}
	if s.TargetAt(Vec3{9.5, 2.5, 2.5}, Vec3{1, 0, 0}).NPC >= 0 {
		t.Fatal("reached through glass")
	}

	w, m = twoRooms()
	w.b[Cell{10, 2, 2}] = "minecraft:glass"
	m.NPCs = []*NPCDef{guard(Vec3{18.5, 1, 9.5}, 0)} // far corner, back turned
	s = New(w, m)
	s.Player.Pos = Vec3{1.5, 1, 9.5}
	s.ArrowHit(AmmoBroadhead, Vec3{10.2, 2.5, 2.5}, -1, false)
	if w.Block(10, 2, 2) != BlockAir {
		t.Fatal("glass not broken")
	}
	run(s, 2)
	if g := s.NPCs[0]; g.State == StatePatrol {
		t.Fatalf("guard didn't react to breaking glass (suspicion %.1f)", g.Suspicion)
	}
}

func TestTorchOnSkipsGlass(t *testing.T) {
	w := &fakeWorld{b: map[Cell]string{{-1, 0, 0}: "minecraft:glass", {1, 0, 0}: "minecraft:stone"}}
	if on := TorchOn(TorchWall(w), Cell{}); on != "east" {
		t.Fatalf("torch hangs on %q, want east (glass is west)", on)
	}
	w.b[Cell{1, 0, 0}] = "minecraft:glass"
	if on := TorchOn(TorchWall(w), Cell{}); on != "floor" {
		t.Fatalf("torch hangs on %q, want floor", on)
	}
}

func TestDecorBlocksButIsNotTargeted(t *testing.T) {
	w, m := twoRooms()
	m.Objects = append(m.Objects, &Object{ID: "box", Kind: KindDecor, Name: "Small box", Pos: Vec3{5.5, 1, 2.5}})
	DecorSize["Small box"] = Vec3{0.25, 0.4, 0.25}
	defer delete(DecorSize, "Small box")
	s := New(w, m)
	p := character.New(2.5, 1, 2.5)
	for i := 0; i < 3*TicksPerSecond; i++ {
		p.Tick(s.Body(), character.Input{Forward: 1, Yaw: math.Pi / 2}) // walk +X into the box
	}
	if want := 5.25 - p.Width/2; math.Abs(p.X-want) > 0.01 {
		t.Fatalf("player at x=%.2f, want stopped against the box at %.2f", p.X, want)
	}
	if tg := s.TargetAt(Vec3{4.5, 1.2, 2.5}, Vec3{1, 0, 0}); tg.Obj >= 0 {
		t.Fatalf("targeted object %d", tg.Obj)
	}
}

func TestPiercedBarrelLeaks(t *testing.T) {
	w, m := twoRooms()
	m.Objects = append(m.Objects,
		&Object{ID: "barrel", Kind: KindDecor, Name: "Barrel", Pos: Vec3{5.5, 1, 2.5}},
		&Object{ID: "torch", Kind: KindTorch, Lit: true, On: "floor", Pos: Vec3{6.5, 1, 2.5}})
	s := New(w, m)
	torch := s.Objects[len(s.Objects)-1]
	s.ArrowHit(AmmoWater, Vec3{5.5, 1.5, 2}, -1, false)
	if n := len(s.Puddles); n != 9 {
		t.Fatalf("water arrow: %d puddle cells, want its own 9", n)
	}
	torch.Lit = true
	clear(s.Puddles)
	s.Player.Pos, s.Player.Yaw, s.Player.Pitch = Vec3{4.5, 1, 2.5}, math.Pi/2, -0.6 // looking at it
	s.Melee("blackjack")
	if len(s.Puddles) != 0 {
		t.Fatalf("blackjack leaked %d cells", len(s.Puddles))
	}
	s.Melee("blade")
	if len(s.Puddles) < 2 {
		t.Fatalf("stabbed: puddle of %d cells, want a few", len(s.Puddles))
	}
	if torch.Lit {
		t.Fatal("the leak didn't douse the torch beside the barrel")
	}
	clear(s.Puddles)
	s.ArrowHit(AmmoBroadhead, Vec3{5.5, 1.5, 2}, -1, false)
	if len(s.Puddles) < 2 {
		t.Fatalf("shot: puddle of %d cells, want a few", len(s.Puddles))
	}
}

func TestGlassBreaksOnPierceBashBlast(t *testing.T) {
	w, m := twoRooms()
	s := New(w, m)
	for _, c := range []Cell{{6, 2, 2}, {6, 2, 6}, {12, 2, 8}} {
		w.SetBlock(c.X, c.Y, c.Z, "minecraft:glass")
	}
	s.ArrowHit(AmmoBroadhead, Vec3{6.2, 2.5, 2.5}, -1, false)
	s.Player.Pos, s.Player.Yaw, s.Player.Pitch = Vec3{5.5, 1, 6.5}, math.Pi/2, 0 // facing the pane at eye height
	s.Melee("blackjack")
	s.Explode(Vec3{12.5, 1.5, 6.5})
	for _, c := range []Cell{{6, 2, 2}, {6, 2, 6}, {12, 2, 8}} {
		if b := w.Block(c.X, c.Y, c.Z); b != BlockAir {
			t.Errorf("%v is still %s", c, b)
		}
	}
	w.SetBlock(6, 2, 2, "minecraft:glass")
	s.ArrowHit(AmmoWater, Vec3{6.2, 2.5, 2.5}, -1, false)
	if w.Block(6, 2, 2) == BlockAir {
		t.Error("a water arrow broke glass")
	}
}

func TestOilBurnsAndWaterPutsItOut(t *testing.T) {
	w, m := twoRooms()
	m.NPCs = []*NPCDef{guard(Vec3{5.5, 1, 3.5}, 0)}
	m.Objects = append(m.Objects, &Object{ID: "oil", Kind: KindDecor, Name: "Oil barrel", Pos: Vec3{5.5, 1, 2.5}})
	s := New(w, m)
	s.ArrowHit(AmmoBroadhead, Vec3{5.5, 1.5, 2}, -1, false)
	if len(s.Oil) < 3 || len(s.Puddles) != 0 {
		t.Fatalf("oil %d cells, water %d", len(s.Oil), len(s.Puddles))
	}
	s.NPCs[0].Body.Teleport(5.5, 1, 2.5)                                         // standing in it
	s.Stimulate(Stimulus{Stim: StimFire, Pos: Vec3{5.5, 1.1, 2.5}, Radius: 0.3}) // a spark
	run(s, 2*TicksPerSecond)
	if s.NPCs[0].State != StateDead || s.NPCs[0].Death != "accident" {
		t.Fatalf("guard %v (%s), want burned", s.NPCs[0].State, s.NPCs[0].Death)
	}
	burning := 0
	for _, f := range s.Oil {
		if f > 0 {
			burning++
		}
	}
	if burning < 3 {
		t.Fatalf("fire spread to %d of %d oil cells", burning, len(s.Oil))
	}
	s.Stimulate(Stimulus{Stim: StimWater, Pos: Vec3{5.5, 1.1, 2.5}, Radius: 3})
	for c, f := range s.Oil {
		if f > 0 {
			t.Fatalf("%v still burning after water", c)
		}
	}
}

func TestBombBurstsOilBarrelIntoFire(t *testing.T) {
	w, m := twoRooms()
	m.Objects = append(m.Objects, &Object{ID: "oil", Kind: KindDecor, Name: "Oil barrel", Pos: Vec3{8.5, 1, 2.5}})
	s := New(w, m)
	s.Explode(Vec3{5.5, 1.2, 2.5})
	burning := 0
	for _, f := range s.Oil {
		if f > 0 {
			burning++
		}
	}
	if !s.Objects[len(s.Objects)-1].Gone || burning == 0 {
		t.Fatalf("barrel gone %v, %d of %d oil cells burning", s.Objects[len(s.Objects)-1].Gone, burning, len(s.Oil))
	}
}

func TestWaterArrowPuddle(t *testing.T) {
	for _, hit := range []Vec3{{4.5, 1.1, 4.5}, {9.6, 2.8, 2.5}} { // floor; wall at x=10, up high
		w, m := twoRooms()
		s := New(w, m)
		s.ArrowHit(AmmoWater, hit, -1, false)
		if len(s.Puddles) != 9 {
			t.Fatalf("hit at %v: %d puddle cells, want 9", hit, len(s.Puddles))
		}
		for c := range s.Puddles {
			if c.Y != 1 {
				t.Fatalf("hit at %v: puddle cell %v off the floor", hit, c)
			}
		}
	}
}

func TestRopeArrowHangsToFloor(t *testing.T) {
	w, m := twoRooms()
	s := New(w, m)
	// Shot level along +X into the wall at x=10, 3 blocks up.
	if !s.AddRope(Vec3{10.25, 3.5, 2.5}, math.Pi/2, 0) {
		t.Fatal("no rope")
	}
	r := s.Ropes[0]
	if math.Abs(r.Top.X-9.47) > 0.01 || math.Abs(r.Len-(3.5-1-0.3)) > 0.01 {
		t.Fatalf("rope top %v len %.2f", r.Top, r.Len)
	}
	if s.AddRope(Vec3{4.5, 0.75, 4.5}, 0, -math.Pi/2) { // straight down into the floor
		t.Fatal("rope from an arrow in the floor")
	}
}
