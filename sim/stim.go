package sim

import "math"

// Stim is a kind of stimulus, Dark Engine style. Sources (arrows, blades,
// fire, explosions, faucets, leaking barrels) emit stimuli with Stimulate;
// objects respond through their receptrons. Whatever emits water or fire
// thus has the same effect on everything it reaches.
type Stim string

const (
	StimWater  Stim = "water"  // douses fire, puddles the ground
	StimFire   Stim = "fire"   // ignites flammables, lights torches
	StimPierce Stim = "pierce" // broadheads, the blade
	StimBash   Stim = "bash"   // the blackjack
	StimBlast  Stim = "blast"  // explosions
)

// Stimulus is one emission: it reaches every object whose box is within
// Radius of Pos.
type Stimulus struct {
	Stim   Stim
	Pos    Vec3
	Radius float64
	Amount int // water: puddle cells it adds to the ground at Pos
}

// response is a receptron's effect on object i; it reports whether the
// object reacted.
type response func(s *Sim, i int, st Stimulus) bool

// receptrons are each archetype's responses, by Kind, or by Name for
// decoration. Torch variants (lanterns, candles) are torches.
// ponytail: a Go table; move it to a data file when missions need their own archetypes.
var receptrons map[string]map[Stim]response

func init() { // in init: leak stimulates in turn
	receptrons = map[string]map[Stim]response{
		KindTorch:     {StimWater: (*Sim).douse, StimFire: (*Sim).kindle, StimBlast: (*Sim).destroy},
		KindFlammable: {StimWater: (*Sim).douse, StimFire: (*Sim).kindle},
		KindLamp:      {StimBlast: (*Sim).destroy},
		KindDoor:      {StimBlast: (*Sim).destroy},
		KindFaucet:    {StimBlast: (*Sim).destroy},
		"Barrel":      {StimPierce: (*Sim).leak},
	}
}

// Stimulate delivers st to every object in reach and, for water, the
// ground. It reports whether any object reacted.
func (s *Sim) Stimulate(st Stimulus) bool {
	reacted := false
	for i, o := range s.Objects {
		if o.Gone {
			continue
		}
		key := o.Kind
		if o.Kind == KindDecor {
			key = o.Name
		}
		r := receptrons[key][st.Stim]
		if r == nil {
			continue
		}
		lo, hi := s.ObjectBox(i)
		dx := st.Pos.X - clamp(st.Pos.X, lo.X, hi.X)
		dy := st.Pos.Y - clamp(st.Pos.Y, lo.Y, hi.Y)
		dz := st.Pos.Z - clamp(st.Pos.Z, lo.Z, hi.Z)
		if math.Sqrt(dx*dx+dy*dy+dz*dz) <= st.Radius && r(s, i, st) {
			reacted = true
		}
	}
	if st.Stim == StimWater {
		for range st.Amount {
			s.spreadWater(st.Pos.Add(Vec3{0, 0.3, 0}).Cell(), 14)
		}
	}
	return reacted
}

// douse puts out a lit torch or a fire.
func (s *Sim) douse(i int, _ Stimulus) bool {
	o := s.Objects[i]
	if !o.Lit && !o.Burning() {
		return false
	}
	o.Lit, o.Burn = false, 0
	s.setLight(i, false)
	s.emit(o.Pos, 12, "hiss", -1)
	return true
}

// kindle sets a flammable alight or lights a torch.
func (s *Sim) kindle(i int, _ Stimulus) bool {
	o := s.Objects[i]
	switch {
	case o.Kind == KindFlammable && !o.Burning() && !o.Burnt:
		o.Burn = 1
	case o.Kind == KindTorch && !o.Lit:
		o.Lit = true
	default:
		return false
	}
	s.setLight(i, true)
	s.emit(o.Pos, 35, "ignite", -1)
	return true
}

// destroy blows an object away (a door leaves its opening).
func (s *Sim) destroy(i int, _ Stimulus) bool {
	o := s.Objects[i]
	if o.Held {
		return false
	}
	s.setLight(i, false)
	if o.Kind == KindDoor {
		for _, dc := range o.doorCells() {
			s.W.SetBlock(dc.X, dc.Y, dc.Z, BlockAir)
			delete(s.doors, dc)
		}
	}
	o.Gone = true
	return true
}

// leak is a punctured barrel: its water splashes out around its foot.
func (s *Sim) leak(i int, st Stimulus) bool {
	s.emit(st.Pos, 18, "splash", -1)
	s.Stimulate(Stimulus{Stim: StimWater, Pos: s.Objects[i].Pos, Radius: 1, Amount: 4})
	return true
}
