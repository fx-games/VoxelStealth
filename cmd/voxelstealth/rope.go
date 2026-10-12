package main

// Rope arrows: a Verlet rope hangs from the nock of each sim.Rope. The
// player grabs one in the air and hangs from it as a pendulum mass, so the
// jump's momentum turns into a swing; the rope above the hands is pinned
// to them and the rest dangles.

import (
	"math"

	"github.com/go-gl/mathgl/mgl64"
	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/projectile"
	"github.com/memmaker/voxellib/voxel/collide"
	"github.com/memmaker/voxellib/voxel/render"
	"github.com/memmaker/voxelstealth/sim"
)

const (
	ropeSeg     = 0.25  // segment length, blocks
	ropeGravity = 0.04  // blocks/tick², the rope's own nodes
	ropeHand    = 1.5   // the hands, above the feet
	ropeReach   = 0.45  // how far from the body's axis a rope is caught
	climbSpeed  = 0.08  // blocks/tick
	swingAccel  = 0.006 // blocks/tick², pumping
)

type ropeState struct {
	ropes  []*rope // g.sim.Ropes, simulated
	onRope *rope
	ropeD  float64   // rope from the nock to the hands
	regrab int       // ticks before a rope can be caught again
	mantle *sim.Vec3 // where a mantle in progress ends
}

type rope struct {
	top     sim.Vec3
	p, prev []sim.Vec3 // nodes; p[0] is tied to the nock
}

func newRope(r sim.Rope) *rope {
	n := max(1, int(r.Len/ropeSeg))
	ro := &rope{top: r.Top}
	fx, fz := math.Sin(r.Yaw), -math.Cos(r.Yaw)
	for i := 0; i <= n; i++ {
		p := r.Top.Add(sim.Vec3{Y: -float64(i) * ropeSeg})
		ro.p = append(ro.p, p)
		// Moving a little along the arrow's flight: the impact sets it swaying.
		ro.prev = append(ro.prev, p.Sub(sim.Vec3{X: fx, Z: fz}.Scale(0.004*float64(i))))
	}
	return ro
}

func (r *rope) length() float64 { return float64(len(r.p)-1) * ropeSeg }

// tick steps the rope; node pin (> 0) is held at hand. The player's body
// pushes free nodes aside.
func (r *rope) tick(w collide.Solids, pin int, hand sim.Vec3, body *character.Character) {
	for i := 1; i < len(r.p); i++ {
		v := r.p[i].Sub(r.prev[i]).Scale(0.98)
		r.prev[i] = r.p[i]
		r.p[i] = r.p[i].Add(v).Add(sim.Vec3{Y: -ropeGravity})
	}
	fixed := func(i int) bool { return i == 0 || i == pin }
	if pin > 0 {
		r.p[pin] = hand
	}
	for i := 1; i < len(r.p); i++ {
		n := &r.p[i]
		dx, dz := n.X-body.X, n.Z-body.Z
		if d := math.Hypot(dx, dz); !fixed(i) && d < 0.3 && d > 1e-6 && n.Y > body.Y && n.Y < body.Y+body.Height {
			n.X, n.Z = body.X+dx/d*0.3, body.Z+dz/d*0.3
		}
	}
	for range 15 {
		for i := 0; i+1 < len(r.p); i++ {
			d := r.p[i+1].Sub(r.p[i])
			l := d.Len()
			if l < 1e-9 {
				continue
			}
			c := d.Scale((l - ropeSeg) / l)
			switch fa, fb := fixed(i), fixed(i+1); {
			case fa && fb:
			case fa:
				r.p[i+1] = r.p[i+1].Sub(c)
			case fb:
				r.p[i] = r.p[i].Add(c)
			default:
				r.p[i], r.p[i+1] = r.p[i].Add(c.Scale(0.5)), r.p[i+1].Sub(c.Scale(0.5))
			}
		}
	}
	for i := 1; i < len(r.p); i++ {
		if c := r.p[i].Cell(); !fixed(i) && w.Solid(c.X, c.Y, c.Z) {
			r.p[i] = r.prev[i]
		}
	}
}

// tickRopes steps the ropes and, while the player hangs on one, the
// player; false leaves the player to the character controller.
func (g *game) tickRopes(in character.Input) bool {
	s, p := g.sim, g.player
	for len(g.ropes) < len(s.Ropes) {
		g.ropes = append(g.ropes, newRope(s.Ropes[len(g.ropes)]))
	}
	if g.regrab > 0 {
		g.regrab--
	}
	if g.mantle == nil && in.Forward > 0 && !p.OnGround && !p.Flying && (in.Jump || g.onRope != nil) {
		if to, ok := ledge(g.world, p, in.Yaw); ok {
			g.mantle, g.onRope = &to, nil
		}
	}
	if g.onRope == nil && g.mantle == nil && g.regrab == 0 && !p.OnGround && !p.Flying {
		g.grabRope()
	}
	handled := g.mantle != nil || g.onRope != nil && g.hang(in)
	if g.mantle != nil && mantleStep(g.world, p, *g.mantle) {
		g.mantle = nil
	}
	sy, cy := math.Sin(in.Yaw), math.Cos(in.Yaw)
	// Drawn in the right hand, ahead and below the eye, not through it.
	hand := vec(p.X+sy*0.3+cy*0.25, p.Y+1.3, p.Z-cy*0.3+sy*0.25)
	for _, r := range g.ropes {
		pin := 0
		if r == g.onRope {
			pin = min(max(1, int(math.Round(g.ropeD/ropeSeg))), len(r.p)-1)
		}
		r.tick(g.world, pin, hand, p)
	}
	return handled
}

// grabRope catches a rope passing between the player's waist and head.
func (g *game) grabRope() {
	p := g.player
	for _, r := range g.ropes {
		for _, n := range r.p[2:] {
			if math.Hypot(n.X-p.X, n.Z-p.Z) < ropeReach && n.Y > p.Y+1 && n.Y < p.Y+p.Height {
				g.onRope = r
				g.ropeD = min(vec(p.X, p.Y+ropeHand, p.Z).Sub(r.top).Len(), r.length())
				return
			}
		}
	}
}

func (g *game) letGo() { g.onRope, g.regrab = nil, 10 }

// hang moves the player hanging on the rope: a pendulum whose hands stay
// within ropeD of the nock. W/S climb (with Ctrl: pump), A/D pump, Space
// jumps off. False when the player let go before moving.
func (g *game) hang(in character.Input) bool {
	p, r := g.player, g.onRope
	sy, cy := math.Sin(in.Yaw), math.Cos(in.Yaw)
	if in.Jump {
		g.letGo()
		p.VX, p.VY, p.VZ = p.VX+sy*0.1, max(p.VY, 0)+0.3, p.VZ-cy*0.1
		return false
	}
	v := vec(p.VX, p.VY, p.VZ)
	if in.Sprint {
		v = v.Add(vec(sy, 0, -cy).Scale(in.Forward * swingAccel))
	} else {
		g.ropeD -= in.Forward * climbSpeed
	}
	v = v.Add(vec(cy, 0, sy).Scale(in.Strafe * swingAccel))
	if g.ropeD > r.length() { // slid off the end
		g.letGo()
		return false
	}
	g.ropeD = max(g.ropeD, 0.6)
	p.VX, p.VY, p.VZ = v.X, v.Y, v.Z
	if pendulum(g.world, p, r.top, g.ropeD) { // feet on the floor: stand
		g.letGo()
	}
	return true
}

// pendulum moves p one tick under gravity with its hands held within d of
// top, keeping the velocity the rope lets through; true when it landed.
func pendulum(w collide.Solids, p *character.Character, top sim.Vec3, d float64) bool {
	v := vec(p.VX, p.VY-0.08, p.VZ).Scale(0.99)
	p.PrevX, p.PrevY, p.PrevZ = p.X, p.Y, p.Z
	hand := vec(p.X, p.Y+ropeHand, p.Z)
	want := hand.Add(v)
	if r := want.Sub(top); r.Len() > d {
		want = top.Add(r.Scale(d / r.Len()))
	}
	dv := want.Sub(hand)
	mx, my, mz := collide.Move(w, p.Box(), dv.X, dv.Y, dv.Z)
	p.X, p.Y, p.Z = p.X+mx, p.Y+my, p.Z+mz
	p.VX, p.VY, p.VZ = mx, my, mz
	p.OnGround = dv.Y < 0 && my > dv.Y
	return p.OnGround
}

// drawRopes draws the rope arrows and their ropes, one model per segment.
func (g *game) drawRopes(alpha float64) {
	for i, r := range g.ropes {
		d := g.sim.Ropes[i]
		sky, blk := g.light(d.Top.X, d.Top.Y, d.Top.Z)
		g.insts = append(g.insts, render.ModelInstance{Model: g.props["arrow_rope"], X: d.Tip.X, Y: d.Tip.Y, Z: d.Tip.Z,
			Yaw: d.Yaw, Pitch: d.Pitch, Sky: sky, Block: blk})
		at := func(k int) sim.Vec3 { return r.prev[k].Add(r.p[k].Sub(r.prev[k]).Scale(alpha)) }
		for k := 0; k+1 < len(r.p); k++ {
			a, b := at(k), at(k+1)
			dir := b.Sub(a)
			yaw, pitch := projectile.Orient(mgl64.Vec3{dir.X, dir.Y, dir.Z})
			sky, blk := g.light(a.X, a.Y, a.Z)
			g.insts = append(g.insts, render.ModelInstance{Model: g.props["rope"], X: a.X, Y: a.Y, Z: a.Z,
				Yaw: yaw, Pitch: pitch, Sky: sky, Block: blk})
		}
	}
}

// Mantling: airborne with Space+W held, or climbing a rope with W, the
// player pulls up onto a ledge within arm's reach ahead.
const (
	mantleReach = 2.2  // highest ledge, above the feet
	mantleAhead = 0.5  // how far ahead the ledge is looked for
	mantleSpeed = 0.12 // blocks/tick
)

// ledge finds the spot ahead (along yaw) a mantle ends on: a box dropped
// from arm's reach that lands above step height.
func ledge(w collide.Solids, p *character.Character, yaw float64) (sim.Vec3, bool) {
	fx, fz := math.Sin(yaw)*mantleAhead, -math.Cos(yaw)*mantleAhead
	box := p.Box().Offset(fx, mantleReach, fz)
	drop := mantleReach - 0.6 // above stepHeight
	if !collide.Fits(w, box) {
		return sim.Vec3{}, false
	}
	_, dy, _ := collide.Move(w, box, 0, -drop, 0)
	if dy <= -drop {
		return sim.Vec3{}, false
	}
	top := p.Y + mantleReach + dy
	if !collide.Fits(w, collide.Box(p.X, top, p.Z, p.Width, p.Height)) { // room to rise
		return sim.Vec3{}, false
	}
	return vec(p.X+fx, top, p.Z+fz), true
}

// mantleStep moves p up to the ledge, then onto it; true when done.
func mantleStep(w collide.Solids, p *character.Character, to sim.Vec3) bool {
	p.PrevX, p.PrevY, p.PrevZ = p.X, p.Y, p.Z
	p.VX, p.VY, p.VZ = 0, 0, 0
	var d sim.Vec3
	if p.Y < to.Y {
		d.Y = min(mantleSpeed, to.Y-p.Y)
	} else {
		d = vec(to.X-p.X, 0, to.Z-p.Z)
		if l := d.Len(); l > mantleSpeed {
			d = d.Scale(mantleSpeed / l)
		}
	}
	mx, my, mz := collide.Move(w, p.Box(), d.X, d.Y, d.Z)
	p.X, p.Y, p.Z = p.X+mx, p.Y+my, p.Z+mz
	moved := math.Abs(mx)+math.Abs(my)+math.Abs(mz) > 1e-6
	done := !moved || (p.Y >= to.Y && math.Hypot(to.X-p.X, to.Z-p.Z) < 1e-6)
	p.OnGround = done
	return done
}
