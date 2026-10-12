package sim

import (
	"math"
	"math/rand"
	"slices"
	"sort"

	"github.com/memmaker/voxellib/voxel/collide"
	"github.com/memmaker/voxellib/voxel/nav"
)

// TicksPerSecond is the simulation rate.
const TicksPerSecond = 20

// Object kinds.
const (
	KindTorch     = "torch"     // wall torch: lit, can be doused or knocked down
	KindLamp      = "lamp"      // standing electric lamp: knock it into water
	KindFaucet    = "faucet"    // leaks water while running
	KindDoor      = "door"      // two cells high; may be locked
	KindLoot      = "loot"      // valuables
	KindChest     = "chest"     // container: loot inside, bodies fit in
	KindKey       = "key"       // opens doors/chests with the same Key
	KindAmmo      = "ammo"      // Name = ammo type, Count
	KindFlammable = "flammable" // rugs, tapestries, hay
	KindDecor     = "decor"     // decoration: Name picks the model; collides (DecorSize), never targeted
)

// Ammo types.
const (
	AmmoBroadhead = "broadhead"
	AmmoWater     = "water"
	AmmoNoise     = "noisemaker"
	AmmoExplosive = "explosive"
)

// Object is anything in the world the player or the stimulus system can
// act on. Pos is the floor centre of the cell it stands in (wall torches:
// the cell in front of the wall).
type Object struct {
	ID, Kind, Name string
	Pos            Vec3
	Yaw            float64
	Value          int    // loot value
	Key            string // lock id (doors, chests) or the id a key opens
	Count          int    // ammo
	Locked, Open   bool
	Lit            bool // torches, lamps (powered and on)
	Fallen         bool
	// On is what a torch is attached to, like Minecraft: "floor" (the block
	// below) or the wall in that direction: north (-Z), south, east (+X),
	// west. Lanterns (Name) stand on floors or hang from the "ceiling".
	// Empty is resolved by TorchOn/LanternOn when the sim starts.
	On        string
	Held      bool // torch in the player's hand
	Running   bool // faucets
	Gone      bool // taken or destroyed
	Burn      int  // ticks burning; 0 = not burning
	Burnt     bool
	Pick      int   // lockpicking progress
	Hidden    []int // NPC indices of bodies stored inside
	LightCell Cell  // where its light block is, when lit
	HasLight  bool
}

// Cells occupied by a door.
func (o *Object) doorCells() [2]Cell {
	c := o.Pos.Cell()
	return [2]Cell{c, c.Add(0, 1, 0)}
}

// wallDirs are the wall a torch hangs on, by On.
var wallDirs = map[string][2]int{"north": {0, -1}, "south": {0, 1}, "east": {1, 0}, "west": {-1, 0}}

// WallDir is the direction of the wall a torch hangs on (0,0: on the floor).
func (o *Object) WallDir() (dx, dz int) {
	d := wallDirs[o.On]
	return d[0], d[1]
}

// TorchWall reports whether a torch can hang on the block at x,y,z: solid
// and not glass (glass holds torches on top, not on its sides).
func TorchWall(w World) func(x, y, z int) bool {
	return func(x, y, z int) bool { return w.Solid(x, y, z) && !IsGlass(w.Block(x, y, z)) }
}

// TorchOn picks what a torch in cell c is attached to: a solid side wall,
// else the floor. Missions without On get this at start.
func TorchOn(wall func(x, y, z int) bool, c Cell) string {
	for _, on := range []string{"north", "south", "east", "west"} {
		d := wallDirs[on]
		if wall(c.X+d[0], c.Y, c.Z+d[1]) {
			return on
		}
	}
	return "floor"
}

// LanternOn picks what a lantern in cell c hangs from: the ceiling when
// the block above is solid, else the floor.
func LanternOn(solid func(x, y, z int) bool, c Cell) string {
	if solid(c.X, c.Y+1, c.Z) {
		return "ceiling"
	}
	return "floor"
}

// Burning reports whether the object is on fire.
func (o *Object) Burning() bool { return o.Burn > 0 }

// Player is the player's state as the game reports it each tick, plus
// what the simulation tracks for them.
type Player struct {
	Pos         Vec3 // feet
	Yaw, Pitch  float64
	Crouch, Run bool
	OnGround    bool
	Health      int
	Carrying    int // NPC index, -1 = none
	Keys        []string
	Ammo        map[string]int
	Loot        int
	Dead        bool
	walk        float64
	last        Vec3
	lastInit    bool
}

// Eye is the player's eye position.
func (p *Player) Eye() Vec3 {
	h := 1.62
	if p.Crouch {
		h = 1.27
	}
	return p.Pos.Add(Vec3{0, h, 0})
}

// EventKind classifies Events.
type EventKind int

const (
	EvSound   EventKind = iota // play Sound at Pos (Loud: source loudness)
	EvBark                     // NPC speaks: caption Text
	EvMessage                  // HUD message Text
	EvHurt                     // player was hurt
	EvSpark                    // electricity in the puddle at Pos
	EvBlast                    // explosion at Pos
)

// Event is something the game should show or play.
type Event struct {
	Kind  EventKind
	Sound string
	Pos   Vec3
	Loud  float64
	Text  string
	Line  string // EvBark: what was said, without the speaker's name
	NPC   int    // EvBark, and EvSound made by an NPC (else -1)
}

// Outcome of the mission.
type Outcome int

const (
	Running Outcome = iota
	Success
	Failed
)

// Stats are shown at the end of the mission.
type Stats struct {
	Ticks, Kills, Knockouts, Accidents, TimesSpotted, BodiesFound int
	TargetsKilled                                                 int
	InnocentsKilled                                               int
}

// sound is a sound emitted this tick, for NPCs to hear.
type sound struct {
	pos    Vec3
	loud   float64
	kind   string
	byNPC  int // -1 = player/world
	player bool
}

type bomb struct {
	Pos  Vec3
	Fuse int
}

type noisemaker struct {
	Pos Vec3
	T   int
}

// State is everything that changes during a mission (saved by quicksave).
type State struct {
	Now         int     // ticks since the start
	Time        float64 // seconds since midnight
	Player      Player
	NPCs        []*NPC
	Objects     []*Object
	Puddles     map[Cell]int // wet floor cells (feet cells); value = ticks of electricity left
	Bombs       []bomb
	Noisemakers []noisemaker
	Outcome     Outcome
	FailReason  string
	Stats       Stats
	Visibility  float64      // player's light gem, 0..1
	Done        map[int]bool // objective index → complete
	Seen        bool         // ever fought
}

// Sim is a running mission.
type Sim struct {
	State
	W       World
	Mission *Mission
	Rooms   *RoomMap
	Events  []Event

	doors  map[Cell]int // door cell → object index
	decor  map[Cell]bool
	sounds []sound
	navs   []*nav.Graph
	rng    *rand.Rand
}

// New starts a mission in a world. The world must already hold the
// mission's blocks.
func New(w World, m *Mission) *Sim {
	s := &Sim{W: w, Mission: m, rng: rand.New(rand.NewSource(1))}
	s.Time = m.StartTime
	s.Puddles = map[Cell]int{}
	s.decor = map[Cell]bool{}
	s.Done = map[int]bool{}
	s.Player = Player{Pos: m.Start, Yaw: m.StartYaw, Health: 20, Carrying: -1, Ammo: map[string]int{}}
	for k, v := range m.Ammo {
		s.Player.Ammo[k] = v
	}
	for _, d := range m.Objects {
		o := *d
		if o.Kind == KindTorch && o.Name == "Lantern" && o.On == "" {
			o.On = LanternOn(w.Solid, o.Pos.Cell())
		}
		if o.Kind == KindTorch && o.On == "" {
			o.On = TorchOn(TorchWall(w), o.Pos.Cell())
		}
		if o.Kind == KindDecor {
			s.decor[o.Pos.Cell()] = true
		}
		s.Objects = append(s.Objects, &o)
	}
	for _, d := range m.NPCs {
		s.NPCs = append(s.NPCs, newNPC(d))
	}
	s.init()
	for i, o := range s.Objects {
		switch o.Kind {
		case KindTorch, KindLamp:
			s.setLight(i, o.Lit)
		case KindDoor:
			s.setDoor(i, o.Open)
		}
	}
	return s
}

// init builds the derived, unsaved structures.
func (s *Sim) init() {
	s.doors = map[Cell]int{}
	for i, o := range s.Objects {
		if o.Kind == KindDoor && !o.Gone {
			for _, c := range o.doorCells() {
				s.doors[c] = i
			}
		}
	}
	s.Rooms = BuildRooms(s.W, s.Mission.Rooms, s.doors)
	s.navs = make([]*nav.Graph, len(s.NPCs))
	for i := range s.NPCs {
		s.navs[i] = s.newNav(i)
	}
	if s.rng == nil {
		s.rng = rand.New(rand.NewSource(int64(s.Now)))
	}
}

// Restore replaces the state with a saved one (quickload). The world must
// already hold the saved blocks.
func (s *Sim) Restore(st State) {
	s.State = st
	if s.Puddles == nil {
		s.Puddles = map[Cell]int{}
	}
	if s.Done == nil {
		s.Done = map[int]bool{}
	}
	s.sounds, s.Events = nil, nil
	s.init()
}

// Daylight is the sky light factor for the time of day: moonlight at
// night, rising over dawn (5:00–7:00) and falling over dusk (19:00–21:00).
func (s *Sim) Daylight() float64 { return DaylightAt(s.Time) }

// DaylightAt is the sky light factor at t seconds since midnight.
func DaylightAt(t float64) float64 {
	h := math.Mod(t/3600, 24)
	const night = 0.12
	switch {
	case h < 5 || h >= 21:
		return night
	case h < 7:
		return night + (1-night)*(h-5)/2
	case h < 19:
		return 1
	default:
		return 1 - (1-night)*(h-19)/2
	}
}

// Clock returns the time of day as hours and minutes.
func (s *Sim) Clock() (int, int) {
	t := int(math.Mod(s.Time, 86400))
	return t / 3600, t / 60 % 60
}

func (s *Sim) event(e Event) { s.Events = append(s.Events, e) }

func (s *Sim) message(text string) { s.event(Event{Kind: EvMessage, Text: text}) }

// emit makes a sound: NPCs hear it this tick, the game plays it.
func (s *Sim) emit(pos Vec3, loud float64, kind string, byNPC int) {
	s.sounds = append(s.sounds, sound{pos: pos, loud: loud, kind: kind, byNPC: byNPC})
	s.event(Event{Kind: EvSound, Sound: kind, Pos: pos, Loud: loud, NPC: byNPC})
}

// Tick advances the simulation one 50 ms step.
func (s *Sim) Tick() {
	if s.Outcome != Running {
		return
	}
	s.Now++
	s.Stats.Ticks++
	s.Time += s.Mission.TimeScale / TicksPerSecond
	s.tickPlayer()
	s.tickObjects()
	s.tickPuddles()
	s.tickBombs()
	for i := range s.NPCs {
		s.tickNPC(i)
	}
	s.sounds = s.sounds[:0]
	s.checkObjectives()
}

// FloorSurface is the footstep surface under someone standing at pos.
func (s *Sim) FloorSurface(pos Vec3) string {
	if _, wet := s.Puddles[pos.Cell()]; wet {
		return "water"
	}
	c := pos.Cell().Add(0, -1, 0)
	if pos.Y-math.Floor(pos.Y) > 0.3 { // standing on a slab-like top
		c = pos.Cell()
	}
	return Surface(s.W.Block(c.X, c.Y, c.Z))
}

// tickPlayer: footsteps, the light gem, carried bodies.
func (s *Sim) tickPlayer() {
	p := &s.Player
	if !p.lastInit {
		p.last, p.lastInit = p.Pos, true
	}
	moved := p.Pos.DistXZ(p.last)
	p.last = p.Pos
	if p.OnGround && moved > 0.01 {
		p.walk += moved
		const stride = 1.7
		if p.walk >= stride {
			p.walk -= stride
			loud := Material(s.FloorSurface(p.Pos))
			switch {
			case p.Crouch:
				loud *= 0.45
			case p.Run:
				loud *= 1.35
			}
			s.emit(p.Pos, loud, "footstep", -1)
		}
	}
	if i := s.HeldTorch(); i >= 0 {
		o := s.Objects[i]
		o.Pos = p.Pos.Add(Vec3{0, 0.6, 0}) // its light sits at chest height
		if o.Lit && (!o.HasLight || o.LightCell != o.Pos.Add(Vec3{0, 0.5, 0}).Cell()) {
			s.setLight(i, true)
		}
	}
	s.Visibility = s.visibility()
	if p.Carrying >= 0 {
		n := s.NPCs[p.Carrying]
		n.Body.Teleport(p.Pos.X, p.Pos.Y, p.Pos.Z)
	}
	if c := p.Pos.Cell(); s.Puddles[c] > 0 && !p.Dead {
		s.HurtPlayer(20, "Electrocuted.")
	}
}

// visibility is the light gem: how lit the player's body is, raised by
// moving and lowered by crouching.
func (s *Sim) visibility() float64 {
	p := &s.Player
	day := s.Daylight()
	l := 0.0
	for _, dy := range []float64{0.2, 1.0} {
		l = math.Max(l, lightAt(s.W, p.Pos.Add(Vec3{0, dy, 0}).Cell(), day))
	}
	v := l
	switch {
	case p.Crouch:
		v *= 0.8
	case p.Run:
		v *= 1.25
	}
	speed := 0.0
	if p.lastInit {
		speed = p.Pos.DistXZ(p.last)
	}
	v += math.Min(0.1, speed*0.4) * l
	if p.Carrying >= 0 {
		v += 0.05
	}
	return clamp(v, 0, 1)
}

// HurtPlayer damages the player; at 0 health the mission fails.
func (s *Sim) HurtPlayer(dmg int, why string) {
	p := &s.Player
	if p.Dead {
		return
	}
	p.Health -= dmg
	s.event(Event{Kind: EvHurt, Loud: float64(dmg)})
	if p.Health <= 0 {
		p.Health, p.Dead = 0, true
		s.fail("You died. " + why)
	}
}

func (s *Sim) fail(reason string) {
	if s.Outcome == Running {
		s.Outcome, s.FailReason = Failed, reason
	}
}

// setLight places or removes an object's light block.
func (s *Sim) setLight(i int, on bool) {
	o := s.Objects[i]
	if o.HasLight {
		c := o.LightCell
		if s.W.Block(c.X, c.Y, c.Z) == BlockLight {
			s.W.SetBlock(c.X, c.Y, c.Z, BlockAir)
		}
		o.HasLight = false
	}
	if !on {
		return
	}
	c := o.Pos.Cell()
	if o.Kind == KindTorch && !o.Fallen {
		c = o.Pos.Add(Vec3{0, 0.5, 0}).Cell()
	}
	if b := s.W.Block(c.X, c.Y, c.Z); b == BlockAir || b == BlockLight {
		s.W.SetBlock(c.X, c.Y, c.Z, BlockLight)
		o.LightCell, o.HasLight = c, true
	}
}

// setDoor opens (air) or closes (door blocks) a door.
func (s *Sim) setDoor(i int, open bool) {
	o := s.Objects[i]
	o.Open = open
	b := BlockDoor
	if open {
		b = BlockAir
	}
	for _, c := range o.doorCells() {
		s.W.SetBlock(c.X, c.Y, c.Z, b)
	}
}

// DoorAt returns the door object index at a cell, or -1.
func (s *Sim) DoorAt(c Cell) int {
	if i, ok := s.doors[c]; ok {
		return i
	}
	return -1
}

// tickObjects: fire, faucets, noisemakers, fallen lamps in water.
func (s *Sim) tickObjects() {
	for i, o := range s.Objects {
		if o.Gone {
			continue
		}
		if o.Burning() {
			o.Burn++
			if o.Burn%30 == 1 {
				s.emit(o.Pos, 30, "fire", -1)
			}
			if o.Burn == 60 { // spreads
				s.Stimulate(Stimulus{Stim: StimFire, Pos: o.Pos, Radius: 2.2})
			}
			for k, n := range s.NPCs {
				if n.alive() && n.pos().DistXZ(o.Pos) < 1.1 && math.Abs(n.pos().Y-o.Pos.Y) < 1.5 {
					s.killNPC(k, "accident", "burned")
				}
			}
			if o.Burn > 30*TicksPerSecond {
				o.Burn, o.Burnt = 0, true
				s.setLight(i, false)
			}
		}
		if o.Kind == KindFaucet && o.Running && s.Now%15 == 0 {
			s.Stimulate(Stimulus{Stim: StimWater, Pos: o.Pos, Radius: 0.5, Amount: 1})
			if s.Now%45 == 0 {
				s.emit(o.Pos, 14, "water", -1)
			}
		}
		if o.Kind == KindLamp && o.Fallen && o.Lit {
			if _, wet := s.Puddles[o.Pos.Cell()]; wet {
				s.electrify(o.Pos.Cell())
				o.Lit = false
				s.setLight(i, false)
			}
		}
	}
	for i := 0; i < len(s.Noisemakers); i++ {
		n := &s.Noisemakers[i]
		if n.T%8 == 0 {
			s.emit(n.Pos, 46, "noisemaker", -1)
		}
		n.T++
	}
	s.Noisemakers = slices.DeleteFunc(s.Noisemakers, func(n noisemaker) bool { return n.T > 6*TicksPerSecond })
}

// walkable reports whether a feet cell is open with solid floor below.
func (s *Sim) walkable(c Cell) bool {
	_, door := s.doors[c]
	return !door && !s.W.Solid(c.X, c.Y, c.Z) && s.W.Solid(c.X, c.Y-1, c.Z)
}

// spreadWater adds one wet cell next to the puddle around start (or start
// itself), up to max cells in the puddle.
func (s *Sim) spreadWater(start Cell, maxCells int) {
	if !s.walkable(start) {
		if g, ok := s.groundBelow(start, 3); ok {
			start = g
		} else {
			return
		}
	}
	if _, ok := s.Puddles[start]; !ok {
		s.wet(start)
		return
	}
	seen := map[Cell]bool{start: true}
	queue := []Cell{start}
	n := 0
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		if _, wet := s.Puddles[c]; !wet {
			s.wet(c)
			return
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

// wet makes c a puddle cell and shows the water.
func (s *Sim) wet(c Cell) {
	s.Puddles[c] = 0
	if s.W.Block(c.X, c.Y, c.Z) == BlockAir {
		s.W.SetBlock(c.X, c.Y, c.Z, BlockPuddle)
	}
}

// groundBelow finds the walkable cell at or below c.
func (s *Sim) groundBelow(c Cell, depth int) (Cell, bool) {
	for i := 0; i <= depth; i++ {
		if s.walkable(c) {
			return c, true
		}
		c.Y--
	}
	return c, false
}

// electrify puts current into the puddle containing c.
func (s *Sim) electrify(c Cell) {
	if _, ok := s.Puddles[c]; !ok {
		return
	}
	seen := map[Cell]bool{c: true}
	queue := []Cell{c}
	for len(queue) > 0 {
		x := queue[0]
		queue = queue[1:]
		s.Puddles[x] = 3 * TicksPerSecond
		for _, d := range []Cell{{1, 0, 0}, {-1, 0, 0}, {0, 0, 1}, {0, 0, -1}} {
			n := x.Add(d.X, 0, d.Z)
			if _, wet := s.Puddles[n]; wet && !seen[n] {
				seen[n] = true
				queue = append(queue, n)
			}
		}
	}
	s.emit(c.Floor(), 55, "zap", -1)
	s.event(Event{Kind: EvSpark, Pos: c.Floor()})
}

// tickPuddles runs electricity down and kills whoever stands in it.
func (s *Sim) tickPuddles() {
	for c, t := range s.Puddles {
		if t <= 0 {
			continue
		}
		s.Puddles[c] = t - 1
		for k, n := range s.NPCs {
			if n.alive() && n.pos().Cell() == c {
				s.killNPC(k, "accident", "electrocuted")
			}
		}
	}
}

func (s *Sim) tickBombs() {
	for i := 0; i < len(s.Bombs); i++ {
		b := &s.Bombs[i]
		if b.Fuse--; b.Fuse%10 == 0 && b.Fuse > 0 {
			s.emit(b.Pos, 18, "fuse", -1)
		}
		if b.Fuse <= 0 {
			s.Explode(b.Pos)
		}
	}
	s.Bombs = slices.DeleteFunc(s.Bombs, func(b bomb) bool { return b.Fuse <= 0 })
}

// Explode destroys blocks, objects and people around pos: the one way to
// break voxels.
func (s *Sim) Explode(pos Vec3) {
	const r = 2.6
	c := pos.Cell()
	for x := -3; x <= 3; x++ {
		for y := -3; y <= 3; y++ {
			for z := -3; z <= 3; z++ {
				b := c.Add(x, y, z)
				if b.Center().Dist(pos) > r {
					continue
				}
				name := s.W.Block(b.X, b.Y, b.Z)
				if name == BlockAir || name == "minecraft:bedrock" || name == "minecraft:obsidian" {
					continue
				}
				if _, door := s.doors[b]; door {
					continue // handled with the objects
				}
				s.W.SetBlock(b.X, b.Y, b.Z, BlockAir)
			}
		}
	}
	s.Stimulate(Stimulus{Stim: StimBlast, Pos: pos, Radius: r})
	s.Stimulate(Stimulus{Stim: StimFire, Pos: pos, Radius: 4.5})
	for k, n := range s.NPCs {
		if !n.alive() {
			continue
		}
		if d := n.pos().Dist(pos); d < 3.5 {
			s.killNPC(k, "player", "blown up")
		}
	}
	if d := s.Player.Pos.Dist(pos); d < 3.5 {
		s.HurtPlayer(int(24-d*5), "Caught in the blast.")
	}
	s.emit(pos, 95, "explosion", -1)
	s.event(Event{Kind: EvBlast, Pos: pos})
	s.init() // doors and walls changed: rebuild the room graph
}

// Shatter breaks the glass block at c, if it is glass: very loud. It
// reports whether it did.
func (s *Sim) Shatter(c Cell) bool {
	if !IsGlass(s.W.Block(c.X, c.Y, c.Z)) {
		return false
	}
	s.W.SetBlock(c.X, c.Y, c.Z, BlockAir)
	s.emit(c.Center(), 95, "shatter", -1) // as loud as a bomb
	s.init()                              // the window is an opening now: rebuild the room graph
	return true
}

// Target is what the player points at: an object or an NPC body.
type Target struct {
	Obj, NPC int // -1 = none
	Dist     float64
}

// NoTarget is the empty target.
var NoTarget = Target{Obj: -1, NPC: -1}

// ObjectBox is an object's selection box.
func (s *Sim) ObjectBox(i int) (lo, hi Vec3) {
	o := s.Objects[i]
	p := o.Pos
	switch o.Kind {
	case KindDoor:
		c := p.Cell()
		return Vec3{float64(c.X), float64(c.Y), float64(c.Z)}, Vec3{float64(c.X + 1), float64(c.Y + 2), float64(c.Z + 1)}
	case KindTorch:
		if !o.Fallen {
			dx, dz := o.WallDir() // a wall torch hangs 0.5 toward its wall
			p = p.Add(Vec3{0.3 * float64(dx), 0, 0.3 * float64(dz)})
			return p.Add(Vec3{-0.25, 0.1, -0.25}), p.Add(Vec3{0.25, 0.9, 0.25})
		}
		return p.Add(Vec3{-0.4, 0, -0.4}), p.Add(Vec3{0.4, 0.25, 0.4})
	case KindLamp:
		if o.Fallen {
			return p.Add(Vec3{-0.5, 0, -0.5}), p.Add(Vec3{0.5, 0.35, 0.5})
		}
		return p.Add(Vec3{-0.3, 0, -0.3}), p.Add(Vec3{0.3, 1.6, 0.3})
	case KindChest:
		return p.Add(Vec3{-0.45, 0, -0.45}), p.Add(Vec3{0.45, 0.8, 0.45})
	case KindFlammable:
		return p.Add(Vec3{-0.5, 0, -0.5}), p.Add(Vec3{0.5, 0.15, 0.5})
	case KindFaucet:
		return p.Add(Vec3{-0.35, 0, -0.35}), p.Add(Vec3{0.35, 1.0, 0.35})
	case KindDecor:
		b := s.DecorBox(i)
		return Vec3{b.MinX, b.MinY, b.MinZ}, Vec3{b.MaxX, b.MaxY, b.MaxZ}
	}
	return p.Add(Vec3{-0.3, 0, -0.3}), p.Add(Vec3{0.3, 0.5, 0.3})
}

// rayBox intersects a ray with a box (slab test).
func rayBox(o, d, lo, hi Vec3) (float64, bool) {
	t0, t1 := 0.0, math.Inf(1)
	for _, a := range [3][4]float64{{o.X, d.X, lo.X, hi.X}, {o.Y, d.Y, lo.Y, hi.Y}, {o.Z, d.Z, lo.Z, hi.Z}} {
		if math.Abs(a[1]) < 1e-9 {
			if a[0] < a[2] || a[0] > a[3] {
				return 0, false
			}
			continue
		}
		ta, tb := (a[2]-a[0])/a[1], (a[3]-a[0])/a[1]
		if ta > tb {
			ta, tb = tb, ta
		}
		t0, t1 = math.Max(t0, ta), math.Min(t1, tb)
		if t0 > t1 {
			return 0, false
		}
	}
	return t0, true
}

// Reach is how far the player can frob.
const Reach = 2.8

// TargetAt finds the nearest object or body along the view ray within
// reach and not behind a wall.
func (s *Sim) TargetAt(eye, dir Vec3) Target {
	best := NoTarget
	best.Dist = Reach
	for i, o := range s.Objects {
		if o.Gone || o.Held || o.Kind == KindDecor {
			continue
		}
		lo, hi := s.ObjectBox(i)
		if t, ok := rayBox(eye, dir, lo, hi); ok && t < best.Dist {
			hit := eye.Add(dir.Scale(t))
			if o.Kind == KindDoor || clearPath(s.W, eye, hit) {
				best = Target{Obj: i, NPC: -1, Dist: t}
			}
		}
	}
	for k, n := range s.NPCs {
		if n.CarriedBy || n.Hidden {
			continue
		}
		p := n.pos()
		lo, hi := p.Add(Vec3{-0.4, 0, -0.4}), p.Add(Vec3{0.4, 1.8, 0.4})
		if !n.alive() {
			lo, hi = p.Add(Vec3{-0.9, 0, -0.9}), p.Add(Vec3{0.9, 0.5, 0.9})
		}
		if t, ok := rayBox(eye, dir, lo, hi); ok && t < best.Dist && clearPath(s.W, eye, eye.Add(dir.Scale(t))) {
			best = Target{Obj: -1, NPC: k, Dist: t}
		}
	}
	return best
}

// Describe returns a short label for the target ("" = nothing).
func (s *Sim) Describe(t Target) string {
	if t.NPC >= 0 {
		n := s.NPCs[t.NPC]
		if !n.alive() {
			return n.Name + " (pick up)"
		}
		return n.Name
	}
	if t.Obj < 0 {
		return ""
	}
	o := s.Objects[t.Obj]
	name := o.Name
	if name == "" {
		name = o.Kind
	}
	switch o.Kind {
	case KindDoor:
		switch {
		case o.Locked && s.hasKey(o.Key):
			return name + " (locked - you have the key)"
		case o.Locked:
			return name + " (locked - hold E to pick)"
		case o.Open:
			return name + " (close)"
		}
		return name + " (open)"
	case KindLoot:
		return name + " (" + itoa(o.Value) + ")"
	case KindChest:
		switch {
		case s.Player.Carrying >= 0:
			return name + " (hide body)"
		case o.Locked && !s.hasKey(o.Key):
			return name + " (locked - hold E to pick)"
		case o.Value > 0:
			return name + " (" + itoa(o.Value) + " inside)"
		}
		return name + " (empty)"
	case KindTorch:
		if s.HeldTorch() < 0 {
			return name + " (take)"
		}
	case KindLamp:
		if !o.Fallen {
			return name + " (knock over)"
		}
	case KindFaucet:
		if o.Running {
			return name + " (turn off)"
		}
		return name + " (turn on)"
	case KindAmmo:
		return name + " x" + itoa(o.Count)
	}
	return name
}

func itoa(n int) string {
	if n < 0 {
		return "-" + itoa(-n)
	}
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}

func (s *Sim) hasKey(k string) bool { return k != "" && slices.Contains(s.Player.Keys, k) }

// Frob uses the target. held is true on the ticks after the first while
// the use key stays down (lockpicking).
func (s *Sim) Frob(t Target, held bool) {
	p := &s.Player
	if t.NPC >= 0 {
		if !held {
			s.pickUpBody(t.NPC)
		}
		return
	}
	if t.Obj < 0 {
		if p.Carrying >= 0 && !held {
			s.DropBody()
		}
		return
	}
	o := s.Objects[t.Obj]
	if o.Locked && (o.Kind == KindDoor || o.Kind == KindChest) && !(o.Kind == KindChest && p.Carrying >= 0) {
		if s.hasKey(o.Key) {
			if !held {
				o.Locked = false
				s.emit(o.Pos, 12, "unlock", -1)
				s.message("Unlocked with the key.")
			}
			return
		}
		o.Pick++
		if o.Pick%10 == 1 {
			s.emit(o.Pos, 14, "lockpick", -1)
		}
		if o.Pick >= 3*TicksPerSecond {
			o.Locked = false
			s.emit(o.Pos, 14, "unlock", -1)
			s.message("Lock picked.")
		}
		return
	}
	if held {
		return
	}
	switch o.Kind {
	case KindDoor:
		if o.Open && s.doorBlocked(t.Obj) {
			return
		}
		s.setDoor(t.Obj, !o.Open)
		s.emit(o.Pos.Add(Vec3{0, 1, 0}), 28, "door", -1)
	case KindLoot:
		o.Gone = true
		p.Loot += o.Value
		s.emit(o.Pos, 8, "loot", -1)
		s.message("Took " + o.Name + " (" + itoa(o.Value) + ")")
	case KindKey:
		o.Gone = true
		p.Keys = append(p.Keys, o.Key)
		s.message("Took " + o.Name)
	case KindAmmo:
		o.Gone = true
		p.Ammo[o.Name] += o.Count
		s.message("Took " + itoa(o.Count) + " " + o.Name)
	case KindChest:
		if p.Carrying >= 0 {
			n := s.NPCs[p.Carrying]
			n.Hidden, n.CarriedBy = true, false
			o.Hidden = append(o.Hidden, p.Carrying)
			p.Carrying = -1
			o.Open = true
			s.emit(o.Pos, 16, "body", -1)
			s.message("Body hidden.")
			return
		}
		if o.Value > 0 {
			p.Loot += o.Value
			s.message("Took " + itoa(o.Value) + " in valuables")
			o.Value = 0
			o.Open = true
			s.emit(o.Pos, 8, "loot", -1)
		} else {
			o.Open = !o.Open
			s.emit(o.Pos, 6, "door", -1)
		}
	case KindTorch:
		if s.HeldTorch() < 0 {
			o.Held, o.Fallen = true, false
			s.emit(o.Pos, 6, "swish", -1)
		}
	case KindLamp:
		if !o.Fallen {
			s.knockLamp(t.Obj)
		}
	case KindFaucet:
		o.Running = !o.Running
		s.emit(o.Pos, 12, "squeak", -1)
	}
}

// doorBlocked reports whether someone stands in a door's cells.
func (s *Sim) doorBlocked(i int) bool {
	for _, c := range s.Objects[i].doorCells() {
		if s.Player.Pos.Cell() == c || s.Player.Pos.Add(Vec3{0, 1, 0}).Cell() == c {
			return true
		}
		for _, n := range s.NPCs {
			if n.pos().Cell() == c && !n.CarriedBy && !n.Hidden {
				return true
			}
		}
	}
	return false
}

// HeldTorch is the index of the torch in the player's hand, or -1.
func (s *Sim) HeldTorch() int {
	for i, o := range s.Objects {
		if o.Held && !o.Gone {
			return i
		}
	}
	return -1
}

// ThrowTorch lobs the held torch up to dist blocks along the view (0 drops
// it at the player's feet). It clatters where it lands, which draws guards
// over, and sets flammables there alight.
func (s *Sim) ThrowTorch(eye, dir Vec3, dist float64) {
	i := s.HeldTorch()
	if i < 0 {
		return
	}
	o := s.Objects[i]
	o.Held, o.Fallen = false, true
	o.Pos = s.landing(eye, dir, dist)
	s.setLight(i, o.Lit)
	s.emit(o.Pos, 22, "clatter", -1)
	if o.Lit {
		s.Stimulate(Stimulus{Stim: StimFire, Pos: o.Pos, Radius: 1.2})
	}
}

// knockLamp tips a standing lamp over, away from the player.
func (s *Sim) knockLamp(i int) {
	o := s.Objects[i]
	yaw := yawTo(s.Player.Pos, o.Pos)
	fx, fz := forward(yaw)
	c := o.Pos.Cell()
	n := Vec3{o.Pos.X + fx, o.Pos.Y, o.Pos.Z + fz}.Cell()
	if g, ok := s.groundBelow(n, 2); ok {
		c = g
	}
	o.Fallen, o.Yaw = true, yaw
	o.Pos = c.Floor()
	if o.Lit {
		s.setLight(i, true)
	}
	s.emit(o.Pos, 36, "crash", -1)
}

// pickUpBody lifts an unconscious or dead NPC onto the player's shoulder.
func (s *Sim) pickUpBody(k int) {
	n := s.NPCs[k]
	if n.alive() || s.Player.Carrying >= 0 {
		return
	}
	n.CarriedBy = true
	s.Player.Carrying = k
	s.message("Carrying " + n.Name + ". Press E to drop, or E on a chest to hide.")
}

// DropBody puts the carried body down in front of the player.
func (s *Sim) DropBody() {
	p := &s.Player
	if p.Carrying < 0 {
		return
	}
	n := s.NPCs[p.Carrying]
	fx, fz := forward(p.Yaw)
	at := p.Pos
	if c := (Vec3{p.Pos.X + fx*0.9, p.Pos.Y + 0.5, p.Pos.Z + fz*0.9}).Cell(); s.walkable(c) {
		at = c.Floor()
	}
	n.Body.Teleport(at.X, at.Y, at.Z)
	n.CarriedBy = false
	p.Carrying = -1
	s.emit(at, 14, "body", -1)
}

// ArrowHit tells the simulation where an arrow struck: npc >= 0 when it
// hit a body (head: in the head).
func (s *Sim) ArrowHit(ammo string, pos Vec3, npc int, head bool) {
	switch ammo {
	case AmmoBroadhead:
		if npc >= 0 {
			n := s.NPCs[npc]
			if !n.alive() {
				return
			}
			dmg := 6
			if head || n.State != StateCombat && n.Suspicion < 55 {
				dmg = 99 // unaware or head shot: one arrow does it
			}
			s.hurtNPC(npc, dmg, "shot")
			return
		}
		if !s.Shatter(pos.Cell()) && !s.Stimulate(Stimulus{Stim: StimPierce, Pos: pos, Radius: 0.05}) {
			s.emit(pos, 26, "arrow", -1)
		}
	case AmmoWater:
		s.emit(pos, 18, "splash", -1)
		s.Stimulate(Stimulus{Stim: StimWater, Pos: pos, Radius: 0.9, Amount: 3})
	case AmmoNoise:
		s.Noisemakers = append(s.Noisemakers, noisemaker{Pos: pos})
	}
}

// Throw lobs an explosive along the view; it goes off after 3 seconds.
func (s *Sim) Throw(eye, dir Vec3) bool {
	p := &s.Player
	if p.Ammo[AmmoExplosive] <= 0 {
		return false
	}
	p.Ammo[AmmoExplosive]--
	land := s.landing(eye, dir, 7)
	s.Bombs = append(s.Bombs, bomb{Pos: land, Fuse: 3 * TicksPerSecond})
	s.emit(land, 20, "clatter", -1)
	return true
}

// landing is the floor where something thrown up to dist blocks along
// dir comes down.
func (s *Sim) landing(eye, dir Vec3, dist float64) Vec3 {
	land := eye
	for t := 0.0; t < dist; t += 0.25 {
		q := eye.Add(dir.Scale(t))
		if s.W.Solid(q.Cell().X, q.Cell().Y, q.Cell().Z) {
			break
		}
		land = q
	}
	if g, ok := s.groundBelow(land.Cell(), 8); ok {
		land = g.Floor()
	}
	return land
}

// Melee swings the blackjack or the blade at the NPC in front of the
// player, else at decoration or a window. It reports whether it hit.
func (s *Sim) Melee(tool string) bool {
	p := &s.Player
	best, bestD := -1, 2.4
	for k, n := range s.NPCs {
		if !n.alive() {
			continue
		}
		d := n.pos().DistXZ(p.Pos)
		if d < bestD && angleDiff(yawTo(p.Pos, n.pos()), p.Yaw) < 0.8 && math.Abs(n.pos().Y-p.Pos.Y) < 1.5 {
			best, bestD = k, d
		}
	}
	if best < 0 {
		s.emit(p.Pos, 6, "swish", -1)
		// A swing at a window within reach breaks it.
		eye := p.Eye()
		fx, fz := forward(p.Yaw)
		cp := math.Cos(p.Pitch)
		dir := Vec3{fx * cp, math.Sin(p.Pitch), fz * cp}
		for t := 0.2; t < 2.2; t += 0.1 {
			pt := eye.Add(dir.Scale(t))
			for i, o := range s.Objects {
				if o.Kind == KindDecor && s.DecorBox(i).Intersects(collide.AABB{MinX: pt.X, MinY: pt.Y, MinZ: pt.Z, MaxX: pt.X, MaxY: pt.Y, MaxZ: pt.Z}) {
					st := StimBash
					if tool == "blade" {
						st = StimPierce
					}
					s.Stimulate(Stimulus{Stim: st, Pos: pt})
					return true
				}
			}
			c := pt.Cell()
			if s.W.Solid(c.X, c.Y, c.Z) {
				return s.Shatter(c)
			}
		}
		return false
	}
	n := s.NPCs[best]
	aware := n.State == StateCombat
	switch tool {
	case "blackjack":
		if aware {
			s.emit(n.pos(), 20, "thud", -1)
			s.alertNPC(best, p.Pos)
			return true
		}
		n.State = StateUnconscious
		n.Path = nil
		s.Stats.Knockouts++
		s.emit(n.pos(), 18, "body", -1)
		s.bark(best, "Ugh...")
	case "blade":
		dmg := 99
		if aware {
			dmg = 4
		}
		s.emit(n.pos(), 24, "stab", -1)
		s.hurtNPC(best, dmg, "stabbed")
	}
	return true
}

// hurtNPC damages an NPC and makes it fight back or run.
func (s *Sim) hurtNPC(k, dmg int, how string) {
	n := s.NPCs[k]
	n.Health -= dmg
	if n.Health <= 0 {
		s.killNPC(k, "player", how)
		return
	}
	s.emit(n.pos(), 30, "pain", k)
	s.alertNPC(k, s.Player.Pos)
}

// killNPC kills an NPC; by is "player" or "accident".
func (s *Sim) killNPC(k int, by, how string) {
	n := s.NPCs[k]
	if n.State == StateDead {
		return
	}
	n.State, n.Path, n.Death = StateDead, nil, by
	if by == "accident" {
		s.Stats.Accidents++
	} else {
		s.Stats.Kills++
	}
	if n.Kind == "target" {
		s.Stats.TargetsKilled++
		if by == "accident" {
			s.message(n.Name + " died in an accident: " + how + ".")
		} else {
			s.message(n.Name + " is dead.")
		}
	} else if n.Kind == "civilian" {
		s.Stats.InnocentsKilled++
	}
	s.emit(n.pos(), 26, "body", -1)
	s.emit(n.eye(), 20, "death", k)
}

// checkObjectives marks objectives done and ends the mission.
func (s *Sim) checkObjectives() {
	m := s.Mission
	required := true
	for i, ob := range m.Objectives {
		done := false
		switch ob.Kind {
		case "steal":
			for _, o := range s.Objects {
				if o.ID == ob.Target && o.Gone && o.Kind == KindLoot {
					done = true
				}
			}
		case "loot":
			done = s.Player.Loot >= m.LootQuota
		case "kill":
			for _, n := range s.NPCs {
				if n.ID == ob.Target && n.State == StateDead {
					done = true
				}
			}
		case "accident":
			for _, n := range s.NPCs {
				if n.ID == ob.Target && n.State == StateDead && n.Death == "accident" {
					done = true
				}
			}
		case "nokill":
			done = s.Stats.InnocentsKilled == 0
		case "ghost":
			done = !s.Seen
		case "exit":
			continue
		}
		s.Done[i] = done
		if !done && !ob.Optional {
			required = false
		}
	}
	for i, ob := range m.Objectives {
		if ob.Kind == "exit" {
			in := m.Exit.Contains(s.Player.Pos.Cell())
			s.Done[i] = in && required
			if in && required {
				s.Outcome = Success
			}
		}
	}
}

// ObjectiveLines returns the objectives for the HUD, with done marks.
func (s *Sim) ObjectiveLines() []string {
	var out []string
	idx := make([]int, len(s.Mission.Objectives))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool {
		return !s.Mission.Objectives[idx[a]].Optional && s.Mission.Objectives[idx[b]].Optional
	})
	for _, i := range idx {
		ob := s.Mission.Objectives[i]
		mark := "[ ] "
		if s.Done[i] {
			mark = "[x] "
		}
		t := ob.Text
		if ob.Optional {
			t += " (optional)"
		}
		out = append(out, mark+t)
	}
	return out
}
