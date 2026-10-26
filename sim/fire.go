package sim

import (
	"cmp"
	"slices"
	"strings"
)

// Fire spreads like Minecraft's fire block (FireBlock.tick, 1.20, Normal
// difficulty, no rain): a burning cell ages 0..15 on fire ticks 30..39
// ticks apart, dies without fuel beside it, eats flammable blocks on its
// faces and leaps to empty cells beside fuel, from 1 below to 4 above.
// A burning cell flickers every tick (fireTick). Each reach is a fire stimulus whose Source is the burning cell; the
// blocks and empty cells answer through their receptrons (burnBlock,
// catchFire). A burning cell is an air cell with the properties burning,
// fire_age (0..15) and fire_next (ticks to its next fire tick).

// fire is a burning cell's state.
type fire struct {
	Age  int // 0..15
	Next int // ticks to its next fire tick
}

// fireAt is the fire burning in c, if any.
func (s *Sim) fireAt(c Cell) (fire, bool) {
	r := cellR(c)
	if _, ok := s.B.Get(r, "fire_age"); !ok || !s.B.Bool(r, "burning") {
		return fire{}, false
	}
	return fire{Age: int(s.B.Float(r, "fire_age")), Next: int(s.B.Float(r, "fire_next"))}, true
}

func (s *Sim) setFire(c Cell, f fire) {
	r := cellR(c)
	s.B.Set(r, "burning", true)
	s.B.Set(r, "fire_age", f.Age)
	s.B.Set(r, "fire_next", f.Next)
}

// FireCells are the burning cells, in a fixed order (same seed, same fire).
func (s *Sim) FireCells() []Cell {
	var out []Cell
	for _, c := range s.B.Addresses() {
		if _, ok := s.fireAt(c); ok {
			out = append(out, c)
		}
	}
	slices.SortFunc(out, func(a, b Cell) int {
		return cmp.Or(cmp.Compare(a.X, b.X), cmp.Compare(a.Y, b.Y), cmp.Compare(a.Z, b.Z))
	})
	return out
}

var faces = [6]Cell{{1, 0, 0}, {-1, 0, 0}, {0, 0, 1}, {0, 0, -1}, {0, -1, 0}, {0, 1, 0}}

var woods = []string{"oak", "spruce", "birch", "jungle", "acacia", "dark_oak", "mangrove", "cherry", "bamboo", "pale_oak"}

// flammability is a block's Minecraft ignite odds (how it encourages fire
// beside it) and burn odds (how fast fire eats it); 0, 0 is fireproof.
func flammability(block string) (ignite, burn int) {
	b := block[strings.IndexByte(block, ':')+1:] // any namespace
	b, _, _ = strings.Cut(b, "[")
	wooden := false
	for _, w := range woods {
		wooden = wooden || strings.HasPrefix(b, w+"_")
	}
	switch {
	case strings.HasSuffix(b, "_planks"),
		wooden && (strings.HasSuffix(b, "_slab") || strings.HasSuffix(b, "_stairs") || strings.HasSuffix(b, "_fence") || strings.HasSuffix(b, "_fence_gate")),
		b == "composter", b == "beehive", b == "bee_nest":
		return 5, 20
	case b == "dark_trunk_log": // dead, dry wood: catches like tinder
		return 30, 20
	case strings.HasSuffix(b, "_log"), strings.HasSuffix(b, "_wood"), b == "coal_block":
		return 5, 5
	case strings.HasSuffix(b, "_leaves"), strings.HasSuffix(b, "_wool"), b == "dried_kelp_block":
		return 30, 60
	case b == "bookshelf", b == "chiseled_bookshelf", b == "lectern":
		return 30, 20
	case strings.HasSuffix(b, "_carpet"), b == "hay_block":
		return 60, 20
	case b == "short_grass", b == "fern", b == "tall_grass", b == "large_fern", b == "dead_bush":
		return 60, 100
	case b == "vine", b == "tnt", b == "target", b == "ladder":
		return 15, 100
	case b == "scaffolding":
		return 60, 60
	}
	return 0, 0
}

// empty is air (or our invisible light): where fire can burn.
func (s *Sim) empty(c Cell) bool {
	b := s.W.Block(c.X, c.Y, c.Z)
	return b == BlockAir || b == BlockLight
}

func (s *Sim) burnable(c Cell) bool { _, b := flammability(s.W.Block(c.X, c.Y, c.Z)); return b > 0 }

// igniteOdds is an empty cell's chance to catch: its most flammable
// face neighbour's ignite odds.
func (s *Sim) igniteOdds(c Cell) int {
	if !s.empty(c) {
		return 0
	}
	best := 0
	for _, d := range faces {
		n := c.Add(d.X, d.Y, d.Z)
		i, _ := flammability(s.W.Block(n.X, n.Y, n.Z))
		best = max(best, i)
	}
	return best
}

func (s *Sim) fireDelay() int { return 30 + s.rng.Intn(10) }

// ignite sets the empty cell c burning.
func (s *Sim) ignite(c Cell, age int) {
	if _, ok := s.fireAt(c); ok || !s.empty(c) {
		return
	}
	s.setFire(c, fire{Age: min(age, 15), Next: s.fireDelay()})
	s.W.SetBlock(c.X, c.Y, c.Z, BlockLight)
}

func (s *Sim) extinguish(c Cell) {
	r := cellR(c)
	s.B.Clear(r, "burning")
	s.B.Clear(r, "fire_age")
	s.B.Clear(r, "fire_next")
	s.unlight(c)
}

// fireTick is a burning cell's flicker (a self source every tick): its
// flames burn what stands in it, and on its fire tick it ages, dies out
// or spreads.
func fireTick(s *Sim, c Cell, _ Delivery) bool {
	f, ok := s.fireAt(c)
	if !ok { // put out this tick, or burning oil
		return false
	}
	if s.Now%10 == 0 { // flames: burn what stands in the cell
		s.Stimulate(s.fireStim(c, c.Center(), 0.45, 1))
	}
	if f.Next--; f.Next > 0 {
		s.setFire(c, f)
		return true
	}
	f.Next = s.fireDelay()
	f.Age = min(15, f.Age+s.rng.Intn(3)/2)
	s.setFire(c, f)
	below := c.Add(0, -1, 0)
	fuel := false
	for _, d := range faces {
		fuel = fuel || s.burnable(c.Add(d.X, d.Y, d.Z))
	}
	if !fuel && (!s.W.Solid(below.X, below.Y, below.Z) || f.Age > 3) ||
		f.Age == 15 && s.rng.Intn(4) == 0 && !s.burnable(below) {
		s.extinguish(c)
		return true
	}
	s.emit(c.Floor(), 30, "fire", -1)
	for _, d := range faces { // eat the blocks around
		if n := c.Add(d.X, d.Y, d.Z); !s.empty(n) {
			s.Stimulate(s.fireStim(c, n.Center(), 0.01, 0))
		}
	}
	for dx := -1; dx <= 1; dx++ { // leap to fuel nearby
		for dz := -1; dz <= 1; dz++ {
			for dy := -1; dy <= 4; dy++ {
				if n := c.Add(dx, dy, dz); n != c && s.igniteOdds(n) > 0 {
					s.Stimulate(s.fireStim(c, n.Center(), 0.01, 0))
				}
			}
		}
	}
	return true
}

// fireStim is fire from the burning cell c reaching around pos.
func (s *Sim) fireStim(c Cell, pos Vec3, radius, intensity float64) Stimulus {
	st := at(StimFire, pos, radius, intensity)
	from := cellR(c)
	st.Source = &from
	return st
}

// Cell responses.

// burnBlock: a fire on a face eats the block, which may catch in turn.
func burnBlock(s *Sim, c Cell, d Delivery) bool {
	f, from, ok := s.fireFrom(d)
	if !ok {
		return false
	}
	_, burn := flammability(s.W.Block(c.X, c.Y, c.Z))
	spread := 300
	if c.Y != from.Y {
		spread = 250
	}
	if s.rng.Intn(spread) >= burn {
		return false
	}
	s.W.SetBlock(c.X, c.Y, c.Z, BlockAir)
	s.reroom = true
	if s.rng.Intn(f.Age+10) < 5 {
		s.ignite(c, f.Age+s.rng.Intn(5)/4)
	}
	return true
}

// primeTNT: TNT that burns (as burnBlock eats it) or is caught in a blast
// is lit like Minecraft's: 4 s of fuse, half a second to 1.5 s in a blast.
func primeTNT(s *Sim, c Cell, d Delivery) bool {
	fuse := 4 * TicksPerSecond
	switch {
	case d.St.Stim == StimBlast:
		s.W.SetBlock(c.X, c.Y, c.Z, BlockAir)
		s.reroom = true
		fuse = 10 + s.rng.Intn(20)
	case !burnBlock(s, c, d):
		return false
	}
	s.Bombs = append(s.Bombs, bomb{Pos: c.Center(), Fuse: fuse})
	s.emit(c.Center(), 30, "fuse", -1)
	return true
}

// catchFire: an empty cell beside fuel catches from a fire nearby (the
// higher above it, the less likely) or from open flame (burning oil, a rug, a torch).
func catchFire(s *Sim, c Cell, d Delivery) bool {
	odds := s.igniteOdds(c)
	age, k := 0, 100
	f, from, fromFire := s.fireFrom(d)
	if fromFire {
		age = f.Age
		if dy := c.Y - from.Y; dy > 1 {
			k += (dy - 1) * 100
		}
	} else if d.St.Intensity <= 0 { // heat, or a dead fire
		return false
	}
	o := (odds + 40 + 2*7) / (age + 30)
	if !fromFire { // flame touching fuel: its ignite odds straight, so a torch lights a hedge, rarely planks
		o = odds
	}
	if odds == 0 || o <= 0 || s.rng.Intn(k) > o {
		return false
	}
	s.ignite(c, age+s.rng.Intn(5)/4)
	return true
}

// douseFire: water puts a burning cell out.
func douseFire(s *Sim, c Cell, _ Delivery) bool {
	s.extinguish(c)
	s.emit(c.Floor(), 12, "hiss", -1)
	return true
}

// fireFrom is the burning cell a fire stimulus comes from.
func (s *Sim) fireFrom(d Delivery) (fire, Cell, bool) {
	if src := d.St.Source; src != nil && src.Obj == 0 {
		f, ok := s.fireAt(src.Addr)
		return f, src.Addr, ok
	}
	return fire{}, Cell{}, false
}
