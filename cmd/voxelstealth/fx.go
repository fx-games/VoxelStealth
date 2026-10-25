package main

// Particle effects (VoxelLib particles): bursts on sim events, plus small
// continuous emitters for flames, smoke and live puddles. Presets follow the
// shape of go-fps's particle presets, retuned to block units.

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/particles"
	"github.com/memmaker/voxelstealth/sim"
)

var fx = map[string]particles.Props{
	// Glass shards: pale blue, fall and skitter off the floor.
	"shatter": {Count: 40, Spread: 0.4, VelVar: 2.5, Radial: 1.5, Gravity: 22, Drag: 0.3, Bounce: 0.35,
		Life: 1.6, LifeVar: 0.5, Size0: 0.07, Size1: 0.05, SizeVar: 0.03,
		Col0: [4]float32{0.75, 0.88, 0.95, 0.85}, Col1: [4]float32{0.75, 0.88, 0.95, 0.5}},
	// A flash bomb: a white burst, gone in a blink.
	"flash": {Count: 50, Spread: 0.2, Radial: 9, Drag: 4, Life: 0.25, LifeVar: 0.1,
		Size0: 0.6, Size1: 0.1, SizeVar: 0.2, Col0: [4]float32{1, 1, 1, 1}, Col1: [4]float32{0.8, 0.9, 1, 0}, Additive: true},
	"explosion": {Count: 70, Spread: 0.3, Radial: 7, Drag: 3, Life: 0.45, LifeVar: 0.15,
		Size0: 0.5, Size1: 0.15, SizeVar: 0.15, Col0: [4]float32{1, 0.75, 0.3, 1}, Col1: [4]float32{0.8, 0.15, 0, 0}, Additive: true},
	"smoke": {Count: 25, Spread: 0.6, Vel: [3]float64{0, 1, 0}, VelVar: 0.6, Gravity: -0.6, Drag: 1, Life: 2.5, LifeVar: 0.8,
		Size0: 0.4, Size1: 1.2, SizeVar: 0.2, Col0: [4]float32{0.25, 0.24, 0.22, 0.7}, Col1: [4]float32{0.3, 0.3, 0.3, 0}},
	"splash": {Count: 25, Spread: 0.25, Vel: [3]float64{0, 3, 0}, VelVar: 1.6, Gravity: 20, Life: 0.6, LifeVar: 0.2,
		Size0: 0.08, Size1: 0.05, Col0: [4]float32{0.45, 0.6, 0.85, 0.8}, Col1: [4]float32{0.45, 0.6, 0.85, 0.2}},
	"sparks": {Count: 12, Spread: 0.3, Vel: [3]float64{0, 2, 0}, VelVar: 3, Gravity: 18, Bounce: 0.5, Life: 0.35, LifeVar: 0.15,
		Size0: 0.05, Size1: 0.02, Col0: [4]float32{0.7, 0.85, 1, 1}, Col1: [4]float32{0.3, 0.5, 1, 0}, Additive: true},
	"ignite": {Count: 25, Spread: 0.3, Vel: [3]float64{0, 1.5, 0}, VelVar: 1, Gravity: -2, Drag: 1, Life: 0.6, LifeVar: 0.2,
		Size0: 0.25, Size1: 0.05, Col0: [4]float32{1, 0.7, 0.2, 1}, Col1: [4]float32{0.9, 0.2, 0, 0}, Additive: true},
	// Stone dust where a bolt hits a wall.
	"dust": {Count: 10, Spread: 0.05, VelVar: 1.2, Gravity: 4, Drag: 2, Life: 0.7, LifeVar: 0.2,
		Size0: 0.06, Size1: 0.18, Col0: [4]float32{0.55, 0.52, 0.48, 0.8}, Col1: [4]float32{0.55, 0.52, 0.48, 0}},
	"steam": {Count: 20, Spread: 0.3, Vel: [3]float64{0, 1.2, 0}, VelVar: 0.4, Gravity: -0.5, Drag: 1, Life: 1.5, LifeVar: 0.4,
		Size0: 0.2, Size1: 0.8, Col0: [4]float32{0.85, 0.85, 0.85, 0.5}, Col1: [4]float32{0.9, 0.9, 0.9, 0}},
	"blood": {Count: 30, Spread: 0.12, Vel: [3]float64{0, 1, 0}, VelVar: 2.2, Gravity: 18, Life: 0.9, LifeVar: 0.25,
		Size0: 0.09, Size1: 0.05, SizeVar: 0.03, Col0: [4]float32{0.65, 0.03, 0.03, 1}, Col1: [4]float32{0.4, 0, 0, 0.85}},
	// Continuous, emitted a few per tick.
	"wick": {Count: 1, Spread: 0.01, Vel: [3]float64{0, 0.3, 0}, VelVar: 0.05, Gravity: -0.5, Drag: 1, Life: 0.3, LifeVar: 0.08,
		Size0: 0.05, Size1: 0.01, Col0: [4]float32{1, 0.75, 0.3, 0.9}, Col1: [4]float32{0.9, 0.3, 0, 0}, Additive: true},
	"flame": {Count: 2, Spread: 0.06, Vel: [3]float64{0, 0.8, 0}, VelVar: 0.2, Gravity: -1, Drag: 1, Life: 0.4, LifeVar: 0.1,
		Size0: 0.16, Size1: 0.03, Col0: [4]float32{1, 0.65, 0.2, 0.9}, Col1: [4]float32{0.9, 0.2, 0, 0}, Additive: true},
	"glow": {Count: 1, Spread: 0.06, Life: 0.4, LifeVar: 0.1,
		Size0: 0.35, Size1: 0.5, Col0: [4]float32{0.75, 0.85, 1, 0.12}, Col1: [4]float32{0.6, 0.75, 1, 0}, Additive: true},
	"wisp": {Count: 1, Spread: 0.2, Vel: [3]float64{0, 0.8, 0}, VelVar: 0.2, Gravity: -0.4, Drag: 0.5, Life: 2, LifeVar: 0.5,
		Size0: 0.25, Size1: 0.7, Col0: [4]float32{0.2, 0.2, 0.2, 0.5}, Col1: [4]float32{0.3, 0.3, 0.3, 0}},
}

// soundFX maps sim sound kinds to the bursts they leave.
var soundFX = map[string][]string{
	"shatter":   {"shatter"},
	"explosion": {"explosion", "smoke"},
	"splash":    {"splash"},
	"zap":       {"sparks"},
	"ignite":    {"ignite"},
	"arrow":     {"dust"},
	"hiss":      {"steam"},
	"flash":     {"flash", "smoke"},
	"stab":      {"blood"},
}

func (g *game) burst(name string, p sim.Vec3) { g.parts.Emit(p.X, p.Y, p.Z, fx[name]) }

// tickFX advances the particles and feeds the continuous emitters near the
// player.
func (g *game) tickFX() {
	s := g.sim
	eye := g.eye()
	for _, o := range s.Objects {
		if o.Gone || o.Pos.Dist(eye) > 24 {
			continue
		}
		if o.Kind == sim.KindChest { // the lid swings over a third of a second
			t := g.lids[o.ID]
			if o.Open() {
				t = min(1, t+0.15)
			} else {
				t = max(0, t-0.15)
			}
			g.lids[o.ID] = t
		}
		switch {
		case o.Burning():
			g.burst("flame", o.Pos.Add(sim.Vec3{Y: 0.3}))
			g.burst("flame", o.Pos.Add(sim.Vec3{Y: 0.5}))
			if s.Now%3 == 0 {
				g.burst("wisp", o.Pos.Add(sim.Vec3{Y: 1}))
			}
		case o.Kind == sim.KindLamp && o.Lit() && !o.Fallen() && lampModel(o) == "lamp_electric":
			g.burst("glow", o.Pos.Add(sim.Vec3{Y: 0.25})) // around the panes
		case o.Kind == sim.KindTorch && o.Lit() && !o.Held() && !o.Fallen() && ownModel[o.Name] == "":
			g.burst("flame", g.torchTip(o))
		case o.Kind == sim.KindTorch && o.Lit() && !o.Held() && !o.Fallen():
			// Candles: a spark or two over each wick, turned with the model.
			sin, cos := math.Sincos(math.Pi - o.Yaw)
			for i, w := range g.wicks[torchModel(o)] {
				if (s.Now+i)%3 == 0 {
					x, z := float64(w[0]), float64(w[2])
					g.burst("wick", o.Pos.Add(sim.Vec3{X: x*cos + z*sin, Y: float64(w[1]), Z: -x*sin + z*cos}))
				}
			}
		case o.Kind == sim.KindTorch && o.Lit() && o.Fallen() && ownModel[o.Name] == "":
			_, _, tip := lyingTorch(o, 1)
			g.burst("flame", tip)
		}
	}
	// Lit fuses: a spark on each bomb's wick once it leaves the hand.
	for _, b := range s.Bombs {
		_, _, _, wick := bombPose(b.Pos, b.Prev, b.Vel, b.Flight, b.Flying, b.Flash, 1)
		g.burst("wick", wick)
	}
	// The held torch: its tip in view space (see drawHand) mapped through
	// the camera basis; the hand's walk bob is ignored.
	if i := s.HeldTorch(); i >= 0 && s.Objects[i].Lit() && ownModel[s.Objects[i].Name] == "" {
		fwd := g.cam.Forward()
		right := fwd.Cross(mgl32.Vec3{0, 1, 0}).Normalize()
		up := right.Cross(fwd)
		p := right.Mul(-0.35).Add(up.Mul(-0.14)).Add(fwd.Mul(0.73))
		small := fx["flame"] // so close to the eye, half size
		small.Size0, small.Size1, small.Vel = 0.08, 0.02, [3]float64{0, 0.4, 0}
		g.handParts.Emit(eye.X+float64(p[0]), eye.Y+float64(p[1]), eye.Z+float64(p[2]), small)
	}
	g.handParts.Tick(tickDur.Seconds(), nil)
	for _, c := range s.LiquidCells(sim.StimWater) {
		if s.Live(c) && s.Now%2 == 0 && c.Floor().Dist(eye) < 24 {
			g.parts.Emit(float64(c.X)+0.5, float64(c.Y)+0.05, float64(c.Z)+0.5, particles.Props{Count: 1, Spread: 0.4,
				Vel: [3]float64{0, 1.5, 0}, VelVar: 1.5, Gravity: 18, Life: 0.2, Size0: 0.04, Size1: 0.02,
				Col0: fx["sparks"].Col0, Col1: fx["sparks"].Col1, Additive: true})
		}
	}
	for _, c := range s.LiquidCells(sim.StimOil) {
		if f := c.Floor(); s.OilBurning(c) && f.Dist(eye) < 24 {
			g.burst("flame", f.Add(sim.Vec3{Y: 0.15}))
			if s.Now%4 == 0 {
				g.burst("wisp", f.Add(sim.Vec3{Y: 0.8}))
			}
		}
	}
	for _, c := range s.FireCells() {
		if f := c.Floor(); f.Dist(eye) < 24 {
			g.burst("flame", f.Add(sim.Vec3{Y: 0.2}))
			g.burst("flame", f.Add(sim.Vec3{Y: 0.5}))
			if s.Now%3 == 0 {
				g.burst("wisp", f.Add(sim.Vec3{Y: 1}))
			}
		}
	}
	g.parts.Tick(tickDur.Seconds(), g.world.Solid)
}
