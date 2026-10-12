package sim

import (
	"math"
	"slices"
)

// Stim is a kind of stimulus, Dark Engine style. Everything that acts on
// the world emits stimuli with Stimulate, and objects, NPCs, blocks and
// puddles respond only through their receptrons, so whatever emits water,
// fire or noise has the same effect on everything it reaches, and effects
// chain (a pierced barrel's water douses the torch beside it).
type Stim string

const (
	StimWater  Stim = "water"  // douses fire, pools as a puddle
	StimOil    Stim = "oil"    // pools as a burnable puddle
	StimFire   Stim = "fire"   // ignites; Amount > 0 is open flame (catches blocks), >= 1 flames that burn people
	StimShock  Stim = "shock"  // electricity: live in water, kills
	StimPierce Stim = "pierce" // broadheads, the blade
	StimBash   Stim = "bash"   // the blackjack
	StimBlast  Stim = "blast"  // explosions
	StimNoise  Stim = "noise"  // sound: reaches NPCs through the room graph, not Radius
)

// Stimulus is one emission. It reaches everything whose box is within
// Radius of Pos.
type Stimulus struct {
	Stim   Stim
	Pos    Vec3
	Radius float64
	Amount float64 // liquid: cells poured; pierce: damage; noise: loudness; fire: > 0 open flame, >= 1 burns people
	Kind   string  // noise: the sound; pierce: how it hurts ("shot", "stabbed")
	Head   bool    // pierce: a head hit
	Source int     // noise: the NPC making it (it doesn't react to itself), -1 none
	From   *Cell   // fire: the burning cell it spreads from (see fire.go)
}

type (
	// response is an object's (or NPC's) receptron: it reports whether i reacted.
	response func(s *Sim, i int, st Stimulus) bool
	// cellResponse is a block's or a puddle's receptron.
	cellResponse func(s *Sim, c Cell, st Stimulus) bool
)

// Receptrons. Objects by Kind, or by Name for decoration (torch variants
// such as lanterns and candles are torches); every NPC alike; blocks by
// material; puddles by liquid; burning cells alike.
// ponytail: Go tables; move them to a data file when missions need their own archetypes.
var (
	receptrons       map[string]map[Stim]response
	npcReceptrons    map[Stim]response
	playerReceptrons map[Stim]func(s *Sim, st Stimulus) bool
	blockReceptrons  map[string]map[Stim]cellResponse
	liquidReceptrons map[Stim]map[Stim]cellResponse
	fireReceptrons   map[Stim]cellResponse // burning cells
)

func init() { // in init: responses stimulate in turn
	receptrons = map[string]map[Stim]response{
		KindTorch:     {StimWater: (*Sim).douse, StimFire: (*Sim).kindle, StimBlast: (*Sim).destroy},
		KindFlammable: {StimWater: (*Sim).douse, StimFire: (*Sim).kindle},
		KindLamp:      {StimWater: (*Sim).shortOut, StimBlast: (*Sim).destroy},
		KindDoor:      {StimBlast: (*Sim).destroy},
		KindFaucet:    {StimBlast: (*Sim).destroy},
		"Barrel":      {StimPierce: leak(StimWater), StimBlast: burst(StimWater)},
		"Oil barrel":  {StimPierce: leak(StimOil), StimBlast: burst(StimOil)},
	}
	blockReceptrons = map[string]map[Stim]cellResponse{
		"glass":     {StimPierce: shatter, StimBash: shatter, StimBlast: shatter},
		"flammable": {StimFire: burnBlock},
		"tnt":       {StimFire: primeTNT, StimBlast: primeTNT},
		"empty":     {StimFire: catchFire},
	}
	fireReceptrons = map[Stim]cellResponse{StimWater: douseFire}
	liquidReceptrons = map[Stim]map[Stim]cellResponse{
		StimWater: {StimShock: (*Sim).electrifyAt},
		StimOil:   {StimFire: (*Sim).igniteOil, StimWater: (*Sim).douseOil},
	}
	npcReceptrons = map[Stim]response{
		StimPierce: (*Sim).wound,
		StimBash:   (*Sim).knockOut,
		StimBlast:  func(s *Sim, k int, _ Stimulus) bool { s.killNPC(k, "player", "blown up"); return true },
		StimFire:   (*Sim).burnNPC,
		StimShock:  func(s *Sim, k int, _ Stimulus) bool { s.killNPC(k, "accident", "electrocuted"); return true },
		StimNoise:  (*Sim).hearNoise,
	}
	playerReceptrons = map[Stim]func(s *Sim, st Stimulus) bool{
		StimShock: func(s *Sim, _ Stimulus) bool { s.HurtPlayer(20, "Electrocuted."); return true },
		StimFire: func(s *Sim, st Stimulus) bool { // flames, not heat
			if st.Amount >= 1 {
				s.HurtPlayer(10, "You're burning!")
			}
			return st.Amount >= 1
		},
		StimBlast: func(s *Sim, st Stimulus) bool {
			s.HurtPlayer(max(1, int(24-st.Pos.Dist(s.Player.Pos)*5)), "Caught in the blast.")
			return true
		},
	}
}

// liquid is the puddle map of a liquid stim: cell -> ticks of electricity
// (water) or of fire (oil) left.
func (s *Sim) liquid(l Stim) map[Cell]int {
	if l == StimOil {
		return s.Oil
	}
	return s.Puddles
}

var liquidBlock = map[Stim]string{StimWater: BlockPuddle, StimOil: BlockOil}

// Stimulate delivers st to every object, NPC, block and puddle in reach,
// then pours a liquid stim's Amount. It reports whether anything reacted.
func (s *Sim) Stimulate(st Stimulus) bool {
	if st.Stim == StimNoise { // NPCs hear it as they perceive (see hearNoise)
		s.sounds = append(s.sounds, st)
		s.event(Event{Kind: EvSound, Sound: st.Kind, Pos: st.Pos, Loud: st.Amount, NPC: st.Source})
		return false
	}
	reacted := false
	for i, o := range s.Objects {
		if o.Gone {
			continue
		}
		key := o.Kind
		if o.Kind == KindDecor {
			key = o.Name
		}
		if r := receptrons[key][st.Stim]; r != nil {
			if lo, hi := s.ObjectBox(i); boxDist(st.Pos, lo, hi) <= st.Radius && r(s, i, st) {
				reacted = true
			}
		}
	}
	for k, n := range s.NPCs {
		b := n.Body.Box()
		if boxDist(st.Pos, Vec3{b.MinX, b.MinY, b.MinZ}, Vec3{b.MaxX, b.MaxY, b.MaxZ}) <= st.Radius && s.stimNPC(k, st) {
			reacted = true
		}
	}
	if p := s.Player.Pos; !s.Player.Dead && playerReceptrons[st.Stim] != nil &&
		boxDist(st.Pos, p.Sub(Vec3{0.3, 0, 0.3}), p.Add(Vec3{0.3, 1.8, 0.3})) <= st.Radius && playerReceptrons[st.Stim](s, st) {
		reacted = true
	}
	r := st.Radius
	lo, hi := st.Pos.Sub(Vec3{r, r, r}).Cell(), st.Pos.Add(Vec3{r, r, r}).Cell()
	for x := lo.X; x <= hi.X; x++ {
		for y := lo.Y; y <= hi.Y; y++ {
			for z := lo.Z; z <= hi.Z; z++ {
				c := Cell{x, y, z}
				if boxDist(st.Pos, Vec3{float64(x), float64(y), float64(z)}, Vec3{float64(x + 1), float64(y + 1), float64(z + 1)}) > r {
					continue
				}
				mat := ""
				switch b := s.W.Block(x, y, z); {
				case IsGlass(b):
					mat = "glass"
				case b == BlockAir || b == BlockLight:
					mat = "empty"
				case b == BlockTNT:
					mat = "tnt"
				case s.burnable(c):
					mat = "flammable"
				}
				if f := blockReceptrons[mat][st.Stim]; f != nil && f(s, c, st) {
					reacted = true
				}
				if _, ok := s.Fire[c]; ok && fireReceptrons[st.Stim] != nil && fireReceptrons[st.Stim](s, c, st) {
					reacted = true
				}
				for l, rs := range liquidReceptrons {
					if _, ok := s.liquid(l)[c]; ok && rs[st.Stim] != nil && rs[st.Stim](s, c, st) {
						reacted = true
					}
				}
			}
		}
	}
	if _, ok := liquidBlock[st.Stim]; ok && st.Amount >= 1 {
		at := st.Pos.Add(Vec3{0, 0.3, 0}).Cell()
		s.pour(st.Stim, at, 14) // a wet spot at once, the rest spreads (tickSpills)
		if n := int(st.Amount) - 1; n > 0 {
			s.Spills = append(s.Spills, spill{Liquid: st.Stim, At: at, Left: n, Next: spillEvery})
		}
	}
	return reacted
}

// stimNPC delivers st to NPC k alone, as a hit that already found it.
func (s *Sim) stimNPC(k int, st Stimulus) bool {
	if n := s.NPCs[k]; !n.alive() || n.Hidden || n.CarriedBy {
		return false
	}
	r := npcReceptrons[st.Stim]
	return r != nil && r(s, k, st)
}

// boxDist is how far p is from the box lo..hi (0 inside).
func boxDist(p, lo, hi Vec3) float64 {
	d := p.Sub(Vec3{clamp(p.X, lo.X, hi.X), clamp(p.Y, lo.Y, hi.Y), clamp(p.Z, lo.Z, hi.Z)})
	return math.Sqrt(d.X*d.X + d.Y*d.Y + d.Z*d.Z)
}

// Object responses.

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

// shortOut is water reaching a fallen, lit lamp: it dies, and its current
// goes into the water. (A standing lamp's wiring is high and dry.)
func (s *Sim) shortOut(i int, _ Stimulus) bool {
	o := s.Objects[i]
	if !o.Fallen || !o.Lit {
		return false
	}
	o.Lit = false
	s.setLight(i, false)
	s.Stimulate(Stimulus{Stim: StimShock, Pos: o.Pos, Radius: 0.2})
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

// leak is a punctured container: its liquid splashes out around its foot.
func leak(liquid Stim) response {
	return func(s *Sim, i int, st Stimulus) bool {
		s.emit(st.Pos, 18, "splash", -1)
		s.Stimulate(Stimulus{Stim: liquid, Pos: s.Objects[i].Pos, Radius: 1, Amount: 4})
		return true
	}
}

// burst is a container blown apart: it spills everything.
func burst(liquid Stim) response {
	return func(s *Sim, i int, st Stimulus) bool {
		s.destroy(i, st)
		s.Stimulate(Stimulus{Stim: liquid, Pos: s.Objects[i].Pos, Radius: 1, Amount: 8})
		return true
	}
}

// NPC responses.

// wound is a pierce: a head hit kills, so does a stab in the back of the
// unaware; anything else does its Amount.
func (s *Sim) wound(k int, st Stimulus) bool {
	n := s.NPCs[k]
	dmg := int(st.Amount)
	if st.Head || st.Kind == "stabbed" && n.State != StateCombat && n.Suspicion < 55 {
		dmg = 99
	}
	s.hurtNPC(k, dmg, st.Kind)
	return true
}

// knockOut is a bash: the unaware drop, the fighting only notice.
func (s *Sim) knockOut(k int, _ Stimulus) bool {
	n := s.NPCs[k]
	if n.State == StateCombat {
		s.emit(n.pos(), 20, "thud", -1)
		s.alertNPC(k, s.Player.Pos)
		return true
	}
	n.State = StateUnconscious
	n.Path = nil
	s.Stats.Knockouts++
	s.emit(n.pos(), 18, "body", -1)
	s.bark(k, "Ugh...")
	return true
}

// burnNPC: flames kill; heat and sparks (Amount < 1) don't.
func (s *Sim) burnNPC(k int, st Stimulus) bool {
	if st.Amount < 1 {
		return false
	}
	s.killNPC(k, "accident", "burned")
	return true
}

// Block and puddle responses.

func shatter(s *Sim, c Cell, _ Stimulus) bool { return s.Shatter(c) }

// electrifyAt puts current into a puddle that isn't live yet.
func (s *Sim) electrifyAt(c Cell, _ Stimulus) bool {
	if s.Puddles[c] > 0 {
		return false
	}
	s.electrify(c)
	return true
}

// oilBurn is how long an oil cell burns.
const oilBurn = 8 * TicksPerSecond

// igniteOil sets an oil cell burning, lighting the cell above.
func (s *Sim) igniteOil(c Cell, _ Stimulus) bool {
	if s.Oil[c] > 0 {
		return false
	}
	s.Oil[c] = oilBurn
	if a := c.Add(0, 1, 0); s.W.Block(a.X, a.Y, a.Z) == BlockAir {
		s.W.SetBlock(a.X, a.Y, a.Z, BlockLight)
	}
	s.emit(c.Floor(), 35, "ignite", -1)
	return true
}

// douseOil puts out burning oil; the oil stays.
func (s *Sim) douseOil(c Cell, _ Stimulus) bool {
	if s.Oil[c] == 0 {
		return false
	}
	s.Oil[c] = 0
	s.unlightAbove(c)
	s.emit(c.Floor(), 12, "hiss", -1)
	return true
}

func (s *Sim) unlightAbove(c Cell) { s.unlight(c.Add(0, 1, 0)) }

// spill is liquid still spreading from where it was poured.
type spill struct {
	Liquid     Stim
	At         Cell
	Left, Next int // cells to come; ticks to the next one
}

const spillEvery = TicksPerSecond / 3 // a puddle grows a cell at a time

func (s *Sim) tickSpills() {
	for i := range s.Spills {
		sp := &s.Spills[i]
		if sp.Next--; sp.Next <= 0 {
			s.pour(sp.Liquid, sp.At, 14)
			sp.Left, sp.Next = sp.Left-1, spillEvery
		}
	}
	s.Spills = slices.DeleteFunc(s.Spills, func(sp spill) bool { return sp.Left <= 0 })
}

// pour adds one cell of liquid next to the puddle around start (or start
// itself), up to maxCells in the puddle. Another liquid blocks it.
func (s *Sim) pour(liquid Stim, start Cell, maxCells int) {
	if !s.walkable(start) {
		g, ok := s.groundBelow(start, 8) // it falls until it lands
		if !ok {
			return
		}
		start = g
	}
	m := s.liquid(liquid)
	seen := map[Cell]bool{start: true}
	queue := []Cell{start}
	for n := 0; len(queue) > 0; {
		c := queue[0]
		queue = queue[1:]
		if s.liquidAt(c) == "" {
			m[c] = 0
			if s.W.Block(c.X, c.Y, c.Z) == BlockAir {
				s.W.SetBlock(c.X, c.Y, c.Z, liquidBlock[liquid])
			}
			return
		}
		if _, mine := m[c]; !mine {
			continue
		}
		if n++; n >= maxCells {
			return
		}
		dirs := []Cell{{1, 0, 0}, {-1, 0, 0}, {0, 0, 1}, {0, 0, -1}}
		s.rng.Shuffle(len(dirs), func(i, j int) { dirs[i], dirs[j] = dirs[j], dirs[i] })
		for _, d := range dirs {
			nc := c.Add(d.X, 0, d.Z)
			if !seen[nc] && s.walkable(nc) {
				seen[nc] = true
				queue = append(queue, nc)
			}
		}
	}
}

// liquidAt is the liquid lying in c, or "".
func (s *Sim) liquidAt(c Cell) Stim {
	for l := range liquidBlock {
		if _, ok := s.liquid(l)[c]; ok {
			return l
		}
	}
	return ""
}
