package sim

import (
	"strings"

	"github.com/memmaker/voxellib/voxel/fall"
)

// Collapsing structures, Dark Messiah style: a weak wall or a balcony held
// up by wooden supports. Every blow to a support strains it (the strain
// receptron), fire eats it like any wood; when the last one gives, the
// structure drops as one piece and crushes whatever it lands on (crush),
// then lies where it fell.

// Structure is a mission's collapsible structure: the blocks in Box, held
// up by the support blocks at Supports.
type Structure struct {
	ID, Name string
	Supports []Cell
	Box      Box
}

// Falling is a structure on its way down.
type Falling = fall.Piece[string]

// supportStrength is the strain a support takes before its post breaks:
// two kicks, three arrows or blackjack blows.
const supportStrength = 3

// StimCrush is a falling structure striking what is under it.
const StimCrush Stim = "crush"

// supportOf is the structure a support cell holds up (-1: none).
func (s *Sim) supportOf(c Cell) int {
	for i, st := range s.Mission.Structures {
		for _, sc := range st.Supports {
			if sc == c && !s.Collapsed[st.ID] && !s.empty(c) {
				return i
			}
		}
	}
	return -1
}

// Strain is how near the support at c is to breaking, 0..1.
func (s *Sim) Strain(c Cell) float64 {
	return min(1, s.B.Float(cellR(c), "strain")/supportStrength)
}

// strain is a support's impact and blast receptron: kicks, blows and
// arrows strain it, a slam by how hard, a blast breaks it outright.
func strain(s *Sim, d Delivery, _ []string) bool {
	c := d.To.Addr
	by := map[Stim]float64{StimKick: 1.5, StimBash: 1, StimPierce: 1, StimBlast: supportStrength}[d.St.Stim]
	if d.St.Stim == StimSlam {
		by = d.Intensity / 8
	}
	if by <= 0 {
		return false
	}
	if s.B.Add(cellR(c), "strain", by); s.B.Float(cellR(c), "strain") < supportStrength {
		s.emit(c.Center(), 30, "creak", -1)
		return true
	}
	s.W.SetBlock(c.X, c.Y, c.Z, BlockAir)
	s.B.Clear(cellR(c), "strain")
	s.emit(c.Center(), 50, "crash", -1)
	return supportBroken(s, c, d)
}

// supportBroken: a support is gone (strained or burnt away), and with it
// the rest of its post (the support cells above and below it). When it
// was a structure's last post, the structure comes down.
func supportBroken(s *Sim, c Cell, _ Delivery) bool {
	s.reroom = true
	for _, st := range s.Mission.Structures {
		if s.Collapsed[st.ID] || !containsCell(st.Supports, c) {
			continue
		}
		for _, dy := range []int{1, -1} {
			for p := c.Add(0, dy, 0); containsCell(st.Supports, p) && !s.empty(p); p = p.Add(0, dy, 0) {
				s.W.SetBlock(p.X, p.Y, p.Z, BlockAir)
				s.B.Clear(cellR(p), "strain")
			}
		}
		standing := false
		for _, sc := range st.Supports {
			standing = standing || !s.empty(sc)
		}
		if !standing {
			s.collapse(st)
		}
	}
	return true
}

func containsCell(cs []Cell, c Cell) bool {
	for _, x := range cs {
		if x == c {
			return true
		}
	}
	return false
}

// collapse drops structure st.
func (s *Sim) collapse(st *Structure) {
	s.Collapsed[st.ID] = true
	var cells [][3]int
	for x := st.Box.Min.X; x <= st.Box.Max.X; x++ {
		for y := st.Box.Min.Y; y <= st.Box.Max.Y; y++ {
			for z := st.Box.Min.Z; z <= st.Box.Max.Z; z++ {
				if b := s.W.Block(x, y, z); b != BlockLight && !strings.HasPrefix(b, BlockDoor) {
					cells = append(cells, [3]int{x, y, z})
				}
			}
		}
	}
	p := fall.Detach[string](fallWorld{s}, cells, BlockAir)
	if len(p.Cells) == 0 {
		return
	}
	s.Falls = append(s.Falls, p)
	s.reroom = true
	s.emit(st.Box.Min.Floor().Add(st.Box.Max.Floor()).Scale(0.5), 70, "crash", -1)
}

// tickFalls lets falling structures fall; each block that passes through
// someone strikes them (crush) and when one lands it lies where it fell,
// short of the cells where bodies lie.
func (s *Sim) tickFalls() {
	live := s.Falls[:0]
	for _, p := range s.Falls {
		landed := p.Tick(fallWorld{s})
		for i := range p.Cells {
			x, y, z := p.At(i)
			lo := Vec3{x, y, z}
			st := Stimulus{Stim: StimCrush, Medium: "radius", Origin: lo.Add(Vec3{0.5, 0.5, 0.5}), Range: 0.5, Intensity: 1}
			s.Stimulate(st)
		}
		if !landed {
			live = append(live, p)
			continue
		}
		p.Settle(fallWorld{s}, BlockAir, func(x, y, z int) bool { return !s.bodyIn(Cell{x, y, z}) })
		s.emit(s.fallCentre(p), 90, "crash", -1)
		s.event(Event{Kind: EvBlast, Pos: s.fallCentre(p)})
		s.reroom = true
	}
	s.Falls = live
}

// crushed reports whether a falling block's crush came down on what it
// reached (not on whoever rides the structure down).
func crushed(s *Sim, d Delivery) bool {
	return d.St.Origin.Y > s.B.World.Locate(d.To).Y+0.6
}

// bodyIn reports whether the player or an NPC's body is in cell c.
func (s *Sim) bodyIn(c Cell) bool {
	if p := s.Player.Pos; !s.Player.Dead() && (p.Cell() == c || p.Add(Vec3{Y: 1}).Cell() == c) {
		return true
	}
	for _, n := range s.NPCs {
		if !n.Hidden && !n.CarriedBy && (n.pos().Cell() == c || n.pos().Add(Vec3{Y: 1}).Cell() == c) {
			return true
		}
	}
	return false
}

func (s *Sim) fallCentre(p *Falling) Vec3 {
	var c Vec3
	for i := range p.Cells {
		x, y, z := p.At(i)
		c = c.Add(Vec3{x, y, z})
	}
	return c.Scale(1 / float64(len(p.Cells)))
}

// fallWorld is the sim's world as a structure falls through it.
type fallWorld struct{ s *Sim }

func (w fallWorld) Block(x, y, z int) string       { return w.s.W.Block(x, y, z) }
func (w fallWorld) SetBlock(x, y, z int, b string) { w.s.W.SetBlock(x, y, z, b) }
func (w fallWorld) Solid(x, y, z int) bool         { return w.s.W.Solid(x, y, z) }
