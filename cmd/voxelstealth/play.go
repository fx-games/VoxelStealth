package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/projectile"
	"github.com/memmaker/voxellib/voxel/collide"
	"github.com/memmaker/voxelstealth/sim"
)

// Weapons in hand, keys 1-4.
const (
	wCrossbow = iota
	wBlackjack
	wBlade
	wExplosive
	numWeapons
)

var weaponNames = [numWeapons]string{"Crossbow", "Blackjack", "Blade", "Flash bomb"}

// arrowTypes cycle with X.
var arrowTypes = []string{sim.AmmoBroadhead, sim.AmmoWater, sim.AmmoNoise}

const (
	reloadTicks = 24 // a hand-spanned crossbow, not the explorer's repeater
	recoilTicks = 5
	swingTicks  = 8
	crouchEye   = 1.15
)

type weapons struct {
	weapon     int
	ammo       string
	reload     int
	recoil     int
	swing      int
	arrows     projectile.World
	arrowAmmo  map[*projectile.Arrow]string
	reported   map[*projectile.Arrow]bool
	npcTargets []*npcTarget
	useHeld    int
	target     sim.Target
	mouseWas   bool
	vkeys      map[glfw.Key]bool // keys held by a -script
	vmouse     bool
	shotReq    bool
}

// npcTarget lets arrows hit an NPC's body.
type npcTarget struct {
	g *game
	k int
}

func (t *npcTarget) Box() collide.AABB {
	n := t.g.sim.NPCs[t.k]
	if !n.Alive() {
		p := n.Pos()
		return collide.AABB{MinX: p.X - 0.8, MinY: p.Y, MinZ: p.Z - 0.8, MaxX: p.X + 0.8, MaxY: p.Y + 0.45, MaxZ: p.Z + 0.8}
	}
	return n.Body.Box()
}

func (t *npcTarget) Frame() (x, y, z, yaw float64) {
	n := t.g.sim.NPCs[t.k]
	return n.Body.X, n.Body.Y, n.Body.Z, n.Yaw
}

// resetPlay clears per-run glue state (new mission, quickload).
func (g *game) resetPlay() {
	g.weapon, g.ammo = wCrossbow, sim.AmmoBroadhead
	g.arrows = projectile.World{}
	g.arrowAmmo, g.reported = map[*projectile.Arrow]string{}, map[*projectile.Arrow]bool{}
	g.npcTargets = nil
	for k := range g.sim.NPCs {
		g.npcTargets = append(g.npcTargets, &npcTarget{g, k})
	}
	g.arrows.Hit = g.arrowHit
	g.npcAnims = nil
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
	case k >= glfw.Key1 && k < glfw.Key1+numWeapons:
		g.weapon = int(k - glfw.Key1)
	case k == glfw.KeyX:
		for i, t := range arrowTypes {
			if t == g.ammo {
				g.ammo = arrowTypes[(i+1)%len(arrowTypes)]
				break
			}
		}
		g.weapon = wCrossbow
	case k == glfw.KeyG:
		if s.Player.Carrying >= 0 {
			s.DropBody()
		} else {
			s.ThrowTorch(g.eye(), g.viewDir(), 0)
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
	running := s.Outcome == sim.Running && !s.Player.Dead
	in := character.Input{Yaw: g.cam.Yaw, Pitch: g.cam.Pitch}
	if running {
		in = g.movementInput()
	}
	p.Tick(g.world, in)
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
	pl.Crouch, pl.Run, pl.OnGround = in.Sneak, in.Sprint && !in.Sneak, p.OnGround

	if running {
		g.target = s.TargetAt(g.eye(), g.viewDir())
		if g.key(glfw.KeyE) || g.key(glfw.KeyF) {
			s.Frob(g.target, g.useHeld > 0)
			g.useHeld++
		} else {
			g.useHeld = 0
		}
		down := g.mouseDown()
		if down && (!g.mouseWas || g.weapon == wCrossbow) {
			g.useWeapon()
		}
		g.mouseWas = down
	}
	if g.reload > 0 {
		g.reload--
	}
	if g.recoil > 0 {
		g.recoil--
	}
	if g.swing > 0 {
		g.swing--
	}
	g.tickArrows()
	s.Tick()
	g.handleEvents()
	g.eng.Renderer.Opt.Daylight = float32(s.Daylight())
	g.tickSounds()
}

// useWeapon fires, swings or throws what is in hand.
func (g *game) useWeapon() {
	s := g.sim
	if s.HeldTorch() >= 0 {
		if g.swing == 0 {
			s.ThrowTorch(g.eye(), g.viewDir(), 9)
			g.swing = swingTicks
		}
		return
	}
	switch g.weapon {
	case wCrossbow:
		if g.reload > 0 {
			return
		}
		if s.Player.Ammo[g.ammo] <= 0 {
			g.reload = reloadTicks
			g.message("No " + g.ammo + " arrows.")
			return
		}
		s.Player.Ammo[g.ammo]--
		e := g.eye()
		a := g.arrows.Shoot(e.X, e.Y-0.1, e.Z, g.cam.Yaw, g.cam.Pitch, projectile.Speed)
		g.arrowAmmo[a] = g.ammo
		g.reload, g.recoil = reloadTicks, recoilTicks
		g.playLocal("twang", 0.5)
	case wBlackjack, wBlade:
		if g.swing > 0 {
			return
		}
		g.swing = swingTicks
		if g.weapon == wBlackjack {
			s.Melee("blackjack")
		} else {
			s.Melee("blade")
		}
	case wExplosive:
		if !s.Throw(g.eye(), g.viewDir()) {
			g.message("No flash bombs left.")
		}
		g.swing = swingTicks
	}
}

// tickArrows flies arrows and reports where they land to the simulation.
func (g *game) tickArrows() {
	s := g.sim
	if len(g.arrows.Arrows) == 0 {
		return
	}
	// Water arrows put out torches they fly past.
	for _, a := range g.arrows.Arrows {
		if a.Stuck || g.arrowAmmo[a] != sim.AmmoWater {
			continue
		}
		for i, o := range s.Objects {
			if o.Kind != sim.KindTorch || !o.Lit || o.Gone {
				continue
			}
			lo, hi := s.ObjectBox(i)
			mid := lo.Add(hi).Scale(0.5)
			if mid.Dist(vec(a.X, a.Y, a.Z)) < 0.8 {
				s.ArrowHit(sim.AmmoWater, vec(a.X, a.Y, a.Z), -1, false)
				a.Discard()
				break
			}
		}
	}
	targets := make([]projectile.Target, 0, len(g.npcTargets))
	for k, t := range g.npcTargets {
		if n := s.NPCs[k]; !n.Hidden && !n.CarriedBy {
			targets = append(targets, t)
		}
	}
	g.arrows.Tick(g.world, targets)
	for _, a := range g.arrows.Arrows {
		if a.Stuck && a.In == nil && !g.reported[a] {
			g.reported[a] = true
			ammo := g.arrowAmmo[a]
			s.ArrowHit(ammo, vec(a.X, a.Y, a.Z), -1, false)
			if ammo != sim.AmmoBroadhead {
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
	nt, ok := t.(*npcTarget)
	if !ok {
		return
	}
	g.reported[a] = true
	ammo := g.arrowAmmo[a]
	head := a.Height(t) > 1.45 && g.sim.NPCs[nt.k].Alive()
	g.sim.ArrowHit(ammo, vec(a.X, a.Y, a.Z), nt.k, head)
	if ammo != sim.AmmoBroadhead {
		a.Discard()
	}
}

// scriptStep is one automated input for -shot runs: at tick (counted from
// the mission start) do action. Actions: fwd/back/left/right/crouch/run/
// use/fire=N hold for N ticks; yaw/pitch=degrees; weapon=1..4; tp=x,y,z;
// ammo=type; save; load.
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
			g.weapon = n - 1
		case "ammo":
			g.ammo = st.arg
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
