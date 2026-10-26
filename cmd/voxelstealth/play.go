package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/platform/glfw"
	"github.com/memmaker/voxellib/projectile"
	"github.com/memmaker/voxellib/voxel/collide"
	"github.com/memmaker/voxelstealth/sim"
)

// Weapons in hand, keys 1-7 (the sword and halberd once taken from a guard).
const (
	wCrossbow = iota
	wBlackjack
	wBlade
	wExplosive
	wTNT
	wSword
	wHalberd
	numWeapons
)

var weaponNames = [numWeapons]string{"Crossbow", "Blackjack", "Blade", "Flash bomb", "TNT", "Long sword", "Halberd"}

// arrowTypes cycle with X.
var arrowTypes = []string{sim.AmmoBroadhead, sim.AmmoWater, sim.AmmoNoise, sim.AmmoRope}

const (
	kickTicks  = 12 // between kicks
	swingTicks = 8
	throwTicks = 10 // a throw's full windup: hard, it bashes what it hits
	crouchEye  = 1.15
	slipTicks  = 20 // on the floor after slipping on oil
)

type weapons struct {
	weapon   int
	ammo     string
	reload   int    // ticks until the crossbow is spanned
	spanning bool   // R: the crossbow is being spanned
	chamber  string // the spanned bolt's arrow type, "" unspanned
	fireWas  bool   // the button was down last tick
	// the hand's height (ItemInHandRenderer): it dips on a use and rises back
	handHeight, prevHandHeight float32
	kickWait                   int // ticks to the next kick
	swing                      int
	windup                     bool // a melee weapon drawn back, the button held
	throwHold                  int  // ticks a throw has been wound up, up to throwTicks
	slipped                    int  // ticks left on the floor after slipping on oil
	placed                     bool // TNT set down this click
	arrows                     projectile.World
	arrowAmmo                  map[*projectile.Arrow]string
	thrown                     map[*projectile.Arrow]int // thrown weapons (throw.go), and which
	lost                       map[int]bool              // weapons thrown and not picked up again
	reported                   map[*projectile.Arrow]bool
	boltBy                     map[*projectile.Arrow]int // guards' bolts (arms.go), by shooter
	blade                      playerBlade
	npcTargets                 []*npcTarget
	useHeld                    int
	target                     sim.Target
	vkeys                      map[glfw.Key]bool // keys held by a -script
	vmouse                     bool
	shotReq                    bool
	ropeState
}

// resetPlay clears per-run glue state (new mission, quickload).
func (g *game) resetPlay() {
	g.weapon, g.ammo = wCrossbow, sim.AmmoBroadhead
	g.arrows = projectile.World{}
	g.arrowAmmo, g.reported = map[*projectile.Arrow]string{}, map[*projectile.Arrow]bool{}
	g.thrown, g.lost = map[*projectile.Arrow]int{}, map[int]bool{}
	g.boltBy, g.torchTips = map[*projectile.Arrow]int{}, map[int]sim.Vec3{}
	g.blade = playerBlade{}
	g.chamber, g.spanning, g.reload = "", false, 0
	g.npcTargets = nil
	g.parts.P, g.handParts.P = g.parts.P[:0], g.handParts.P[:0]
	clear(g.lids)
	for k := range g.sim.NPCs {
		t := &npcTarget{g: g, k: k}
		if n := g.sim.NPCs[k]; !n.Alive() { // already down (quickload)
			t.dying, t.fallYaw = 1, n.Yaw+math.Pi // the ragdoll slumps again
		}
		g.npcTargets = append(g.npcTargets, t)
	}
	g.arrows.Hit = g.arrowHit
	g.npcAnims = nil
	g.ropes, g.onRope, g.mantle = nil, nil, nil
	g.player.EyeHeight = character.PlayerEyeHeight
}

func (g *game) setupInput() {
	g.capture(true)
	g.win.SetKeyCallback(func(w *glfw.Window, k glfw.Key, sc int, a glfw.Action, mods glfw.ModifierKey) {
		if a != glfw.Press {
			return
		}
		g.keyPressed(k, mods)
	})
	g.win.SetMouseButtonCallback(func(w *glfw.Window, b glfw.MouseButton, a glfw.Action, m glfw.ModifierKey) {
		if a == glfw.Press && !g.captured {
			g.capture(true)
			return
		}
		if a == glfw.Press && g.editing && g.sim == nil {
			g.editorClick(b)
		}
	})
	g.win.SetScrollCallback(func(w *glfw.Window, dx, dy float64) {
		if g.editing && g.sim == nil {
			g.editorScroll(dy)
		}
	})
	lastX, lastY := g.win.GetCursorPos()
	g.win.SetCursorPosCallback(func(w *glfw.Window, x, y float64) {
		dx, dy := x-lastX, y-lastY
		lastX, lastY = x, y
		if !g.captured {
			return
		}
		const sens = 0.0025
		g.cam.Yaw += dx * sens
		g.cam.Pitch = math.Max(-1.55, math.Min(1.55, g.cam.Pitch-dy*sens))
	})
}

func (g *game) capture(on bool) {
	g.captured = on
	if on {
		g.win.SetInputMode(glfw.CursorMode, glfw.CursorDisabled)
		if glfw.RawMouseMotionSupported() {
			g.win.SetInputMode(glfw.RawMouseMotion, glfw.True)
		}
	} else {
		g.win.SetInputMode(glfw.CursorMode, glfw.CursorNormal)
	}
}

// keyPressed handles single key presses (held keys are polled in ticks).
func (g *game) keyPressed(k glfw.Key, mods glfw.ModifierKey) {
	switch k {
	case glfw.KeyEscape:
		if g.captured {
			g.capture(false)
		} else {
			g.win.SetShouldClose(true)
		}
		return
	case glfw.KeyF1:
		g.showHelp = !g.showHelp
		return
	case glfw.KeyF2:
		g.shotReq = true
		return
	case glfw.KeyF6:
		if g.editing && g.ready {
			g.togglePlaytest()
		}
		return
	}
	if !g.ready {
		return
	}
	if g.editing && g.sim == nil {
		g.editorKey(k, mods)
		return
	}
	s := g.sim
	switch {
	case k == glfw.KeyTab:
		g.showObjectives = !g.showObjectives
	case k == glfw.KeyF5:
		g.quicksave()
	case k == glfw.KeyF9:
		g.quickload()
	case k == glfw.KeyR && s.Outcome != sim.Running:
		g.restart()
	case k == glfw.KeyR:
		g.startSpan()
	case k == glfw.KeyT:
		g.throwWeapon()
	case k >= glfw.Key1 && k < glfw.Key1+numWeapons:
		g.selectWeapon(int(k - glfw.Key1))
	case k == glfw.KeyX:
		for i, t := range arrowTypes {
			if t == g.ammo {
				g.ammo = arrowTypes[(i+1)%len(arrowTypes)]
				break
			}
		}
		g.weapon = wCrossbow
	case k == glfw.KeyQ && g.kickWait == 0 && s.Player.Carrying < 0:
		s.Kick()
		g.kickWait, g.kicked = kickTicks, true
	case k == glfw.KeyG:
		if s.Player.Carrying >= 0 {
			s.DropBody()
		} else {
			s.ThrowTorch(g.eye(), g.viewDir(), 0, false)
		}
	}
}

// restart begins the mission again.
func (g *game) restart() {
	g.world.revert(nil)
	g.startSim()
	g.log = nil
}

func (g *game) key(k glfw.Key) bool {
	if g.vkeys[k] {
		return true
	}
	return g.captured && g.win.GetKey(k) == glfw.Press
}

func (g *game) mouseDown() bool {
	return g.vmouse || g.captured && g.win.GetMouseButton(glfw.MouseButtonLeft) == glfw.Press
}

// movementInput reads WASD and the look direction.
func (g *game) movementInput() character.Input {
	in := character.Input{Yaw: g.cam.Yaw, Pitch: g.cam.Pitch}
	in.Forward = axis(g.key(glfw.KeyW), g.key(glfw.KeyS))
	in.Strafe = axis(g.key(glfw.KeyD), g.key(glfw.KeyA))
	in.Jump = g.key(glfw.KeySpace)
	in.Sneak = g.key(glfw.KeyLeftShift) || g.key(glfw.KeyC)
	in.Sprint = g.key(glfw.KeyLeftControl)
	return in
}

func axis(pos, neg bool) float64 {
	v := 0.0
	if pos {
		v++
	}
	if neg {
		v--
	}
	return v
}

// tickPlay runs one tick of the mission.
func (g *game) tickPlay() {
	s := g.sim
	p := g.player
	running := s.Outcome == sim.Running && !s.Player.Dead()
	in := character.Input{Yaw: g.cam.Yaw, Pitch: g.cam.Pitch}
	if running {
		in = g.movementInput()
		if g.spanning { // a crawl
			in.Forward, in.Strafe, in.Sprint, in.Jump = in.Forward*spanCrawl, in.Strafe*spanCrawl, false, false
		}
		if g.slipped > 0 { // feet gone: down for a moment
			g.slipped--
			in.Forward, in.Strafe, in.Sprint, in.Jump, in.Sneak = 0, 0, false, false, true
		}
	}
	if g.tickRopes(in) {
		p.FallDistance, p.Landed = 0, 0 // a rope breaks a fall
	} else {
		p.Tick(s.Body(), in)
	}
	g.prevWalk = g.walkDist
	if p.OnGround {
		g.walkDist += math.Hypot(p.X-p.PrevX, p.Z-p.PrevZ)
	}
	eyeWant := character.PlayerEyeHeight
	if in.Sneak {
		eyeWant = crouchEye
	}
	p.EyeHeight += (eyeWant - p.EyeHeight) * 0.35

	pl := &s.Player
	pl.Pos = vec(p.X, p.Y, p.Z)
	pl.Yaw, pl.Pitch = g.cam.Yaw, g.cam.Pitch
	pl.Crouch, pl.Run, pl.OnGround, pl.Landed = in.Sneak, in.Sprint && !in.Sneak, p.OnGround, p.Landed

	if running {
		g.target = s.TargetAt(g.eye(), g.viewDir())
		if g.key(glfw.KeyE) || g.key(glfw.KeyF) {
			s.Frob(g.target, g.useHeld > 0)
			g.useHeld++
		} else {
			g.useHeld = 0
		}
		held := g.mouseDown()
		switch {
		case s.HeldTorch() >= 0 || g.weapon == wExplosive:
			g.windup = false
			g.windThrow(held)
		case g.weapon == wTNT:
			g.windup, g.throwHold = false, 0
			g.place(held)
		case g.lost[g.weapon]: // thrown: the hand is empty
			g.windup, g.throwHold = false, 0
		case blades[g.weapon] != "":
			g.throwHold = 0
			g.bladeStrike(held)
		case g.fp[g.weapon] != nil:
			g.throwHold = 0
			g.strike(held)
		default:
			g.windup, g.throwHold = false, 0
			if held && !g.fireWas {
				g.shoot()
			}
		}
		g.fireWas = held
	}
	g.tickCrossbow()
	if g.swing > 0 {
		g.swing--
	}
	if g.kickWait > 0 {
		g.kickWait--
	}
	for _, t := range g.npcTargets { // NPCs going down or kicked ragdoll, arrows or not
		t.tickFall()
	}
	g.tickArrows()
	s.Tick()
	g.tickArms()
	g.tickBlade()
	g.handleEvents()
	g.tickFX()
	g.eng.Renderer.Opt.Daylight = float32(s.Daylight())
	g.tickSounds()
}

// strike draws the melee weapon back while the button is held and strikes
// on its release; the next windup waits for the strike clip to end.
func (g *game) strike(held bool) {
	arm, a := fpArms[g.weapon], g.fp[g.weapon]
	switch {
	case held && !g.windup && g.swing == 0:
		g.windup = true
		a.Play("windup", true)
	case !held && g.windup:
		g.windup = false
		a.Play(arm.strike, true)
		g.swing = int(math.Ceil(float64(a.Clip.Length) / tickDur.Seconds()))
		g.sim.Melee(arm.melee)
	}
}

// windThrow winds up a throw (the held torch, else a flash bomb) while the
// button is held and throws on its release: the longer the windup the
// farther, and at full windup hard, bashing the first thing it hits.
func (g *game) windThrow(held bool) {
	s := g.sim
	switch {
	case held && g.swing == 0:
		if g.throwHold == 0 && s.HeldTorch() < 0 && s.Player.Ammo[sim.AmmoExplosive] > 0 {
			g.fpHand.Play("windup", true)
		}
		g.throwHold = min(g.throwHold+1, throwTicks)
	case !held && g.throwHold > 0:
		k, hard := float64(g.throwHold)/throwTicks, g.throwHold == throwTicks
		g.throwHold, g.swing = 0, swingTicks
		if s.HeldTorch() >= 0 {
			s.ThrowTorch(g.eye(), g.viewDir(), 6+6*k, hard)
		} else if s.Throw(g.eye(), g.viewDir(), 5+4*k, hard) {
			g.fpHand.Play("throw", true)
		} else {
			g.message("No flash bombs left.")
		}
	}
}

// place sets lit TNT down in front of the player, once per click.
func (g *game) place(held bool) {
	switch {
	case !held:
		g.placed = false
	case !g.placed && g.swing == 0:
		g.placed, g.swing = true, swingTicks
		if g.sim.PlaceTNT(g.eye(), g.viewDir()) {
			g.fpHand.Play("place", true)
		} else {
			g.message("No TNT left.")
		}
	}
}

// tickArrows flies arrows and reports where they land to the simulation.
func (g *game) tickArrows() {
	s := g.sim
	if len(g.arrows.Arrows) == 0 {
		return
	}
	// Water arrows put out torches and street lamps they fly past.
	for _, a := range g.arrows.Arrows {
		if a.Stuck || g.arrowAmmo[a] != sim.AmmoWater {
			continue
		}
		for i, o := range s.Objects {
			street := o.Kind == sim.KindLamp && o.Name == sim.StreetLamp
			if o.Kind != sim.KindTorch && !street || !o.Lit() || o.Gone {
				continue
			}
			lo, hi := s.ObjectBox(i)
			mid := lo.Add(hi).Scale(0.5)
			if street {
				mid = o.Pos.Add(sim.Vec3{Y: sim.StreetLampHead})
			}
			if mid.Dist(vec(a.X, a.Y, a.Z)) < 0.8 {
				s.ArrowHit(sim.AmmoWater, vec(a.X, a.Y, a.Z), -1, false, 0)
				a.Discard()
				break
			}
		}
	}
	targets := make([]projectile.Target, 0, len(g.npcTargets)+1)
	targets = append(targets, playerTarget{g})
	for k, t := range g.npcTargets {
		if n := s.NPCs[k]; !n.Hidden && !n.CarriedBy {
			targets = append(targets, t)
		}
	}
	for i, o := range s.Objects {
		if o.Kind == sim.KindDecor && !o.Gone {
			targets = append(targets, decorTarget{s, i})
		}
	}
	g.arrows.Tick(g.world, targets)
	g.pickUpThrown()
	for _, a := range g.arrows.Arrows {
		if a.Stuck && a.In == nil && !g.reported[a] {
			g.reported[a] = true
			if _, ok := g.thrown[a]; ok {
				g.thrownHit(a, nil)
				continue
			}
			ammo := g.arrowAmmo[a]
			at := vec(a.X, a.Y, a.Z)
			if ammo == sim.AmmoWater { // splash in the air before the face, not in the block
				cp := math.Cos(a.Pitch)
				at = at.Add(vec(-math.Sin(a.Yaw)*cp, -math.Sin(a.Pitch), math.Cos(a.Yaw)*cp).Scale(0.35))
			}
			s.ArrowHit(ammo, at, -1, false, 0)
			if ammo == sim.AmmoRope {
				if s.AddRope(vec(a.X, a.Y, a.Z), a.Yaw, a.Pitch) {
					a.Discard() // drawRopes draws it from now on
				}
				continue
			}
			if c := vec(a.X, a.Y, a.Z).Cell(); ammo != sim.AmmoBroadhead || !g.world.Solid(c.X, c.Y, c.Z) { // or the glass it hit broke
				a.Discard()
			}
		}
	}
	// Arrows in bodies that were hidden away go with them.
	for k, t := range g.npcTargets {
		if s.NPCs[k].Hidden {
			g.arrows.Release(t)
		}
	}
}

func (g *game) arrowHit(a *projectile.Arrow, t projectile.Target) {
	if _, ok := g.thrown[a]; ok {
		nt, _ := t.(*npcTarget)
		g.reported[a] = nt != nil || g.reported[a]
		if nt != nil {
			g.thrownHit(a, nt)
		}
		return
	}
	if _, ok := t.(playerTarget); ok {
		g.reported[a] = true
		if k, ok := g.boltBy[a]; ok {
			g.sim.BoltHit(k, vec(a.X, a.Y, a.Z), true, -1, false, a.Yaw)
		}
		a.Discard()
		return
	}
	if _, ok := t.(decorTarget); ok { // stays stuck, as in a block
		g.reported[a] = true
		g.sim.ArrowHit(g.arrowAmmo[a], vec(a.X, a.Y, a.Z), -1, false, 0)
		if g.arrowAmmo[a] != sim.AmmoBroadhead {
			a.Discard()
		}
		return
	}
	nt, ok := t.(*npcTarget)
	if !ok {
		return
	}
	g.reported[a] = true
	ammo := g.arrowAmmo[a]
	n := g.sim.NPCs[nt.k]
	alive := n.Alive()
	g.sim.ArrowHit(ammo, vec(a.X, a.Y, a.Z), nt.k, alive && nt.isHead(a.Part), a.Yaw)
	if alive {
		g.burst("blood", vec(a.X, a.Y, a.Z))
	}
	if alive && !n.Alive() {
		nt.fall(a.Yaw) // pushed over along the arrow's flight
	}
	if ammo != sim.AmmoBroadhead {
		a.Discard()
	}
}

// scriptStep is one automated input for -shot runs: at tick (counted from
// the mission start) do action. Actions: fwd/back/left/right/crouch/run/
// use/fire=N hold for N ticks; yaw/pitch=degrees; weapon=1..4; tp=x,y,z;
// ammo=type; give=sword|halberd; save; load.
type scriptStep struct {
	at     int
	action string
	arg    string
}

func parseScript(s string) []scriptStep {
	var out []scriptStep
	for _, part := range strings.Split(s, ";") {
		at, rest, ok := strings.Cut(strings.TrimSpace(part), ":")
		if !ok {
			continue
		}
		t, _ := strconv.Atoi(at)
		act, arg, _ := strings.Cut(rest, "=")
		out = append(out, scriptStep{t, act, arg})
	}
	return out
}

var scriptKeys = map[string]glfw.Key{"fwd": glfw.KeyW, "back": glfw.KeyS, "left": glfw.KeyA, "right": glfw.KeyD,
	"crouch": glfw.KeyLeftShift, "run": glfw.KeyLeftControl, "use": glfw.KeyE, "jump": glfw.KeySpace}

type heldKey struct {
	k     glfw.Key
	mouse bool
	until int
}

var scriptHeld []heldKey

func (g *game) runScript() {
	if g.vkeys == nil {
		g.vkeys = map[glfw.Key]bool{}
	}
	keep := scriptHeld[:0]
	for _, h := range scriptHeld {
		if g.playTick >= h.until {
			if h.mouse {
				g.vmouse = false
			} else {
				g.vkeys[h.k] = false
			}
			continue
		}
		keep = append(keep, h)
	}
	scriptHeld = keep
	for _, st := range g.script {
		if st.at != g.playTick {
			continue
		}
		n, _ := strconv.Atoi(st.arg)
		switch st.action {
		case "yaw":
			f, _ := strconv.ParseFloat(st.arg, 64)
			g.cam.Yaw = f * math.Pi / 180
		case "pitch":
			f, _ := strconv.ParseFloat(st.arg, 64)
			g.cam.Pitch = f * math.Pi / 180
		case "weapon":
			g.selectWeapon(n - 1)
		case "ammo":
			g.ammo = st.arg
		case "give": // a taken weapon: sword, halberd
			g.sim.Player.Weapons[st.arg] = true
		case "tp":
			var x, y, z float64
			fmt.Sscanf(st.arg, "%f,%f,%f", &x, &y, &z)
			g.player.Teleport(x, y, z)
		case "fire":
			g.vmouse = true
			scriptHeld = append(scriptHeld, heldKey{mouse: true, until: g.playTick + max(n, 1)})
		case "save":
			g.quicksave()
		case "load":
			g.quickload()
		case "key":
			g.keyPressed(glfw.Key(n), 0)
		default:
			if k, ok := scriptKeys[st.action]; ok {
				g.vkeys[k] = true
				scriptHeld = append(scriptHeld, heldKey{k: k, until: g.playTick + max(n, 1)})
			}
		}
	}
}

// decorTarget lets arrows stick in decoration i.
type decorTarget struct {
	s *sim.Sim
	i int
}

func (d decorTarget) Box() collide.AABB { return d.s.DecorBox(d.i) }

func (d decorTarget) Frame() (x, y, z, yaw float64) {
	o := d.s.Objects[d.i]
	return o.Pos.X, o.Pos.Y, o.Pos.Z, o.Yaw
}
