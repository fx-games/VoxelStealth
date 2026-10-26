package sim

// Food and drink, poison and bottles. NPCs on their rounds eat or drink
// what stands by a waypoint they stop at (an eat stimulus to it); a
// poison vial poured into food or drink poisons it for the rest of the
// run, and whoever eats it is poisoned in turn: ten seconds as if nothing
// happened, then it thrashes and dies (stimuli.toml). Bottles are taken,
// then thrown (they shatter, loud: a distraction; thrown hard they knock
// out whoever they strike) or swung once as a club.

// Bottle is the drink the player can take.
const Bottle = "Bottle"

const (
	// EatTicks is how long an NPC stands eating or drinking.
	EatTicks = 3 * TicksPerSecond
	// mealReach is how far from a waypoint food or drink is eaten.
	mealReach = 1.6
)

// Eating reports whether NPC k is eating or drinking (at MealAt).
func (s *Sim) Eating(k int) bool { return s.B.Bool(s.npcR(k), "eating") }

// Convulsing reports whether NPC k is in the poison's death throes.
func (s *Sim) Convulsing(k int) bool { return s.B.Bool(s.npcR(k), "convulsing") }

// Poisoned reports whether object i (food or drink) is poisoned.
func (o *Object) Poisoned() bool { return o.is("poisoned") }

// edible reports whether object i is food or drink to eat.
func (s *Sim) edible(i int) bool {
	o := s.Objects[i]
	return !o.Gone && !o.Held() && (o.Kind == KindFood || o.Kind == KindDrink)
}

// eatAt has NPC k, stopped at a waypoint, eat or drink the nearest food or
// drink within reach of it (an eat stimulus). False when there is none.
func (s *Sim) eatAt(k int) bool {
	at, best, bestD := s.NPCs[k].pos(), -1, mealReach
	for i, o := range s.Objects {
		if d := o.Pos.DistXZ(at); s.edible(i) && d < bestD && o.Pos.Y > at.Y-0.5 && o.Pos.Y < at.Y+1.5 {
			best, bestD = i, d
		}
	}
	if best < 0 {
		return false
	}
	r, by := s.objR(best), s.npcR(k)
	return s.Stimulate(Stimulus{Stim: StimEat, Intensity: 1, Target: &r, Source: &by, Origin: s.Objects[best].Pos})
}

// feed is food or drink i eaten by the NPC that ate it: it stands eating,
// and is poisoned if the food is.
func (s *Sim) feed(i int, d Delivery, _ []string) bool {
	if d.St.Source == nil {
		return false
	}
	k := s.npcIndex(*d.St.Source)
	if k < 0 {
		return false
	}
	n, o := s.NPCs[k], s.Objects[i]
	s.B.SetFor(s.npcR(k), "eating", true, EatTicks)
	n.MealAt, n.Wait = o.Pos, max(n.Wait, EatTicks)
	if o.Poisoned() {
		src := s.objR(i)
		s.hit(s.npcR(k), Stimulus{Stim: StimPoison, Intensity: 1, Source: &src, Origin: o.Pos})
	}
	return true
}

// poisoned shows food or drink i just poisoned.
func (s *Sim) poisoned(i int, _ Delivery, _ []string) bool {
	o := s.Objects[i]
	s.event(Event{Kind: EvPoisoned, Pos: o.Pos})
	s.message("Poisoned the " + lower(o.Name) + ".")
	return true
}

// PoisonTarget pours a poison vial into the food or drink t: true when it
// took (a vial used); food already poisoned takes no more.
func (s *Sim) PoisonTarget(t Target) bool {
	p := &s.Player
	if p.Ammo[AmmoPoison] <= 0 || t.Obj < 0 || !s.edible(t.Obj) {
		return false
	}
	r, pr := s.objR(t.Obj), s.playerR()
	if !s.Stimulate(Stimulus{Stim: StimPoison, Intensity: 1, Target: &r, Source: &pr, Origin: s.Objects[t.Obj].Pos}) {
		return false
	}
	p.Ammo[AmmoPoison]--
	return true
}

// takeBottle is the player taking a bottle to throw or swing.
func (s *Sim) takeBottle(i int, _ Delivery, _ []string) bool {
	s.Player.Ammo[AmmoBottle]++
	s.emit(s.Objects[i].Pos, 6, "swish", -1)
	s.message("Took a bottle")
	s.B.Destroy(s.objR(i))
	return true
}

// ThrowItem throws one of the player's bottles or poison vials (ammo) dist
// blocks along dir: it flies as a thrown torch does (fly) and breaks where
// it lands (land). False with none left.
func (s *Sim) ThrowItem(ammo string, eye, dir Vec3, dist float64, hard bool) bool {
	p := &s.Player
	if p.Ammo[ammo] <= 0 {
		return false
	}
	p.Ammo[ammo]--
	o := &Object{ID: "thrown_" + ammo + "_" + itoa(len(s.Objects)), Kind: KindAmmo, Name: ammo, Count: 1}
	if ammo == AmmoBottle {
		o.Kind, o.Name = KindDrink, Bottle
	}
	i := s.addObject(o)
	s.B.Set(s.objR(i), "flying", true)
	o.Flight, o.Hard = 1, hard
	o.Yaw = yawTo(Vec3{}, dir)
	o.Pos, o.Vel = s.launch(eye, dir, dist)
	o.Prev = o.Pos
	return true
}

func lower(s string) string {
	b := []byte(s)
	if len(b) > 0 && b[0] >= 'A' && b[0] <= 'Z' {
		b[0] += 'a' - 'A'
	}
	return string(b)
}
