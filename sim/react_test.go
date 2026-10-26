package sim

import (
	"math"
	"testing"
)

func TestBurningGuardRunsForWater(t *testing.T) {
	w, m := twoRooms()
	m.NPCs = []*NPCDef{guard(Vec3{2.5, 1, 5.5}, 0)}
	s := New(w, m)
	s.pourOut(StimWater, Cell{8, 1, 5}, 1)
	s.hit(s.npcR(0), at(StimFire, Vec3{2.5, 1.5, 5.5}, 0.5, 1))
	if !s.Burning(0) {
		t.Fatal("flames didn't set the guard alight")
	}
	for i := 0; i < 6*TicksPerSecond && s.Burning(0); i++ {
		s.Tick()
	}
	if n := s.NPCs[0]; s.Burning(0) || !n.alive() || n.pos().DistXZ(Vec3{8.5, 1, 5.5}) > 1.5 {
		t.Fatalf("guard burning %v, %v at %v", s.Burning(0), n.State(), n.pos())
	}
}

func TestBurningWithoutWaterKills(t *testing.T) {
	w, m := twoRooms()
	m.NPCs = []*NPCDef{guard(Vec3{2.5, 1, 5.5}, 0)}
	s := New(w, m)
	s.hit(s.npcR(0), at(StimFire, Vec3{2.5, 1.5, 5.5}, 0.5, 1))
	run(s, 8*TicksPerSecond)
	if n := s.NPCs[0]; n.State() != StateDead || n.Death != "accident" {
		t.Fatalf("guard %v (%s), want burned to death", n.State(), n.Death)
	}
}

func TestGuardSlipsOnOil(t *testing.T) {
	w, m := twoRooms()
	g := guard(Vec3{2.5, 1, 5.5}, 0)
	g.Patrol = []Waypoint{{Pos: Vec3{8.5, 1, 5.5}}, {Pos: Vec3{2.5, 1, 5.5}}}
	m.NPCs = []*NPCDef{g}
	s := New(w, m)
	s.pourOut(StimOil, Cell{5, 1, 5}, 1)
	for i := 0; i < 8*TicksPerSecond; i++ {
		s.Tick()
		if s.Airborne(0) {
			return
		}
	}
	t.Fatal("guard walked over oil without slipping")
}

func TestDoorBurnsAway(t *testing.T) {
	w, m := twoRooms()
	s := New(w, m)
	s.hit(s.objR(0), at(StimFire, Vec3{10.5, 1.5, 5.5}, 0.5, 1))
	if !s.Objects[0].Burning() {
		t.Fatal("door didn't catch")
	}
	run(s, 31*TicksPerSecond)
	if !s.Objects[0].Gone || w.Block(10, 1, 5) != BlockAir {
		t.Fatal("burnt door still stands")
	}
}

func TestWoodenStairsCreak(t *testing.T) {
	w, m := twoRooms()
	w.SetBlock(4, 1, 4, "minecraft:spruce_stairs[facing=east,half=bottom]")
	s := New(w, m)
	s.Events = nil
	s.step(Vec3{4.5, 1.5, 4.5}, 1, -1)
	s.step(Vec3{6.5, 1, 6.5}, 1, -1)
	var got []string
	for _, e := range s.Events {
		if e.Kind == EvSound {
			got = append(got, e.Sound)
		}
	}
	if len(got) != 2 || got[0] != "creak" || got[1] != "footstep" {
		t.Fatalf("sounds %v, want creak then footstep", got)
	}
}

func TestStruckGuardTurnsOnSource(t *testing.T) {
	w, m := twoRooms()
	m.NPCs = []*NPCDef{guard(Vec3{5.5, 1, 5.5}, 0)}
	s := New(w, m)
	s.Player.Pos = Vec3{5.5, 1, 8.5} // behind it
	pr := s.playerR()
	s.hit(s.npcR(0), Stimulus{Stim: StimSlam, Intensity: 3, Origin: s.NPCs[0].pos(), Source: &pr})
	if n := s.NPCs[0]; n.State() != StateSearch || n.LKP.Dist(s.Player.Pos) > 0.1 {
		t.Fatalf("guard %v looking at %v", n.State(), n.LKP)
	}
}

func TestGuardWakesComrade(t *testing.T) {
	w, m := twoRooms()
	a := guard(Vec3{5.5, 1, 5.5}, 0)
	b := guard(Vec3{5.5, 1, 9.5}, 0)
	b.ID = "g2"
	m.NPCs = []*NPCDef{a, b}
	m.Objects = append(m.Objects, &Object{ID: "t", Kind: KindTorch, Pos: Vec3{6.5, 2, 6.5}, StartLit: true})
	s := New(w, m)
	s.Player.Pos, s.Player.Yaw = Vec3{5.5, 1, 7}, 0
	if !s.Melee("blackjack") || s.NPCs[0].State() != StateUnconscious {
		t.Fatalf("knockout failed: %v", s.NPCs[0].State())
	}
	s.Player.Pos = Vec3{18.5, 1, 1.5}
	run(s, 10*TicksPerSecond)
	if !s.NPCs[0].alive() {
		t.Fatalf("guard 1 still %v, guard 2 %v chore %q", s.NPCs[0].State(), s.NPCs[1].State(), s.NPCs[1].Chore)
	}
}

func TestGuardRelightsTorch(t *testing.T) {
	w, m := twoRooms()
	m.NPCs = []*NPCDef{guard(Vec3{5.5, 1, 8.5}, 0)}
	m.Objects = append(m.Objects, &Object{ID: "t", Kind: KindTorch, Pos: Vec3{5.5, 2, 2.5}, StartLit: true})
	s := New(w, m)
	s.hit(s.objR(1), at(StimWater, Vec3{5.5, 2.5, 2.5}, 0.5, 0))
	if s.Objects[1].Lit() {
		t.Fatal("water didn't put the torch out")
	}
	run(s, 12*TicksPerSecond)
	if !s.Objects[1].Lit() {
		t.Fatalf("torch still out; guard %v chore %q at %v", s.NPCs[0].State(), s.NPCs[0].Chore, s.NPCs[0].pos())
	}
}

func TestServantMopsPuddle(t *testing.T) {
	w, m := twoRooms()
	c := guard(Vec3{5.5, 1, 6.5}, 0)
	c.Kind = "civilian"
	m.NPCs = []*NPCDef{c}
	s := New(w, m)
	s.pourOut(StimWater, Cell{5, 1, 4}, 1)
	run(s, 10*TicksPerSecond)
	if l := s.Liquid[Cell{5, 1, 4}]; l != "" {
		t.Fatalf("puddle still there; servant %v chore %q", s.NPCs[0].State(), s.NPCs[0].Chore)
	}
}

func TestKickedBarrelToppleAndSpills(t *testing.T) {
	w, m := twoRooms()
	m.Objects = append(m.Objects, &Object{ID: "b", Kind: KindDecor, Name: "Barrel", Pos: Vec3{5.5, 1, 4.5}})
	s := New(w, m)
	s.Player.Pos, s.Player.Yaw = Vec3{5.5, 1, 5.7}, 0
	s.Kick()
	spread(s)
	if down, way := s.Objects[1].Toppled(); !down || math.Abs(way) > 0.01 {
		t.Fatalf("barrel toppled %v way %v", down, way)
	}
	if len(s.LiquidCells(StimWater)) < 4 {
		t.Fatalf("barrel spilled %d cells", len(s.LiquidCells(StimWater)))
	}
}

func TestFallsHurt(t *testing.T) {
	for _, c := range []struct {
		h    float64
		want NPCState
	}{{2, StatePatrol}, {5, StateUnconscious}, {9, StateDead}} {
		w, m := twoRooms()
		for y := 1; y < 12; y++ { // open the ceiling above the drop
			w.SetBlock(5, y, 5, BlockAir)
		}
		m.NPCs = []*NPCDef{guard(Vec3{5.5, 1 + c.h, 5.5}, 0)}
		s := New(w, m)
		s.Player.Pos = Vec3{18.5, 1, 9.5}
		run(s, 2*TicksPerSecond)
		if st := s.NPCs[0].State(); (st == StateDead) != (c.want == StateDead) || (st == StateUnconscious) != (c.want == StateUnconscious) {
			t.Errorf("guard dropped %v blocks is %v, want %v", c.h, st, c.want)
		}
	}
	w, m := twoRooms()
	s := New(w, m)
	s.Player.Landed = 6
	s.Tick()
	if h := s.Player.Health(); h != 17 {
		t.Fatalf("player at %d after a 6-block fall, want 17", h)
	}
}

// A sound from the next room sends a guard to the doorway it came through
// and on into that room, not straight to its source.
func TestGuardFollowsSoundThroughDoorway(t *testing.T) {
	w, m := twoRooms()
	m.NPCs = []*NPCDef{guard(Vec3{15.5, 1, 8.5}, 0)}
	s := New(w, m)
	s.Player.Pos = Vec3{18.5, 1, 1.5}
	s.setDoor(0, true)
	src := Vec3{3.5, 1, 2.5}
	s.emit(src, 60, "shatter", -1)
	n := s.NPCs[0]
	if n.LKP.Dist(src) < 3 || n.LKP.X < 8 || n.LKP.X > 10.5 {
		t.Fatalf("guard goes to %v for a sound at %v; want just inside room A by the door", n.LKP, src)
	}
}
