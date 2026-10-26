package sim

import (
	"math"

	"github.com/memmaker/stimres"
	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/ragdoll"
)

// The household's reactions to what happens around it: people on fire,
// oil underfoot, footsteps on creaking wood, being struck, comrades out
// cold, torches put out and mess on the floor. Each is a receptron in
// stimuli.toml calling one of these; chores are what an NPC then walks
// over to do about it.

// Chores an NPC can take on.
const (
	ChoreRevive  = "revive"  // shake a comrade awake (Of: the NPC)
	ChoreRelight = "relight" // light a torch put out (Of: the object)
	ChoreMop     = "mop"     // wipe up a puddle (At: its cell)
)

const choreWork = 3 * TicksPerSecond // ticks spent at it

// addReactions adds this file's responses (stim.go's init calls it).
func addReactions() {
	npc := func(f func(s *Sim, k int, d Delivery, args []string) bool) response {
		return func(s *Sim, d Delivery, args []string) bool { return f(s, s.npcIndex(d.To), d, args) }
	}
	for name, r := range map[string]response{
		"catch_fire_npc": npc((*Sim).catchFireNPC),
		"doused_npc": npc(func(s *Sim, k int, _ Delivery, _ []string) bool {
			s.bark(k, "Thank God...")
			return true
		}),
		"scorch_npc": npc(func(s *Sim, k int, _ Delivery, _ []string) bool {
			if s.B.Add(s.npcR(k), "health", -1); s.NPCs[k].Health() <= 0 {
				s.killNPC(k, "accident", "burned")
			}
			return true
		}),
		"slip_npc":     npc((*Sim).slipNPC),
		"startle":      npc((*Sim).startle),
		"revive":       npc((*Sim).revive),
		"notice_dark":  npc((*Sim).noticeDark),
		"notice_spill": npc((*Sim).noticeSpill),
		"catch_fire_player": func(s *Sim, _ Delivery, _ []string) bool {
			s.message("You're on fire! Find water!")
			return true
		},
		"doused_player": func(s *Sim, _ Delivery, _ []string) bool { s.message("The flames are out."); return true },
		"slip_player": func(s *Sim, _ Delivery, _ []string) bool {
			if !s.Player.OnGround || s.Player.moved < 0.02 {
				return false
			}
			s.event(Event{Kind: EvSlip, Pos: s.Player.Pos})
			s.emit(s.Player.Pos, 22, "thud", -1)
			return true
		},
		"footstep":       footstep,
		"strain":         strain,
		"support_broken": func(s *Sim, d Delivery, _ []string) bool { return supportBroken(s, d.To.Addr, d) },
		"crush_npc": npc(func(s *Sim, k int, d Delivery, _ []string) bool {
			if !crushed(s, d) {
				return false
			}
			s.killNPC(k, "accident", "crushed")
			return true
		}),
		"crush_player": func(s *Sim, d Delivery, _ []string) bool {
			if !crushed(s, d) {
				return false
			}
			s.HurtPlayer(20, "Crushed.")
			return true
		},
		"topple": (*Sim).topple,
	} {
		responses[name] = r
	}
}

// Burning people.

// Burning reports whether NPC k is on fire.
func (s *Sim) Burning(k int) bool { return s.B.Bool(s.npcR(k), "burning") }

// PlayerBurning reports whether the player is on fire.
func (s *Sim) PlayerBurning() bool { return s.B.Bool(s.playerR(), "burning") }

// catchFireNPC: its clothes catch. It screams and runs for water (act).
func (s *Sim) catchFireNPC(k int, _ Delivery, _ []string) bool {
	n := s.NPCs[k]
	n.Chore, n.Path = "", nil
	s.bark(k, pick(k, "I'm on fire!", "Water! Water!", "Aaargh, it burns!"))
	s.emit(n.eye(), 60, "pain", k)
	return true
}

// runBurning is a burning NPC's tick: it sprints for the nearest water,
// or about at random if there is none.
func (s *Sim) runBurning(k int) {
	n := s.NPCs[k]
	in := character.Input{Yaw: n.Yaw, Sprint: true}
	best, bd := Vec3{}, math.Inf(1)
	for c, l := range s.Liquid {
		if d := c.Floor().Dist(n.pos()); l == StimWater && d < bd && d < 30 {
			best, bd = c.Floor(), d
		}
	}
	if bd < math.Inf(1) && n.NoPath == 0 {
		s.walkTo(k, best, 1, &in)
	} else {
		if n.StateT%10 == 0 {
			n.Wander = s.rng.Float64()*2 - 1
		}
		n.Yaw += n.Wander * 0.2
		in.Yaw, in.Forward = n.Yaw, 1
	}
	n.StateT++
	s.move(k, in)
}

// Oil.

// slipNPC: oil underfoot takes a walking NPC's feet away. It goes down as
// if kicked (gentler) and gets up dazed.
func (s *Sim) slipNPC(k int, _ Delivery, _ []string) bool {
	n := s.NPCs[k]
	if !n.alive() || !n.Moving || !n.Body.OnGround || s.Airborne(k) {
		return false
	}
	ragdoll.Kick(&n.Body, n.Yaw, kickPower*0.3, 0.15)
	n.Flight, n.Path = ragdoll.Flight{}, nil
	s.B.Set(s.npcR(k), "airborne", true)
	s.emit(n.pos(), 22, "thud", k)
	s.bark(k, pick(k, "Whoa!", "Argh, my back!", "Who spilled this?!"))
	return true
}

// Being struck.

// startle is the impact receptron of an NPC still standing after it: it
// turns on where the blow came from.
func (s *Sim) startle(k int, d Delivery, _ []string) bool {
	n := s.NPCs[k]
	if !n.alive() || n.State() == StateCombat || n.State() == StateFlee || n.State() == StateCower {
		return false
	}
	from := d.St.Origin
	if d.St.Source != nil {
		from = s.B.World.Locate(*d.St.Source)
	}
	n.LKP = from
	n.setSuspicion(math.Max(n.Suspicion(), InvestigateAt+10))
	n.Stirred = true
	if n.IsGuard() {
		s.setState(k, StateSearch)
		s.bark(k, pick(k, "Who did that?!", "Show yourself!"))
	} else {
		s.setState(k, StateFlee)
		s.bark(k, "Help!")
	}
	return true
}

// Chores.

// setChore gives NPC k a chore unless it already has one.
func (s *Sim) setChore(k int, kind string, of int, at Vec3) bool {
	n := s.NPCs[k]
	if n.Chore != "" || s.Burning(k) {
		return false
	}
	n.Chore, n.ChoreOf, n.ChoreAt, n.ChoreT = kind, of, at, 0
	n.Path = nil
	return true
}

// doChore walks to the chore and works at it; it reports false when the
// NPC has none it can do now.
func (s *Sim) doChore(k int, in *character.Input) bool {
	n := s.NPCs[k]
	if n.Chore == "" || n.State() == StateCombat || n.State() == StateFlee || n.State() == StateCower {
		return false
	}
	if !s.choreStands(k) {
		n.Chore = ""
		return false
	}
	at := n.ChoreAt
	if n.Chore == ChoreRevive {
		at = s.NPCs[n.ChoreOf].pos()
	}
	if n.pos().DistXZ(at) > 1.3 || math.Abs(n.pos().Y-at.Y) > 1.5 {
		if !s.walkTo(k, at, 0.6, in) && n.NoPath > 0 {
			n.Chore = "" // can't get there
		}
		return true
	}
	s.turn(n, yawTo(n.pos(), at), 0.2)
	if n.ChoreT++; n.ChoreT < choreWork {
		return true
	}
	me, kind := s.npcR(k), n.Chore
	n.Chore = ""
	switch kind {
	case ChoreRevive:
		s.hit(s.npcR(n.ChoreOf), Stimulus{Stim: StimFrob, Origin: n.pos(), Source: &me, Extra: map[string]any{"revive": true}})
	case ChoreRelight:
		s.hit(s.objR(n.ChoreOf), Stimulus{Stim: StimFire, Intensity: 0.5, Origin: n.ChoreAt, Source: &me})
	case ChoreMop:
		s.hit(cellR(n.ChoreAt.Cell()), Stimulus{Stim: StimMop, Origin: n.ChoreAt, Source: &me})
	}
	return true
}

// choreStands reports whether NPC k's chore still needs doing.
func (s *Sim) choreStands(k int) bool {
	n := s.NPCs[k]
	switch n.Chore {
	case ChoreRevive:
		o := s.NPCs[n.ChoreOf]
		return o.State() == StateUnconscious && !o.Hidden && !o.CarriedBy && !s.Airborne(n.ChoreOf)
	case ChoreRelight:
		o := s.Objects[n.ChoreOf]
		return !o.Gone && !o.Lit() && !o.Fallen() && !o.Held()
	case ChoreMop:
		c := n.ChoreAt.Cell()
		return s.Liquid[c] != "" && !s.OilBurning(c)
	}
	return false
}

// revive is a comrade's frob on an NPC out cold: it comes to and gets up
// (rising), looking for whoever did it.
func (s *Sim) revive(k int, _ Delivery, _ []string) bool {
	n, r := s.NPCs[k], s.npcR(k)
	if n.State() != StateUnconscious {
		return false
	}
	s.B.Clear(r, "down")
	n.setState(StateSearch)
	n.StateT = 0
	n.setSuspicion(90)
	n.alert(0.5)
	s.B.SetFor(r, "rising", true, RiseTicks)
	s.bark(k, pick(k, "Ugh... my head...", "What hit me?"))
	return true
}

// noticeDark is a guard seeing a torch put out where it hangs.
func (s *Sim) noticeDark(k int, d Delivery, _ []string) bool {
	n, i := s.NPCs[k], s.objIndex(*d.St.Source)
	if i < 0 || n.State() > StateSuspicious || !s.canSee(k, d.St.Origin, 12, 0) || !s.setChore(k, ChoreRelight, i, s.Objects[i].Pos) {
		return false
	}
	n.setSuspicion(math.Max(n.Suspicion(), SuspiciousAt))
	n.LKP = d.St.Origin
	s.bark(k, pick(k, "Who put out this torch?", "Blasted draught."))
	return true
}

// noticeSpill is a servant seeing a puddle on the floor.
func (s *Sim) noticeSpill(k int, d Delivery, _ []string) bool {
	n := s.NPCs[k]
	if n.State() > StateSuspicious || !s.canSee(k, d.St.Origin, 10, 0.05) || !s.setChore(k, ChoreMop, -1, d.St.Origin) {
		return false
	}
	s.bark(k, pick(k, "Who made this mess?", "Look at this floor..."))
	return true
}

// Footsteps.

// step delivers a footstep of loudness factor f by maker (an NPC index,
// or -1 for the player) to the cell it lands on.
func (s *Sim) step(pos Vec3, f float64, maker int) {
	st := Stimulus{Stim: StimStep, Origin: pos, Intensity: f}
	if maker >= 0 {
		r := s.npcR(maker)
		st.Source = &r
	} else {
		r := s.playerR()
		st.Source = &r
	}
	s.hit(cellR(s.stepCell(pos)), st)
}

// stepCell is the cell a footstep at pos lands on: the puddle, the slab,
// the rung, else the block below.
func (s *Sim) stepCell(pos Vec3) Cell {
	c := pos.Cell()
	if s.liquidAt(c) != "" || pos.Y-math.Floor(pos.Y) > 0.3 || s.climbable(c) {
		return c
	}
	return c.Add(0, -1, 0)
}

func (s *Sim) climbable(c Cell) bool {
	cl, ok := s.W.(character.Climber)
	return ok && cl.Climbable(c.X, c.Y, c.Z)
}

// footstep is a cell's step receptron: the sound of what it is, at the
// stride's loudness; "footstep creak" adds the groan of old wood.
func footstep(s *Sim, d Delivery, args []string) bool {
	by := -1
	if d.St.Source != nil {
		by = s.npcIndex(*d.St.Source)
	}
	loud := Material(s.FloorSurface(d.St.Origin)) * d.Intensity
	if len(args) > 0 {
		loud = math.Max(loud, 18*d.Intensity)
		s.emit(d.St.Origin, loud, "creak", by)
		return true
	}
	s.emit(d.St.Origin, loud, "footstep", by)
	return true
}

// Decoration.

// Toppled reports whether decoration i was knocked over, and which way.
func (o *Object) Toppled() (bool, float64) {
	if !o.is("toppled") {
		return false, 0
	}
	return true, o.b.Float(stimres.Object[Cell](o.R), "toppled_yaw")
}

// topple knocks a small decoration over the way it was kicked.
func (s *Sim) topple(d Delivery, _ []string) bool {
	i := s.objIndex(d.To)
	if i < 0 {
		return false
	}
	o := s.Objects[i]
	if sz, ok := DecorSize[o.Name]; ok && (sz.Y > 1.6 || sz.X > 0.6 || sz.Z > 0.6) {
		return false
	}
	yaw, _ := d.St.Extra["yaw"].(float64)
	s.B.Set(d.To, "toppled", true)
	s.B.Set(d.To, "toppled_yaw", yaw)
	return true
}
