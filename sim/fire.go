package sim

import (
	"cmp"
	"maps"
	"slices"
	"strings"
)

// Fire spreads like Minecraft's fire block (FireBlock.tick, 1.20, Normal
// difficulty, no rain): a burning cell ages 0..15 on fire ticks 30..39
// ticks apart, dies without fuel beside it, eats flammable blocks on its
// faces and leaps to empty cells beside fuel, from 1 below to 4 above.
// Each reach is a fire stimulus From the burning cell; the blocks and
// empty cells answer through their receptrons (burnBlock, catchFire).

// fire is a burning cell.
type fire struct {
	Age  int // 0..15
	Next int // ticks to its next fire tick
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
	case b == "vine", b == "tnt", b == "target":
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
	if _, ok := s.Fire[c]; ok || !s.empty(c) {
		return
	}
	s.Fire[c] = fire{Age: min(age, 15), Next: s.fireDelay()}
	s.W.SetBlock(c.X, c.Y, c.Z, BlockLight)
}

func (s *Sim) extinguish(c Cell) {
	delete(s.Fire, c)
	s.unlight(c)
}

func (s *Sim) tickFire() {
	cells := slices.SortedFunc(maps.Keys(s.Fire), func(a, b Cell) int { // same seed, same fire
		return cmp.Or(cmp.Compare(a.X, b.X), cmp.Compare(a.Y, b.Y), cmp.Compare(a.Z, b.Z))
	})
	for _, c := range cells {
		f, ok := s.Fire[c]
		if !ok { // put out this tick
			continue
		}
		if s.Now%10 == 0 { // flames: burn what stands in the cell
			s.Stimulate(Stimulus{Stim: StimFire, Pos: c.Center(), Radius: 0.45, Amount: 1, From: &c})
		}
		if f.Next--; f.Next > 0 {
			s.Fire[c] = f
			continue
		}
		f.Next = s.fireDelay()
		f.Age = min(15, f.Age+s.rng.Intn(3)/2)
		s.Fire[c] = f
		below := c.Add(0, -1, 0)
		fuel := false
		for _, d := range faces {
			fuel = fuel || s.burnable(c.Add(d.X, d.Y, d.Z))
		}
		if !fuel && (!s.W.Solid(below.X, below.Y, below.Z) || f.Age > 3) ||
			f.Age == 15 && s.rng.Intn(4) == 0 && !s.burnable(below) {
			s.extinguish(c)
			continue
		}
		s.emit(c.Floor(), 30, "fire", -1)
		for _, d := range faces { // eat the blocks around
			if n := c.Add(d.X, d.Y, d.Z); !s.empty(n) {
				s.Stimulate(Stimulus{Stim: StimFire, Pos: n.Center(), Radius: 0.01, From: &c})
			}
		}
		for dx := -1; dx <= 1; dx++ { // leap to fuel nearby
			for dz := -1; dz <= 1; dz++ {
				for dy := -1; dy <= 4; dy++ {
					if n := c.Add(dx, dy, dz); n != c && s.igniteOdds(n) > 0 {
						s.Stimulate(Stimulus{Stim: StimFire, Pos: n.Center(), Radius: 0.01, From: &c})
					}
				}
			}
		}
	}
	if s.reroom { // burnt walls and floors open new ways
		s.reroom = false
		s.init()
	}
}

// Cell responses.

// burnBlock: a fire on a face eats the block, which may catch in turn.
func burnBlock(s *Sim, c Cell, st Stimulus) bool {
	f, ok := s.fireFrom(st)
	if !ok {
		return false
	}
	_, burn := flammability(s.W.Block(c.X, c.Y, c.Z))
	spread := 300
	if c.Y != st.From.Y {
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
func primeTNT(s *Sim, c Cell, st Stimulus) bool {
	fuse := 4 * TicksPerSecond
	switch {
	case st.Stim == StimBlast:
		s.W.SetBlock(c.X, c.Y, c.Z, BlockAir)
		s.reroom = true
		fuse = 10 + s.rng.Intn(20)
	case !burnBlock(s, c, st):
		return false
	}
	s.Bombs = append(s.Bombs, bomb{Pos: c.Center(), Fuse: fuse})
	s.emit(c.Center(), 30, "fuse", -1)
	return true
}

// catchFire: an empty cell beside fuel catches from a fire nearby (the
// higher above it, the less likely) or from open flame (burning oil, a rug, a torch).
func catchFire(s *Sim, c Cell, st Stimulus) bool {
	if _, burning := s.Fire[c]; burning {
		return false
	}
	odds := s.igniteOdds(c)
	age, k := 0, 100
	if f, ok := s.fireFrom(st); ok {
		age = f.Age
		if dy := c.Y - st.From.Y; dy > 1 {
			k += (dy - 1) * 100
		}
	} else if st.From != nil || st.Amount <= 0 { // heat, or a dead fire
		return false
	}
	o := (odds + 40 + 2*7) / (age + 30)
	if odds == 0 || o <= 0 || s.rng.Intn(k) > o {
		return false
	}
	s.ignite(c, age+s.rng.Intn(5)/4)
	return true
}

// douseFire: water puts a burning cell out.
func douseFire(s *Sim, c Cell, _ Stimulus) bool {
	s.extinguish(c)
	s.emit(c.Floor(), 12, "hiss", -1)
	return true
}

// fireFrom is the burning cell a fire stimulus comes from.
func (s *Sim) fireFrom(st Stimulus) (fire, bool) {
	if st.From == nil {
		return fire{}, false
	}
	f, ok := s.Fire[*st.From]
	return f, ok
}
