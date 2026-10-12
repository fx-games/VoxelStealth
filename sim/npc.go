package sim

import (
	"math"
	"slices"

	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/voxel/collide"
	"github.com/memmaker/voxellib/voxel/nav"
)

// NPC awareness follows Thief: suspicion builds continuously from what an
// NPC sees and hears, and crossing thresholds changes what it does. The
// state list is adapted from TerminalAssassin's AI state stack (see
// docs/references.md); the rates are tuned for light and sound instead of
// tiles and teams.

// NPCState is what an NPC is doing.
type NPCState int

const (
	StatePatrol      NPCState = iota // following its route
	StateSuspicious                  // stopped, looking toward a stimulus
	StateInvestigate                 // walking to a stimulus
	StateSearch                      // looking around a last known position
	StateCombat                      // fighting the player (guards)
	StateFlee                        // running to a guard to report (civilians)
	StateCower                       // hiding in place (civilians)
	StateUnconscious
	StateDead
)

var stateNames = [...]string{"patrol", "suspicious", "investigate", "search", "combat", "flee", "cower", "unconscious", "dead"}

func (s NPCState) String() string { return stateNames[s] }

// Suspicion thresholds.
const (
	SuspiciousAt  = 25
	InvestigateAt = 55
	AlertAt       = 100
)

// Waypoint is a patrol stop.
type Waypoint struct {
	Pos  Vec3
	Wait int     // ticks to stand here
	Yaw  float64 // facing while waiting (if Face)
	Face bool
}

// NPCDef is an NPC as placed in the mission.
type NPCDef struct {
	ID, Name, Kind, Model string // Kind: guard | civilian | target
	Voice                 string // speaker id: assets/voice/<Voice>/
	Pos                   Vec3
	Yaw                   float64
	Patrol                []Waypoint
	Keys                  []string
}

// NPC is a running NPC.
type NPC struct {
	ID, Name, Kind, Model string
	Voice                 string
	Body                  character.Character
	Yaw                   float64
	State                 NPCState
	StateT                int
	Health                int
	Suspicion             float64
	Alertness             float64 // grows after each alert: they see and hear better
	Patrol                []Waypoint
	PatrolI               int
	Wait                  int
	Path                  []Cell
	PathI                 int
	Goal                  Vec3
	Repath, Stuck         int
	LKP                   Vec3 // last known position of whatever worries it
	LastSeen              int  // ticks since it last saw the player
	Known                 []int
	Keys                  []string
	Hidden, CarriedBy     bool
	Death                 string
	AttackT, Cool         int
	BarkT                 int
	Reporter              bool // fled and reported already
	Moving                bool // walked this tick (animation)
	Running               bool
	Attacking             bool
	prevPos               Vec3
	walk                  float64 // distance since the last footstep
}

func newNPC(d *NPCDef) *NPC {
	n := &NPC{ID: d.ID, Name: d.Name, Kind: d.Kind, Model: d.Model, Voice: d.Voice, Yaw: d.Yaw, Health: 10, Alertness: 1,
		Patrol: slices.Clone(d.Patrol), Keys: slices.Clone(d.Keys)}
	n.Body = *character.New(d.Pos.X, d.Pos.Y, d.Pos.Z)
	if n.Name == "" {
		n.Name = map[string]string{"guard": "Guard", "civilian": "Servant", "target": "Target"}[n.Kind]
	}
	return n
}

func (n *NPC) pos() Vec3   { return Vec3{n.Body.X, n.Body.Y, n.Body.Z} }
func (n *NPC) eye() Vec3   { return Vec3{n.Body.X, n.Body.Y + 1.62, n.Body.Z} }
func (n *NPC) alive() bool { return n.State != StateDead && n.State != StateUnconscious }

// Pos is the NPC's feet position.
func (n *NPC) Pos() Vec3 { return n.pos() }

// Alive reports whether it is conscious.
func (n *NPC) Alive() bool { return n.alive() }

// IsGuard reports whether it fights.
func (n *NPC) IsGuard() bool { return n.Kind == "guard" }

// navWorld lets an NPC's pathfinder walk through door cells (it opens
// them), but not decorations' cells.
type navWorld struct{ s *Sim }

func (w navWorld) Solid(x, y, z int) bool {
	if _, door := w.s.doors[Cell{x, y, z}]; door {
		return false
	}
	return w.s.W.Solid(x, y, z) || w.s.decor[Cell{x, y, z}]
}

// bodyWorld is the character controller's view: closed doors are solid,
// and decorations are boxes (DecorSize).
type bodyWorld struct{ s *Sim }

func (w bodyWorld) Solid(x, y, z int) bool { return w.s.W.Solid(x, y, z) }

func (w bodyWorld) Boxes(area collide.AABB, out []collide.AABB) []collide.AABB {
	for _, o := range w.s.Objects {
		if o.Kind != KindDecor {
			continue
		}
		sz, ok := DecorSize[o.Name]
		if !ok {
			sz = Vec3{0.5, 1, 0.5} // unknown: the whole cell
		}
		b := collide.AABB{MinX: o.Pos.X - sz.X, MinY: o.Pos.Y, MinZ: o.Pos.Z - sz.Z, MaxX: o.Pos.X + sz.X, MaxY: o.Pos.Y + sz.Y, MaxZ: o.Pos.Z + sz.Z}
		if b.Intersects(area) {
			out = append(out, b)
		}
	}
	return out
}

// DecorSize is each decoration's collision box by Name: half width (X, Z)
// and height (Y) from its feet. The game fills it from the models,
// widest extent on both axes so the box covers any yaw.
var DecorSize = map[string]Vec3{}

func (w bodyWorld) Liquid(x, y, z int) character.Liquid {
	if b := w.s.W.Block(x, y, z); b == "minecraft:water" {
		return character.LiquidWater
	}
	return character.LiquidNone
}

// Body is the world as the player's and NPCs' bodies collide with it.
func (s *Sim) Body() character.World { return bodyWorld{s} }

var _ character.World = bodyWorld{}
var _ collide.Boxer = bodyWorld{}

func (s *Sim) newNav(k int) *nav.Graph {
	n := s.NPCs[k]
	return &nav.Graph{W: navWorld{s}, MaxDrop: 3, Passable: func(from, to nav.Pos) bool {
		if di, ok := s.doors[Cell{to.X, to.Y, to.Z}]; ok {
			d := s.Objects[di]
			return !d.Locked || slices.Contains(n.Keys, d.Key)
		}
		return true
	}}
}

// bark makes an NPC say something (rate limited).
func (s *Sim) bark(k int, text string) {
	n := s.NPCs[k]
	if n.BarkT > 0 && text != "" {
		return
	}
	n.BarkT = 3 * TicksPerSecond
	s.event(Event{Kind: EvBark, Text: n.Name + ": " + text, Line: text, Pos: n.eye(), NPC: k})
}

func (s *Sim) setState(k int, st NPCState) {
	n := s.NPCs[k]
	if n.State == st {
		return
	}
	n.State, n.StateT = st, 0
	n.Path, n.Stuck, n.Repath = nil, 0, 0
}

// alertNPC puts an NPC on full alert toward pos (it was hit or saw the
// player clearly).
func (s *Sim) alertNPC(k int, pos Vec3) {
	n := s.NPCs[k]
	if !n.alive() {
		return
	}
	n.Suspicion, n.LKP = AlertAt, pos
	if n.IsGuard() {
		if n.State != StateCombat {
			s.Stats.TimesSpotted++
			s.Seen = true
			s.bark(k, "There you are!")
			s.emit(n.eye(), 60, "alarm", k)
		}
		s.setState(k, StateCombat)
		n.LastSeen = 0
		return
	}
	if n.State != StateFlee && n.State != StateCower {
		s.bark(k, "Help! Guards!")
		s.emit(n.eye(), 60, "alarm", k)
		s.setState(k, StateFlee)
	}
}

func (s *Sim) tickNPC(k int) {
	n := s.NPCs[k]
	if n.BarkT > 0 {
		n.BarkT--
	}
	n.Moving, n.Running, n.Attacking = false, false, false
	if !n.alive() || n.CarriedBy || n.Hidden {
		if !n.CarriedBy && !n.Hidden {
			n.Body.Tick(bodyWorld{s}, character.Input{Yaw: n.Yaw}) // fall
		}
		return
	}
	n.StateT++
	stim := s.perceive(k)
	s.decide(k, stim)
	s.act(k)
}

// perceive updates suspicion from sight and sound; it reports whether
// anything new caught the NPC's attention this tick.
func (s *Sim) perceive(k int) bool {
	n := s.NPCs[k]
	got := false
	// Sight of the player.
	if g := s.seePlayer(k); g > 0 {
		n.Suspicion += g * n.Alertness
		n.LKP = s.Player.Pos
		got = true
		if n.Suspicion >= AlertAt {
			n.LastSeen = 0
		}
	} else {
		n.LastSeen++
	}
	// Sounds.
	thr := 8.0
	if n.Kind != "guard" {
		thr = 11
	}
	for _, snd := range s.sounds {
		if snd.byNPC == k {
			continue
		}
		f := s.Rooms.spread(snd.pos, s.doorClosed)
		rcv, _ := s.Rooms.heard(f, snd.loud, n.eye())
		if rcv <= thr {
			continue
		}
		over := rcv - thr
		switch snd.kind {
		case "alarm", "explosion", "pain":
			if n.State != StateCombat {
				n.Suspicion = math.Max(n.Suspicion, 80)
				n.LKP = snd.pos
				got = true
				if n.IsGuard() {
					s.setState(k, StateSearch)
					s.bark(k, "What's going on?!")
				} else {
					s.setState(k, StateFlee)
				}
			}
			continue
		case "footstep", "lockpick":
			n.Suspicion += over * 0.9
		case "door", "squeak", "loot", "unlock":
			n.Suspicion += over * 1.2
		case "noisemaker", "crash", "clatter", "arrow", "zap", "stab", "thud":
			n.Suspicion += over * 2.5
		case "body", "fire", "ignite", "splash", "hiss":
			n.Suspicion += over * 1.8
		default:
			n.Suspicion += over * 0.5
		}
		n.Suspicion = math.Min(n.Suspicion, AlertAt-1) // sound alone never confirms
		n.LKP = snd.pos
		got = true
	}
	// Bodies and fires.
	for j, o := range s.NPCs {
		if j == k || o.alive() || o.Hidden || o.CarriedBy || slices.Contains(n.Known, j) {
			continue
		}
		if s.canSee(k, o.pos().Add(Vec3{0, 0.3, 0}), 14, 0.1) {
			n.Known = append(n.Known, j)
			s.Stats.BodiesFound++
			n.LKP = o.pos()
			got = true
			if o.State == StateDead {
				s.bark(k, "Someone's been murdered!")
			} else {
				s.bark(k, "Wake up! Who did this?")
			}
			s.emit(n.eye(), 60, "alarm", k)
			n.Alertness = math.Min(2, n.Alertness+0.5)
			if n.IsGuard() {
				n.Suspicion = 90
				s.setState(k, StateSearch)
			} else {
				s.setState(k, StateFlee)
			}
		}
	}
	for _, o := range s.Objects {
		if o.Burning() && n.State <= StateSuspicious && s.canSee(k, o.Pos.Add(Vec3{0, 0.5, 0}), 20, 0) {
			n.Suspicion = math.Max(n.Suspicion, InvestigateAt+5)
			n.LKP = o.Pos
			s.bark(k, "Fire!")
			got = true
		}
	}
	return got
}

// canSee reports whether a point is in an NPC's view cone, in range,
// lit enough (or close) and not behind a wall.
func (s *Sim) canSee(k int, p Vec3, rng, minLight float64) bool {
	n := s.NPCs[k]
	e := n.eye()
	d := e.Dist(p)
	if d > rng || angleDiff(yawTo(e, p), n.Yaw) > 1.2 {
		return false
	}
	if d > 3 && lightAt(s.W, p.Cell(), s.Daylight()) < minLight {
		return false
	}
	return lineOfSight(s.W, e, p)
}

// seePlayer returns how much suspicion sight of the player adds this tick:
// a narrow focal cone sees fast and far, a wide peripheral cone slowly and
// near, and both scale with the light gem.
func (s *Sim) seePlayer(k int) float64 {
	n := s.NPCs[k]
	p := &s.Player
	if p.Dead {
		return 0
	}
	e := n.eye()
	body := p.Pos.Add(Vec3{0, 1.0, 0})
	d := e.Dist(body)
	if d > 26 {
		return 0
	}
	off := angleDiff(yawTo(e, body), n.Yaw)
	var cone, rng float64
	switch {
	case off < 0.55: // ~30° each side
		cone, rng = 1, 24
	case off < 1.35: // ~75° each side
		cone, rng = 0.3, 10
	case d < 1.6:
		cone, rng = 0.15, 2 // someone bumping into you from behind
	default:
		return 0
	}
	if d > rng {
		return 0
	}
	if !lineOfSight(s.W, e, p.Eye()) && !lineOfSight(s.W, e, body) {
		return 0
	}
	vis := s.Visibility
	if d < 1.5 {
		vis = math.Max(vis, 0.5) // too close to hide
	}
	eff := math.Max(0, vis-0.06)
	if eff == 0 {
		return 0
	}
	near := 1 - d/rng
	return 70 * cone * math.Pow(eff, 1.4) * (0.25 + 0.75*near)
}

func (s *Sim) doorClosed(i int) bool { o := s.Objects[i]; return !o.Open && !o.Gone }

// decide moves between states.
func (s *Sim) decide(k int, stim bool) {
	n := s.NPCs[k]
	if !stim && n.State != StateCombat {
		n.Suspicion = math.Max(0, n.Suspicion-0.35)
	}
	if n.Suspicion >= AlertAt && n.LastSeen == 0 {
		s.alertNPC(k, s.Player.Pos)
		return
	}
	switch n.State {
	case StatePatrol:
		switch {
		case n.Suspicion >= InvestigateAt:
			s.setState(k, StateInvestigate)
			s.bark(k, "I'd better check that out.")
		case n.Suspicion >= SuspiciousAt:
			s.setState(k, StateSuspicious)
			s.bark(k, pick(s.rng.Intn(3), "Hm? What was that?", "Is someone there?", "Who's there?"))
		}
	case StateSuspicious:
		switch {
		case n.Suspicion >= InvestigateAt:
			s.setState(k, StateInvestigate)
			s.bark(k, "I'd better check that out.")
		case n.Suspicion < SuspiciousAt-8:
			s.setState(k, StatePatrol)
			s.bark(k, pick(s.rng.Intn(2), "Must have been rats.", "Just my imagination."))
		}
	case StateInvestigate:
		if n.StateT > 25*TicksPerSecond || n.Suspicion < SuspiciousAt-8 {
			s.setState(k, StatePatrol)
			s.bark(k, "Nothing here.")
		}
	case StateSearch:
		if n.StateT > 30*TicksPerSecond {
			s.setState(k, StatePatrol)
			n.Suspicion = SuspiciousAt - 10
			n.Alertness = math.Min(2, n.Alertness+0.25)
			s.bark(k, "Must have gone. I'll keep my eyes open.")
		}
	case StateCombat:
		if n.LastSeen > 5*TicksPerSecond {
			s.setState(k, StateSearch)
			n.Suspicion = 90
			s.bark(k, "Where did you go?")
		}
	case StateCower:
		if n.StateT > 40*TicksPerSecond {
			s.setState(k, StatePatrol)
			n.Suspicion = 0
		}
	}
}

func pick(i int, s ...string) string { return s[i%len(s)] }

// act moves and fights according to the state.
func (s *Sim) act(k int) {
	n := s.NPCs[k]
	in := character.Input{Yaw: n.Yaw}
	switch n.State {
	case StatePatrol:
		if len(n.Patrol) == 0 {
			break
		}
		wp := n.Patrol[n.PatrolI%len(n.Patrol)]
		if n.Wait > 0 {
			n.Wait--
			if wp.Face {
				s.turn(n, wp.Yaw, 0.15)
			}
			if n.Wait == 0 {
				n.PatrolI = (n.PatrolI + 1) % len(n.Patrol)
			}
			break
		}
		if s.walkTo(k, wp.Pos, 0.35, &in) {
			n.Wait = max(1, wp.Wait)
		}
	case StateSuspicious:
		s.turn(n, yawTo(n.pos(), n.LKP), 0.12)
	case StateInvestigate:
		if n.pos().DistXZ(n.LKP) < 1.5 || !s.walkTo(k, n.LKP, 0.55, &in) && n.Path == nil && n.StateT > 20 {
			// Arrived (or can't get there): look around.
			s.turn(n, n.Yaw+0.05, 1)
		}
	case StateSearch:
		if n.Goal == (Vec3{}) || n.pos().DistXZ(n.Goal) < 1.2 || n.Stuck > 40 || (n.Path != nil && len(n.Path) == 0) || n.StateT%(8*TicksPerSecond) == 0 {
			n.Goal = s.searchPoint(n.LKP)
			n.Path = nil
		}
		s.walkTo(k, n.Goal, 0.6, &in)
	case StateCombat:
		s.fight(k, &in)
	case StateFlee:
		g := s.nearestGuard(n.pos())
		if g < 0 {
			s.setState(k, StateCower)
			break
		}
		gp := s.NPCs[g].pos()
		if n.pos().DistXZ(gp) < 2.5 {
			gd := s.NPCs[g]
			if gd.State != StateCombat {
				gd.LKP, gd.Suspicion = n.LKP, 90
				s.setState(g, StateSearch)
				s.bark(g, "Show me where!")
			}
			n.Reporter = true
			s.setState(k, StateCower)
			break
		}
		s.walkTo(k, gp, 1, &in)
		in.Sprint = true
	case StateCower:
		s.turn(n, yawTo(n.pos(), n.LKP), 0.1)
	}
	n.prevPos = n.pos()
	n.Body.Tick(bodyWorld{s}, in)
	n.Moving = math.Hypot(n.Body.X-n.Body.PrevX, n.Body.Z-n.Body.PrevZ) > 0.01
	n.Running = n.Moving && in.Forward > 0.6
	// Footsteps are for the player's ears only: not a sim sound, or NPCs
	// would spook each other.
	if n.Moving && n.Body.OnGround {
		n.walk += n.pos().DistXZ(n.prevPos)
		if n.walk >= 1.7 {
			n.walk -= 1.7
			loud := Material(s.FloorSurface(n.pos()))
			if n.Running {
				loud *= 1.35
			}
			s.event(Event{Kind: EvSound, Sound: "footstep", Pos: n.pos(), Loud: loud, NPC: k})
		}
	}
}

// fight chases and strikes the player.
func (s *Sim) fight(k int, in *character.Input) {
	n := s.NPCs[k]
	p := &s.Player
	if n.Cool > 0 {
		n.Cool--
	}
	d := n.pos().DistXZ(p.Pos)
	if n.AttackT > 0 {
		n.AttackT++
		n.Attacking = true
		s.turn(n, yawTo(n.pos(), p.Pos), 0.3)
		if n.AttackT == 9 && d < 2.4 && math.Abs(n.pos().Y-p.Pos.Y) < 1.6 {
			s.emit(p.Pos, 28, "hit", k)
			s.HurtPlayer(7, "Cut down by "+n.Name+".")
		}
		if n.AttackT > 16 {
			n.AttackT, n.Cool = 0, 12
		}
		return
	}
	if n.LastSeen == 0 && d < 2.0 && n.Cool == 0 {
		n.AttackT = 1
		return
	}
	target := n.LKP
	if n.LastSeen == 0 {
		target = p.Pos
		if n.Repath <= 0 {
			n.Path = nil
		}
	}
	if d > 1.4 || n.LastSeen > 0 {
		s.walkTo(k, target, 1, in)
	} else {
		s.turn(n, yawTo(n.pos(), p.Pos), 0.3)
	}
}

func (s *Sim) nearestGuard(p Vec3) int {
	best, bd := -1, math.Inf(1)
	for j, g := range s.NPCs {
		if g.IsGuard() && g.alive() {
			if d := g.pos().Dist(p); d < bd {
				best, bd = j, d
			}
		}
	}
	return best
}

// searchPoint picks a walkable spot within a few blocks of p.
func (s *Sim) searchPoint(p Vec3) Vec3 {
	for i := 0; i < 12; i++ {
		c := Vec3{p.X + s.rng.Float64()*12 - 6, p.Y + 1, p.Z + s.rng.Float64()*12 - 6}.Cell()
		if g, ok := s.groundBelow(c, 3); ok {
			return g.Floor()
		}
	}
	return p
}

// turn rotates the NPC toward yaw by at most rate radians.
func (s *Sim) turn(n *NPC, yaw, rate float64) {
	d := math.Remainder(yaw-n.Yaw, 2*math.Pi)
	n.Yaw += clamp(d, -rate, rate)
}

// walkTo follows a path toward goal at the given speed (0..1) and reports
// arrival. It opens closed doors on the way.
func (s *Sim) walkTo(k int, goal Vec3, speed float64, in *character.Input) bool {
	n := s.NPCs[k]
	p := n.pos()
	if p.DistXZ(goal) < 0.45 && math.Abs(p.Y-goal.Y) < 1.2 {
		n.Path = nil
		return true
	}
	if n.Repath > 0 {
		n.Repath--
	}
	if n.Path == nil || n.Goal.Dist(goal) > 1 || n.Stuck > 30 || (len(n.Path) == 0 && n.Repath == 0) {
		g := s.navs[k]
		from, ok1 := g.Ground(floor(p.X), floor(p.Y+0.3), floor(p.Z))
		to, ok2 := g.Ground(floor(goal.X), floor(goal.Y+0.3), floor(goal.Z))
		n.Goal, n.Stuck, n.Repath = goal, 0, 10
		if !ok1 || !ok2 {
			n.Path = []Cell{}
			return false
		}
		path, ok := g.Path(from, to, 6000)
		if !ok {
			n.Path = []Cell{}
			return false
		}
		n.Path = n.Path[:0]
		for _, q := range path {
			n.Path = append(n.Path, Cell{q.X, q.Y, q.Z})
		}
		n.PathI = 1
	}
	if n.PathI >= len(n.Path) {
		return len(n.Path) > 0 && p.DistXZ(goal) < 1.2
	}
	next := n.Path[n.PathI]
	// Doors: open them as we reach them.
	for _, c := range []Cell{next, next.Add(0, 1, 0)} {
		if di, ok := s.doors[c]; ok && !s.Objects[di].Open {
			if p.DistXZ(c.Center()) < 1.8 {
				s.setDoor(di, true)
				s.emit(c.Center(), 28, "door", k)
			}
		}
	}
	t := next.Floor()
	if p.DistXZ(t) < 0.3 && math.Abs(p.Y-t.Y) < 0.6 {
		n.PathI++
		if n.PathI >= len(n.Path) {
			return p.DistXZ(goal) < 1.2
		}
		t = n.Path[n.PathI].Floor()
		next = n.Path[n.PathI]
	}
	s.turn(n, yawTo(p, t), 0.35)
	in.Yaw = n.Yaw
	in.Forward = speed
	if angleDiff(yawTo(p, t), n.Yaw) > 1.2 {
		in.Forward = 0.05 // turn first
	}
	if float64(next.Y) > p.Y+0.5 && n.Body.OnGround && p.DistXZ(t) < 1.4 {
		in.Jump = true
	}
	if moved := n.pos().DistXZ(n.prevPos); moved < 0.01 {
		n.Stuck++
	} else {
		n.Stuck = 0
	}
	return false
}
