package sim

import "testing"

// mealRound is a servant whose round stops by a cake on a table-high
// floor beside its waypoint, and a player with a poison vial.
func mealRound() *Sim {
	w, m := twoRooms()
	n := guard(Vec3{4.5, 1, 4.5}, 0)
	n.Kind, n.Weapon = "civilian", ""
	n.Patrol = []Waypoint{{Pos: Vec3{4.5, 1, 4.5}, Wait: 20}, {Pos: Vec3{7.5, 1, 8.5}, Wait: 20}}
	m.NPCs = []*NPCDef{n}
	m.Objects = append(m.Objects, &Object{ID: "cake", Kind: KindFood, Name: "Banana cake", Pos: Vec3{5.5, 1, 4.5}})
	s := New(w, m)
	s.Player.Pos = Vec3{18.5, 1, 9.5}
	s.Player.Ammo[AmmoPoison] = 2
	return s
}

func cakeIndex(s *Sim) int {
	for i, o := range s.Objects {
		if o.ID == "cake" {
			return i
		}
	}
	return -1
}

// A vial poisons food once, for good; the vial is used.
func TestPoisonFoodOnce(t *testing.T) {
	s := mealRound()
	c := cakeIndex(s)
	if !s.PoisonTarget(Target{Obj: c, NPC: -1}) || !s.Objects[c].Poisoned() || s.Player.Ammo[AmmoPoison] != 1 {
		t.Fatalf("poisoned %v, vials %d", s.Objects[c].Poisoned(), s.Player.Ammo[AmmoPoison])
	}
	if s.PoisonTarget(Target{Obj: c, NPC: -1}) || s.Player.Ammo[AmmoPoison] != 1 {
		t.Fatalf("poisoned twice: vials %d", s.Player.Ammo[AmmoPoison])
	}
	seen := false
	for _, e := range s.Events {
		seen = seen || e.Kind == EvPoisoned
	}
	if !seen {
		t.Fatal("no EvPoisoned event")
	}
	run(s, 2000)
	if !s.Objects[c].Poisoned() {
		t.Fatal("the poison wore off")
	}
}

// An NPC stopping by food eats it, standing there; eating it poisoned it
// goes on as if nothing happened for ten seconds, then thrashes and dies.
func TestPoisonedMealKills(t *testing.T) {
	s := mealRound()
	c := cakeIndex(s)
	s.PoisonTarget(Target{Obj: c, NPC: -1})
	k, ate := 0, -1
	for i := range 200 {
		s.Tick()
		if s.Eating(k) {
			ate = i
			break
		}
	}
	if ate < 0 {
		t.Fatal("never ate the cake")
	}
	at := s.NPCs[k].pos()
	run(s, EatTicks-2)
	if !s.Eating(k) || s.NPCs[k].pos().DistXZ(at) > 0.05 {
		t.Fatalf("eating %v, moved %.2f", s.Eating(k), s.NPCs[k].pos().DistXZ(at))
	}
	run(s, 9*TicksPerSecond-EatTicks)
	if !s.NPCs[k].alive() || s.Convulsing(k) {
		t.Fatalf("after 9 s alive %v convulsing %v", s.NPCs[k].alive(), s.Convulsing(k))
	}
	thrash := false
	for range 4 * TicksPerSecond {
		s.Tick()
		thrash = thrash || s.Convulsing(k)
	}
	if !thrash || s.NPCs[k].State() != StateDead || s.NPCs[k].Death != "player" {
		t.Fatalf("thrashed %v, state %v by %q", thrash, s.NPCs[k].State(), s.NPCs[k].Death)
	}
}

// A clean meal does nothing to the NPC.
func TestCleanMealHarmless(t *testing.T) {
	s := mealRound()
	run(s, 30*TicksPerSecond)
	if !s.NPCs[0].alive() {
		t.Fatal("died of a clean cake")
	}
}

// A bottle is taken; thrown it shatters where it lands, loud; thrown hard
// at an NPC it knocks it out; swung, it knocks out once and is gone.
func TestBottles(t *testing.T) {
	w, m := twoRooms()
	m.NPCs = []*NPCDef{guard(Vec3{5.5, 1, 2.5}, 0)}
	m.Objects = append(m.Objects, &Object{ID: "b", Kind: KindDrink, Name: Bottle, Pos: Vec3{2.5, 1, 8.5}})
	s := New(w, m)
	s.Player.Pos = Vec3{2.5, 1, 8.5}
	b := len(s.Objects) - 1
	s.Frob(Target{Obj: b, NPC: -1}, false)
	if s.Player.Ammo[AmmoBottle] != 1 || !s.Objects[b].Gone {
		t.Fatalf("bottles %d, taken %v", s.Player.Ammo[AmmoBottle], s.Objects[b].Gone)
	}
	s.Events = nil
	eye := s.Player.Eye()
	if !s.ThrowItem(AmmoBottle, eye, Vec3{0, 0, 1}, 2, false) || s.ThrowItem(AmmoBottle, eye, Vec3{0, 0, 1}, 2, false) {
		t.Fatal("threw a bottle not had")
	}
	thrown := len(s.Objects) - 1
	run(s, 60)
	shattered := false
	for _, e := range s.Events {
		shattered = shattered || e.Kind == EvSound && e.Sound == "shatter"
	}
	if !s.Objects[thrown].Gone || !shattered {
		t.Fatalf("landed bottle gone %v, shattered %v", s.Objects[thrown].Gone, shattered)
	}
	// Hard at the guard's back, from close by: out cold.
	s.Player.Ammo[AmmoBottle] = 2
	s.Player.Pos = Vec3{5.5, 1, 5.5}
	from := s.Player.Eye()
	dir := s.NPCs[0].pos().Add(Vec3{0, 1.2, 0}).Sub(from)
	dir = dir.Scale(1 / dir.Len())
	s.ThrowItem(AmmoBottle, from, dir, 6, true)
	run(s, 20)
	if s.NPCs[0].State() != StateUnconscious {
		t.Fatalf("hit hard by a bottle: %v", s.NPCs[0].State())
	}
	// Swung: knocks out a second guard, and is gone with the blow.
	w2, m2 := twoRooms()
	m2.NPCs = []*NPCDef{guard(Vec3{5.5, 1, 4.5}, 0)}
	s = New(w2, m2)
	s.Player.Ammo[AmmoBottle] = 1
	s.Player.Pos, s.Player.Yaw = Vec3{5.5, 1, 5.8}, 0
	if !s.Melee("bottle") || s.NPCs[0].State() != StateUnconscious || s.Player.Ammo[AmmoBottle] != 0 {
		t.Fatalf("swung: %v, bottles left %d", s.NPCs[0].State(), s.Player.Ammo[AmmoBottle])
	}
}
