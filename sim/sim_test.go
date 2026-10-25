package sim

import (
	"math"
	"math/rand"
	"os"
	"strings"
	"testing"

	"github.com/memmaker/stimres"
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
	return n != BlockAir && n != BlockLight && n != BlockPuddle && n != BlockOil && !OpenDoor(n)
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

func spilling(s *Sim) bool {
	for _, c := range s.B.Addresses() {
		if s.B.Float(cellR(c), "spill_left") > 0 {
			return true
		}
	}
	return false
}

// spread lets every spill finish pouring.
func spread(s *Sim) {
	for len(s.B.Addresses()) > 0 && spilling(s) {
		s.B.Step()
	}
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
	m.Objects = append(m.Objects, &Object{ID: "t", Kind: KindTorch, Pos: Vec3{3.5, 2, 2.5}, StartLit: true})
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
		m.Objects = append(m.Objects, &Object{ID: "t", Kind: KindTorch, Pos: Vec3{2.5, 2, 1.5}, StartLit: lit})
		s := New(w, m)
		s.Player.OnGround = true
		run(s, 60)
		st := s.NPCs[0].State()
		if lit && st != StateCombat {
			t.Fatalf("lit: guard state %v, want combat", st)
		}
		if !lit && st != StatePatrol {
			t.Fatalf("dark: guard state %v (suspicion %.1f), want patrol", st, s.NPCs[0].Suspicion())
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
	if g.State() != StateInvestigate && g.State() != StateSuspicious || g.pos().X > 11 {
		t.Fatalf("guard %v at %.1f,%.1f; want investigating in room A", g.State(), g.pos().X, g.pos().Z)
	}
}

func TestLampInPuddleAccident(t *testing.T) {
	w, m := twoRooms()
	m.NPCs = []*NPCDef{{ID: "lord", Kind: "target", Pos: Vec3{5.5, 1, 5.5}, Yaw: math.Pi}}
	m.Objects = append(m.Objects,
		&Object{ID: "f", Kind: KindFaucet, Pos: Vec3{5.5, 1, 6.5}, StartRunning: true},
		&Object{ID: "l", Kind: KindLamp, Pos: Vec3{7.5, 1, 5.5}, StartLit: true})
	m.Objectives = []Objective{{Kind: "accident", Target: "lord"}}
	s := New(w, m)
	s.Player.Pos = Vec3{8.5, 1, 5.5} // pick the lamp up and toss it toward -X, into the water
	s.Player.Ammo = map[string]int{}
	run(s, 15*TicksPerSecond)
	if s.Liquid[Cell{5, 1, 5}] != StimWater {
		t.Fatal("no puddle under the target")
	}
	if s.Liquid[Cell{6, 1, 5}] != StimWater {
		s.Liquid[Cell{6, 1, 5}] = StimWater // make sure the lamp lands in water
	}
	s.Frob(Target{Obj: 2, NPC: -1}, false)
	if !s.Objects[2].Held() {
		t.Fatal("lamp not picked up")
	}
	s.ThrowTorch(Vec3{8.5, 2.6, 5.5}, Vec3{-1, 0, 0}, 2, false)
	run(s, 20)
	if o := s.Objects[2]; o.Flight > 0 || o.Pos.Cell() != (Cell{6, 1, 5}) {
		t.Fatalf("lamp not landed in the water: %v", o.Pos)
	}
	if s.NPCs[0].State() != StateDead || s.NPCs[0].Death != "accident" || !s.Done[0] {
		t.Fatalf("target %v (%s), objective %v", s.NPCs[0].State(), s.NPCs[0].Death, s.Done[0])
	}
}

func TestTorchOnRugFire(t *testing.T) {
	w, m := twoRooms()
	m.NPCs = []*NPCDef{guard(Vec3{3.5, 1, 9.5}, 0)}
	m.Objects = append(m.Objects,
		&Object{ID: "t", Kind: KindTorch, Pos: Vec3{3.5, 2, 3.5}, StartLit: true},
		&Object{ID: "rug", Kind: KindFlammable, Pos: Vec3{3.5, 1, 3.5}})
	s := New(w, m)
	s.Player.Pos = Vec3{6.5, 1, 3.5}
	s.Frob(Target{Obj: 1, NPC: -1}, false) // take it: its light comes along
	run(s, 2)
	if o := s.Objects[1]; !o.Held() || o.LightCell != (Cell{6, 2, 3}) {
		t.Fatalf("torch not carried: held %v light %v", o.Held(), o.LightCell)
	}
	s.ThrowTorch(s.Player.Eye(), Vec3{-1, 0, 0}, 3, false) // onto the rug
	s.Player.Pos = Vec3{18.5, 1, 9.5}
	run(s, 2)
	if o := s.Objects[1]; o.Flight == 0 || s.Objects[2].Burning() {
		t.Fatalf("torch landed at once: flight %d", o.Flight)
	}
	run(s, TicksPerSecond)
	if o := s.Objects[1]; o.Flight != 0 || o.Pos.Y != 1 || o.LightCell != o.Pos.Cell() {
		t.Fatalf("torch at %v flight %d light %v; want lying lit on the floor", o.Pos, o.Flight, o.LightCell)
	}
	if !s.Objects[2].Burning() {
		t.Fatal("rug not burning")
	}
	run(s, 3*TicksPerSecond)
	if g := s.NPCs[0]; g.State() != StateInvestigate {
		t.Fatalf("guard %v; want investigating the fire", g.State())
	}
}

func TestBlackjackAndBodyFound(t *testing.T) {
	w, m := twoRooms()
	a := guard(Vec3{5.5, 1, 5.5}, 0)
	b := guard(Vec3{5.5, 1, 9.5}, math.Pi) // facing +Z, away from a
	b.ID = "g2"
	m.NPCs = []*NPCDef{a, b}
	m.Objects = append(m.Objects, &Object{ID: "t", Kind: KindTorch, Pos: Vec3{6.5, 2, 6.5}, StartLit: true})
	s := New(w, m)
	s.Player.Pos, s.Player.Yaw = Vec3{5.5, 1, 7}, 0 // behind a, facing -Z
	if !s.Melee("blackjack") || s.NPCs[0].State() != StateUnconscious {
		t.Fatalf("knockout failed: %v", s.NPCs[0].State())
	}
	s.Player.Pos = Vec3{18.5, 1, 1.5}
	s.NPCs[1].Yaw = 0 // turns around
	run(s, 10)
	if s.NPCs[1].State() != StateSearch || s.Stats.BodiesFound != 1 {
		t.Fatalf("guard 2 %v, bodies %d", s.NPCs[1].State(), s.Stats.BodiesFound)
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
	m.Objects = append(m.Objects, &Object{ID: "t", Kind: KindTorch, Pos: Vec3{6.5, 2, 2.5}, StartLit: true})
	s := New(w, m)
	s.Player.Pos, s.Player.OnGround = Vec3{7.5, 1, 2.5}, true
	run(s, 60)
	if st := s.NPCs[0].State(); st != StateCombat {
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
	if g := s.NPCs[0]; g.State() == StatePatrol {
		t.Fatalf("guard didn't react to breaking glass (suspicion %.1f)", g.Suspicion())
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
	if p.X < 5.75+p.Width/2 { // lower than stepHeight: stepped over, as in Minecraft
		t.Fatalf("player at x=%.2f, want stepped over the box", p.X)
	}
	if tg := s.TargetAt(Vec3{4.5, 1.2, 2.5}, Vec3{1, 0, 0}); tg.Obj >= 0 {
		t.Fatalf("targeted object %d", tg.Obj)
	}
}

func TestPiercedBarrelLeaks(t *testing.T) {
	w, m := twoRooms()
	m.Objects = append(m.Objects,
		&Object{ID: "barrel", Kind: KindDecor, Name: "Barrel", Pos: Vec3{5.5, 1, 2.5}},
		&Object{ID: "torch", Kind: KindTorch, StartLit: true, On: "floor", Pos: Vec3{6.5, 1, 2.5}})
	s := New(w, m)
	torch := s.Objects[len(s.Objects)-1]
	s.ArrowHit(AmmoWater, Vec3{5.5, 1.5, 2}, -1, false)
	spread(s)
	if n := len(s.LiquidCells(StimWater)); n != 5 {
		t.Fatalf("water arrow: %d puddle cells, want its own 5", n)
	}
	s.B.Set(stimres.Object[Cell](torch.R), "lit", true)
	clear(s.Liquid)
	s.Player.Pos, s.Player.Yaw, s.Player.Pitch = Vec3{4.5, 1, 2.5}, math.Pi/2, -0.6 // looking at it
	s.Melee("blackjack")
	if len(s.LiquidCells(StimWater)) != 0 {
		t.Fatalf("blackjack leaked %d cells", len(s.LiquidCells(StimWater)))
	}
	s.Melee("blade")
	spread(s)
	if len(s.LiquidCells(StimWater)) < 2 {
		t.Fatalf("stabbed: puddle of %d cells, want a few", len(s.LiquidCells(StimWater)))
	}
	if torch.Lit() {
		t.Fatal("the leak didn't douse the torch beside the barrel")
	}
	clear(s.Liquid)
	s.ArrowHit(AmmoBroadhead, Vec3{5.5, 1.5, 2}, -1, false)
	spread(s)
	if len(s.LiquidCells(StimWater)) < 2 {
		t.Fatalf("shot: puddle of %d cells, want a few", len(s.LiquidCells(StimWater)))
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
	spread(s)
	if len(s.LiquidCells(StimOil)) < 3 || len(s.LiquidCells(StimWater)) != 0 {
		t.Fatalf("oil %d cells, water %d", len(s.LiquidCells(StimOil)), len(s.LiquidCells(StimWater)))
	}
	s.NPCs[0].Body.Teleport(5.5, 1, 2.5) // standing in it
	s.Player.Pos = Vec3{5.5, 1, 3.5}     // and the player
	s.B.Set(s.playerR(), "health", 1000)
	s.Stimulate(at(StimFire, Vec3{5.5, 1.1, 2.5}, 0.3, 0)) // a spark
	run(s, 2*TicksPerSecond)
	if s.Player.Health() == 1000 {
		t.Fatal("player unhurt standing in burning oil")
	}
	if s.NPCs[0].State() != StateDead || s.NPCs[0].Death != "accident" {
		t.Fatalf("guard %v (%s), want burned", s.NPCs[0].State(), s.NPCs[0].Death)
	}
	burning := burningOil(s)
	if burning < 3 {
		t.Fatalf("fire spread to %d of %d oil cells", burning, len(s.LiquidCells(StimOil)))
	}
	s.Stimulate(at(StimWater, Vec3{5.5, 1.1, 2.5}, 3, 0))
	for _, c := range s.LiquidCells(StimOil) {
		if s.OilBurning(c) {
			t.Fatalf("%v still burning after water", c)
		}
	}
}

func TestBombBurstsOilBarrelIntoFire(t *testing.T) {
	w, m := twoRooms()
	m.Objects = append(m.Objects, &Object{ID: "oil", Kind: KindDecor, Name: "Oil barrel", Pos: Vec3{8.5, 1, 2.5}})
	s := New(w, m)
	s.Explode(Vec3{5.5, 1.2, 2.5})
	burning := burningOil(s)
	if !s.Objects[len(s.Objects)-1].Gone || burning == 0 {
		t.Fatalf("barrel gone %v, %d of %d oil cells burning", s.Objects[len(s.Objects)-1].Gone, burning, len(s.LiquidCells(StimOil)))
	}
}

func TestWaterArrowPuddle(t *testing.T) {
	for _, hit := range []Vec3{{4.5, 1.1, 4.5}, {9.6, 2.8, 2.5}} { // floor; wall at x=10, up high
		w, m := twoRooms()
		s := New(w, m)
		s.ArrowHit(AmmoWater, hit, -1, false)
		if len(s.LiquidCells(StimWater)) != 1 {
			t.Fatalf("hit at %v: %d puddle cells at once, want 1", hit, len(s.LiquidCells(StimWater)))
		}
		run(s, 2*spillEvery)
		if len(s.LiquidCells(StimWater)) != 3 {
			t.Fatalf("hit at %v: %d puddle cells after 2 steps, want 3", hit, len(s.LiquidCells(StimWater)))
		}
		spread(s)
		if len(s.LiquidCells(StimWater)) != 5 {
			t.Fatalf("hit at %v: %d puddle cells, want 5", hit, len(s.LiquidCells(StimWater)))
		}
		for _, c := range s.LiquidCells(StimWater) {
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

// A lit torch lying by a hedge or the graveyard's dead tree sets it alight.
func TestTorchLightsHedge(t *testing.T) {
	for _, fuel := range []string{"minecraft:oak_leaves[distance=1,persistent=true]", "voxelstealth:dark_trunk_log"} {
		testTorchLights(t, fuel)
	}
}

func testTorchLights(t *testing.T, fuel string) {
	caught := 0
	for seed := range 20 {
		w, m := twoRooms()
		for z := 2; z <= 6; z++ {
			w.b[Cell{4, 1, z}] = fuel
		}
		m.Objects = append(m.Objects, &Object{ID: "t", Kind: KindTorch, Pos: Vec3{5.5, 1, 4.5}, StartLit: true})
		s := New(w, m)
		s.B.Set(s.objR(len(s.Objects)-1), "fallen", true)
		s.rng = rand.New(rand.NewSource(int64(seed)))
		run(s, 20*TicksPerSecond)
		if len(s.FireCells()) > 0 || w.Block(4, 1, 4) != fuel {
			caught++
		}
	}
	if caught < 20 {
		t.Fatalf("%s caught %d of 20 times", fuel, caught)
	}
}

// A torch lying on a rug it set alight keeps its light when the rug burns out.
func TestLyingTorchOutlastsRug(t *testing.T) {
	w, m := twoRooms()
	m.Objects = append(m.Objects,
		&Object{ID: "t", Kind: KindTorch, Pos: Vec3{3.5, 1, 3.5}, StartLit: true},
		&Object{ID: "rug", Kind: KindFlammable, Pos: Vec3{3.5, 1, 3.5}})
	s := New(w, m)
	s.B.Set(s.objR(len(s.Objects)-2), "fallen", true)
	s.B.SetFor(s.objR(len(s.Objects)-1), "burning", true, 1) // about to burn out
	s.setLight(len(s.Objects)-1, true)
	run(s, 2)
	if !s.Objects[len(s.Objects)-1].Burnt() {
		t.Fatal("rug didn't burn out")
	}
	if b := w.Block(3, 1, 3); b != BlockLight {
		t.Fatalf("torch's cell is %s, want its light", b)
	}
}

// Broadheads kill with a head shot; in the body they take a quarter of
// full health, so four do it.
func TestBroadheadDamage(t *testing.T) {
	w, m := twoRooms()
	m.NPCs = []*NPCDef{guard(Vec3{3.5, 1, 9.5}, 0), guard(Vec3{5.5, 1, 9.5}, 0)}
	s := New(w, m)
	s.ArrowHit(AmmoBroadhead, Vec3{3.5, 2.7, 9.5}, 0, true)
	if s.NPCs[0].State() != StateDead {
		t.Fatal("head shot didn't kill")
	}
	for i := 1; i <= 4; i++ {
		s.ArrowHit(AmmoBroadhead, Vec3{5.5, 1.5, 9.5}, 1, false)
		if dead := s.NPCs[1].State() == StateDead; dead != (i == 4) {
			t.Fatalf("body shot %d: dead %v, health %d", i, dead, s.NPCs[1].Health())
		}
	}
}

// Fire spreads over a plank floor, eats it and dies out; on bare stone it
// dies out by itself; water puts it out.
func TestFireSpreads(t *testing.T) {
	w, m := twoRooms()
	for x := 0; x <= 9; x++ {
		for z := 0; z <= 10; z++ {
			w.b[Cell{x, 0, z}] = "minecraft:oak_planks"
		}
	}
	s := New(w, m)
	s.Player.Pos = Vec3{18.5, 1, 9.5}
	s.ignite(Cell{4, 1, 4}, 0)
	s.ignite(Cell{15, 1, 4}, 0) // on stone
	most := 0
	for range 4 * 60 * TicksPerSecond { // the last plank under a fire can hold out a while
		s.Tick()
		most = max(most, len(s.FireCells()))
	}
	planks := 0
	for _, b := range w.b {
		if b == "minecraft:oak_planks" {
			planks++
		}
	}
	t.Logf("at most %d cells burning, %d of 110 planks left", most, planks)
	if most < 5 || planks > 100 || len(s.FireCells()) > 0 {
		t.Fatalf("at most %d cells burning, %d of 110 planks left, %d still burning", most, planks, len(s.FireCells()))
	}
	if w.Block(15, 0, 4) != "minecraft:stone" || w.Block(10, 1, 4) != "minecraft:stone_bricks" {
		t.Fatal("fire ate stone")
	}
	if b := w.Block(4, 1, 4); b != BlockAir {
		t.Fatalf("burnt-out cell is %s", b)
	}

	s.ignite(Cell{15, 1, 4}, 0)
	s.Stimulate(at(StimWater, Vec3{15.5, 1.2, 4.5}, 0.9, 0))
	if len(s.FireCells()) != 0 {
		t.Fatal("water didn't put the fire out")
	}
}

// A floor torch placed in mid-air stands on the floor below it.
func TestFloatingTorchSettles(t *testing.T) {
	w, m := twoRooms()
	m.Objects = append(m.Objects, &Object{ID: "t", Kind: KindTorch, Pos: Vec3{4.5, 2, 4.5}, On: "floor"})
	if s := New(w, m); s.Objects[len(s.Objects)-1].Pos.Y != 1 {
		t.Fatalf("torch at y %v, want on the floor at 1", s.Objects[len(s.Objects)-1].Pos.Y)
	}
}

// A lit torch landing on a wooden floor can set it alight; one on stone
// can't.
func TestTorchStartsBlockFire(t *testing.T) {
	caught := 0
	for seed := range 200 {
		w, m := twoRooms()
		for x := 0; x <= 9; x++ {
			for z := 0; z <= 10; z++ {
				w.b[Cell{x, 0, z}] = "minecraft:oak_planks"
			}
		}
		m.Objects = append(m.Objects, &Object{ID: "t", Kind: KindTorch, Pos: Vec3{4.5, 1, 4.5}, StartLit: true})
		s := New(w, m)
		s.B.Set(s.objR(len(s.Objects)-1), "held", true)
		s.rng.Seed(int64(seed))
		s.Player.Pos = Vec3{4.5, 1, 4.5}
		s.ThrowTorch(Vec3{4.5, 2.6, 4.5}, Vec3{0, 0, 1}, 0, false)
		run(s, TicksPerSecond)
		if len(s.FireCells()) > 0 {
			caught++
		}
	}
	if caught == 0 || caught == 200 {
		t.Fatalf("%d of 200 torches started a fire; want some", caught)
	}
	t.Logf("%d of 200 torches started a fire", caught)
	w, m := twoRooms()
	m.Objects = append(m.Objects, &Object{ID: "t", Kind: KindTorch, Pos: Vec3{4.5, 1, 4.5}, StartLit: true})
	s := New(w, m)
	s.B.Set(s.objR(len(s.Objects)-1), "held", true)
	s.ThrowTorch(Vec3{4.5, 2.6, 4.5}, Vec3{0, 0, 1}, 0, false)
	run(s, TicksPerSecond)
	if len(s.FireCells()) > 0 {
		t.Fatal("a torch set stone alight")
	}
}

// Fire primes the TNT it burns; the blast primes TNT nearby, which goes
// off in turn.
func TestBurningTNTExplodes(t *testing.T) {
	w, m := twoRooms()
	w.b[Cell{4, 1, 4}] = BlockTNT
	w.b[Cell{7, 1, 4}] = BlockTNT // out of the first blast's reach, not its stimulus
	s := New(w, m)
	s.Player.Pos = Vec3{18.5, 1, 9.5}
	s.ignite(Cell{5, 1, 4}, 0)
	run(s, 30*TicksPerSecond)
	if w.Block(4, 1, 4) == BlockTNT || w.Block(7, 1, 4) == BlockTNT || len(s.Bombs) > 0 {
		t.Fatalf("TNT left: %s, %s; %d primed", w.Block(4, 1, 4), w.Block(7, 1, 4), len(s.Bombs))
	}
	if w.Block(7, 0, 4) != BlockAir {
		t.Fatal("the second TNT didn't go off")
	}
}

// NPCs' footsteps are sounds, but only the player's make a guard wonder.
func TestNPCFootstepsDontSpook(t *testing.T) {
	w, m := twoRooms()
	walker := &NPCDef{ID: "w", Kind: "servant", Pos: Vec3{2.5, 1, 2.5},
		Patrol: []Waypoint{{Pos: Vec3{2.5, 1, 2.5}}, {Pos: Vec3{8.5, 1, 2.5}}}}
	m.NPCs = []*NPCDef{walker, guard(Vec3{5.5, 1, 4.5}, math.Pi)}
	s := New(w, m)
	s.Player.Pos = Vec3{18.5, 1, 9.5}
	steps := 0
	for range 10 * TicksPerSecond {
		s.Tick()
		for _, e := range s.Events {
			if e.Kind == EvSound && e.Sound == "footstep" && e.NPC == 0 {
				steps++
			}
		}
		s.Events = s.Events[:0]
	}
	if steps < 5 || s.NPCs[1].Suspicion() > 0 {
		t.Fatalf("%d NPC footsteps, guard suspicion %.1f", steps, s.NPCs[1].Suspicion())
	}
	s.emit(Vec3{4.5, 1, 4.5}, 40, "footstep", -1)
	s.Tick()
	if s.NPCs[1].Suspicion() == 0 {
		t.Fatal("the guard didn't hear the player's step")
	}
}

func TestOwnBlocksBurnAndSound(t *testing.T) {
	if i, _ := flammability("voxelstealth:red_leaves"); i == 0 {
		t.Error("red leaves don't burn")
	}
	if i, _ := flammability("voxelstealth:dark_trunk_log"); i == 0 {
		t.Error("the dark trunk doesn't burn")
	}
	for b, want := range map[string]string{"voxelstealth:corrupted_dirt": "grass", "voxelstealth:corrupted_grass": "grass", "stone": "stone", "minecraft:oak_planks": "wood"} {
		if got := Surface(b); got != want {
			t.Errorf("Surface(%q) = %q, want %q", b, got, want)
		}
	}
}

func TestHardThrowBashes(t *testing.T) {
	for _, hard := range []bool{false, true} {
		w, m := twoRooms()
		m.NPCs = []*NPCDef{guard(Vec3{3.5, 1, 3.5}, 0)}
		m.Objects = append(m.Objects, &Object{ID: "t", Kind: KindTorch, Pos: Vec3{6.5, 2, 4.5}})
		s := New(w, m)
		s.Player.Pos = Vec3{6.5, 1, 3.5}
		s.Frob(Target{Obj: len(s.Objects) - 1, NPC: -1}, false)
		s.ThrowTorch(s.Player.Eye(), Vec3{-1, 0, 0}, 8, hard) // at the guard's head
		run(s, TicksPerSecond)
		if out := s.NPCs[0].State() == StateUnconscious; out != hard {
			t.Errorf("hard %v: guard knocked out %v", hard, out)
		}
	}
}

func TestFlashBombBlinds(t *testing.T) {
	w, m := twoRooms()
	m.NPCs = []*NPCDef{guard(Vec3{5.5, 1, 5.5}, 0), guard(Vec3{5.5, 1, 9.5}, math.Pi)} // facing -Z; facing +Z, away
	m.NPCs[1].ID = "g2"
	s := New(w, m)
	s.Player.Pos, s.Player.Yaw = Vec3{8.5, 1, 6.5}, math.Pi/2
	s.setState(0, StateCombat)
	s.Player.Ammo[AmmoExplosive] = 1
	if !s.Throw(Vec3{9, 2.6, 2.5}, Vec3{-1, 0, 0}, 0.5, false) {
		t.Fatal("no bomb thrown")
	}
	run(s, 3*TicksPerSecond+1) // the fuse; it lands by the wall, in front of guard 1
	if s.NPCs[0].Blind() == 0 || s.NPCs[1].Blind() != 0 {
		t.Fatalf("blind: facing %d, facing away %d", s.NPCs[0].Blind(), s.NPCs[1].Blind())
	}
	if !w.Solid(10, 1, 2) || !s.NPCs[0].alive() {
		t.Fatal("a flash bomb broke the wall or hurt the guard")
	}
	s.NPCs[1].Hidden = true                       // heard the bang and would come running: out of the way
	s.NPCs[0].Body.X, s.NPCs[0].Body.Z = 7.5, 6.5 // right beside the player, still in combat
	hp := s.Player.Health()
	run(s, 2*TicksPerSecond)
	if s.Player.Health() != hp || s.NPCs[0].Attacking || s.NPCs[0].Blind() == 0 {
		t.Fatalf("blinded guard fought: health %d -> %d, blind %d", hp, s.Player.Health(), s.NPCs[0].Blind())
	}
	s.Player.Yaw = yawTo(s.Player.Pos, s.NPCs[0].pos())
	if !s.Melee("blackjack") || s.NPCs[0].State() != StateUnconscious {
		t.Fatalf("blinded guard in combat not knocked out: %v", s.NPCs[0].State())
	}
}

func TestPlacedTNTBreaksWall(t *testing.T) {
	w, m := twoRooms()
	s := New(w, m)
	s.Player.Ammo[AmmoTNT] = 1
	if !s.PlaceTNT(Vec3{8.5, 2.6, 2.5}, Vec3{1, 0, 0}) || s.Bombs[0].Pos.X < 9 {
		t.Fatalf("TNT not placed by the wall: %v", s.Bombs)
	}
	run(s, 4*TicksPerSecond+1)
	if w.Solid(10, 1, 2) {
		t.Fatal("TNT left the wall standing")
	}
}

func TestTNTRestsInDoorway(t *testing.T) {
	w, m := twoRooms()
	s := New(w, m)
	s.setDoor(0, true)
	s.Player.Ammo[AmmoTNT] = 1
	s.PlaceTNT(Vec3{8.5, 2.6, 5.5}, Vec3{0.8, -0.6, 0})
	if c := s.Bombs[0].Pos.Cell(); c != (Cell{10, 1, 5}) || s.Bombs[0].Pos.Y != 1 {
		t.Fatalf("TNT not on the doorway floor: %v", s.Bombs[0].Pos)
	}
}

func TestFlashBombFlies(t *testing.T) {
	w, m := twoRooms()
	s := New(w, m)
	s.Player.Ammo[AmmoExplosive] = 1
	s.Throw(Vec3{3.5, 2.6, 2.5}, Vec3{1, 0, 0}, 5, false)
	run(s, 3)
	if b := s.Bombs[0]; !b.Flying || b.Pos.X < 4.5 || b.Pos.Y < 1.5 {
		t.Fatalf("bomb not in the air after 3 ticks: %+v", b)
	}
	run(s, 30)
	if b := s.Bombs[0]; b.Flying || b.Pos.Y != 1 || b.Pos.X < 6 {
		t.Fatalf("bomb not landed ahead: %+v", b)
	}
}

func TestStreetLampDoused(t *testing.T) {
	w, m := twoRooms()
	m.Objects = append(m.Objects, &Object{ID: "l", Kind: KindLamp, Name: StreetLamp, Pos: Vec3{7.5, 1, 5.5}, StartLit: true})
	s := New(w, m)
	s.Player.Pos = Vec3{8.5, 1, 5.5}
	s.Frob(Target{Obj: 1, NPC: -1}, false)
	if o := s.Objects[1]; o.Fallen() || !o.Lit() {
		t.Fatal("street lamp knocked over")
	}
	s.ArrowHit(AmmoWater, Vec3{7.5, 1 + StreetLampHead, 5.5}, -1, false)
	if o := s.Objects[1]; o.Lit() || o.HasLight {
		t.Fatal("water left the street lamp lit")
	}
}

func TestThrownLanternSpillsBurningOil(t *testing.T) {
	w, m := twoRooms()
	m.Objects = append(m.Objects, &Object{ID: "l", Kind: KindTorch, Name: "Lantern", Pos: Vec3{6.5, 1, 3.5}, StartLit: true, On: "floor"})
	s := New(w, m)
	s.Player.Pos = Vec3{6.5, 1, 3.5}
	s.Frob(Target{Obj: 1, NPC: -1}, false)
	s.ThrowTorch(Vec3{6.5, 2.6, 3.5}, Vec3{-1, 0, 0}, 3, false)
	run(s, 30)
	burning := burningOil(s)
	if len(s.LiquidCells(StimOil)) < 2 || burning == 0 {
		t.Fatalf("lantern spilled %d oil cells, %d burning", len(s.LiquidCells(StimOil)), burning)
	}
}

func TestDoorSwings(t *testing.T) {
	w, m := twoRooms()
	s := New(w, m)
	if !w.Solid(10, 1, 5) || !w.Solid(10, 2, 5) {
		t.Fatalf("shut door not solid: %q", w.Block(10, 1, 5))
	}
	s.setDoor(0, true)
	if w.Solid(10, 1, 5) || w.Solid(10, 2, 5) {
		t.Fatalf("open door still solid: %q", w.Block(10, 1, 5))
	}
	if b := w.Block(10, 2, 5); !strings.Contains(b, "half=upper") {
		t.Fatalf("upper half = %q", b)
	}
}

// burningOil counts the oil cells alight.
func burningOil(s *Sim) int {
	n := 0
	for _, c := range s.LiquidCells(StimOil) {
		if s.OilBurning(c) {
			n++
		}
	}
	return n
}
