package sim

import (
	"bytes"
	"fmt"
	"math"
	"math/rand"
	"slices"
	"sort"

	"github.com/memmaker/stimres"
	"github.com/memmaker/voxellib/voxel/nav"
)

// TicksPerSecond is the simulation rate.
// StreetLamp names the big lamp post outside, its light StreetLampHead
// blocks up; other lamps are small. Water puts a street lamp out; it is
// too heavy to knock over.
const (
	StreetLamp     = "Street lamp"
	StreetLampHead = 2.97
)

const TicksPerSecond = 20

// Object kinds.
const (
	KindTorch     = "torch"     // wall torch: lit, can be doused or knocked down
	KindLamp      = "lamp"      // electric lamp (StreetLamp: the big post outside): knock it into water
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
	AmmoRope      = "rope"
	AmmoExplosive = "explosive" // flash bombs
	AmmoTNT       = "tnt"
)

// Object is anything in the world the player or the stimulus system can
// act on. Pos is the floor centre of the cell it stands in (wall torches:
// the cell in front of the wall). What the rules read and write (lit,
// open, burning...) are its properties in the broker (Lit, Open...); the
// Start fields are the mission's values for them.
type Object struct {
	ID, Kind, Name         string
	Pos                    Vec3
	Yaw                    float64
	Value                  int    // loot value
	Key                    string // lock id (doors, chests) or the id a key opens
	Count                  int    // ammo
	StartLocked, StartOpen bool
	StartLit               bool // torches, lamps (powered and on)
	StartRunning           bool // faucets
	// On is what a torch is attached to, like Minecraft: "floor" (the block
	// below) or the wall in that direction: north (-Z), south, east (+X),
	// west. Lanterns (Name) stand on floors or hang from the "ceiling".
	// Empty is resolved by TorchOn/LanternOn when the sim starts.
	On        string
	Vel, Prev Vec3  // a thrown torch in flight: blocks/tick, last tick's Pos
	Flight    int   // ticks in the air; 0 at rest
	Hard      bool  // thrown at full windup: bashes the first thing it hits
	Gone      bool  // taken or destroyed
	Pick      int   // lockpicking progress
	Hidden    []int // NPC indices of bodies stored inside
	LightCell Cell  // where its light block is, when lit
	HasLight  bool
	R         stimres.ID // in the broker
	b         *Broker
}

// is reads property prop; a mission's object, in no sim, has its Start values.
func (o *Object) is(prop string) bool {
	if o.b == nil {
		return map[string]bool{"lit": o.StartLit, "open": o.StartOpen, "locked": o.StartLocked, "running": o.StartRunning}[prop]
	}
	return o.b.Bool(stimres.Object[Cell](o.R), prop)
}

func (o *Object) Lit() bool     { return o.is("lit") }     // torches, lamps (powered and on)
func (o *Object) Open() bool    { return o.is("open") }    // doors, chests
func (o *Object) Locked() bool  { return o.is("locked") }  // doors, chests
func (o *Object) Running() bool { return o.is("running") } // faucets
func (o *Object) Fallen() bool  { return o.is("fallen") }  // a torch or lamp lying on the floor
func (o *Object) Held() bool    { return o.is("held") }    // torch in the player's hand
func (o *Object) Burnt() bool   { return o.is("burnt") }

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
func (o *Object) Burning() bool { return o.is("burning") }

// Player is the player's state as the game reports it each tick, plus
// what the simulation tracks for them.
type Player struct {
	Pos         Vec3 // feet
	Yaw, Pitch  float64
	Crouch, Run bool
	OnGround    bool
	Carrying    int // NPC index, -1 = none
	Keys        []string
	Ammo        map[string]int
	Loot        int
	R           stimres.ID // in the broker
	b           *Broker
	walk        float64
	last        Vec3
	lastInit    bool
}

func (p *Player) r() Receiver { return stimres.Object[Cell](p.R) }

// Health is what the player has left of 20.
func (p *Player) Health() int { return int(p.b.Float(p.r(), "health")) }

// Dead reports whether the player died.
func (p *Player) Dead() bool { return p.b.Bool(p.r(), "dead") }

// Blind is how many ticks the player has left dazzled by a flash (the
// screen whites out).
func (p *Player) Blind() int { return p.b.Left(p.r(), "blind") }

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
	EvFlash                    // a flash bomb went off at Pos
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

type bomb struct {
	Pos, Prev, Vel Vec3 // Prev and Vel while Flying
	Fuse           int
	Flash          bool // a flash bomb, else TNT
	Flying, Hard   bool // thrown and still in the air (fly); thrown hard
	Flight         int  // ticks in the air, for its spin
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
	Liquid      map[Cell]Stim // puddles (feet cells): water or oil; whether live or alight is in the broker
	Spills      []spill
	Bombs       []bomb
	Noisemakers []noisemaker
	Ropes       []Rope
	Outcome     Outcome
	FailReason  string
	Stats       Stats
	Visibility  float64      // player's light gem, 0..1
	Done        map[int]bool // objective index → complete
	Seen        bool         // ever fought
	Broker      []byte       // the broker's state (Snapshot)
}

// Sim is a running mission.
type Sim struct {
	State
	W       World
	Mission *Mission
	Rooms   *RoomMap
	Events  []Event
	B       *Broker // every stimulus goes through it (stim.go)

	doors  map[Cell]int // door cell → object index
	decor  map[Cell]bool
	byID   map[stimres.ID]int // broker ID -> object index, or -NPC-1
	navs   []*nav.Graph
	rng    *rand.Rand
	reroom bool // blocks burnt away: rebuild the rooms
}

// New starts a mission in a world. The world must already hold the
// mission's blocks.
func New(w World, m *Mission) *Sim {
	s := &Sim{W: w, Mission: m, rng: rand.New(rand.NewSource(1))}
	s.Time = m.StartTime
	s.Liquid = map[Cell]Stim{}
	s.Done = map[int]bool{}
	s.B = s.newBroker()
	s.Player = Player{Pos: m.Start, Yaw: m.StartYaw, Carrying: -1, Ammo: map[string]int{}, R: s.B.NewObject("player")}
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
		if c := o.Pos.Cell(); o.Kind == KindTorch && o.On == "floor" && !w.Solid(c.X, c.Y-1, c.Z) {
			if g, ok := s.floorBelow(c, 16); ok { // nothing under it: it stands on the floor below
				o.Pos.Y = float64(g.Y)
			}
		}
		o.R = s.B.NewObject(archetypeOf(&o))
		r := stimres.Object[Cell](o.R)
		for prop, v := range map[string]bool{"lit": o.StartLit, "open": o.StartOpen, "locked": o.StartLocked, "running": o.StartRunning} {
			if v {
				s.B.Set(r, prop, true)
			}
		}
		s.Objects = append(s.Objects, &o)
	}
	for _, d := range m.NPCs {
		n := newNPC(d)
		n.R = s.B.NewObject(npcArchetype(n.Kind))
		s.B.Set(stimres.Object[Cell](n.R), "health", npcHealth)
		s.NPCs = append(s.NPCs, n)
	}
	s.init()
	for i, o := range s.Objects {
		switch o.Kind {
		case KindTorch, KindLamp:
			s.setLight(i, o.Lit())
		case KindDoor:
			s.setDoor(i, o.Open())
		}
	}
	return s
}

// init builds the derived, unsaved structures.
func (s *Sim) init() {
	s.doors, s.decor = map[Cell]int{}, map[Cell]bool{}
	s.byID = map[stimres.ID]int{}
	s.Player.b = s.B
	for k, n := range s.NPCs {
		n.b, s.byID[n.R] = s.B, -k-1
	}
	for i, o := range s.Objects {
		o.b, s.byID[o.R] = s.B, i
		if o.Kind == KindDoor && !o.Gone {
			for _, c := range o.doorCells() {
				s.doors[c] = i
			}
		}
		if o.Kind == KindDecor && !o.Gone {
			s.decor[o.Pos.Cell()] = true
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

// Snapshot is the state to save (quicksave), the broker's included.
func (s *Sim) Snapshot() State {
	var buf bytes.Buffer
	if err := s.B.Save(&buf, codec{}); err != nil {
		panic(err) // our own types: cannot fail
	}
	st := s.State
	st.Broker = buf.Bytes()
	return st
}

// Restore replaces the state with a saved one (quickload). The world must
// already hold the saved blocks.
func (s *Sim) Restore(st State) error {
	s.State = st
	if s.Liquid == nil {
		s.Liquid = map[Cell]Stim{}
	}
	if s.Done == nil {
		s.Done = map[int]bool{}
	}
	s.Events = nil
	s.init()
	return s.B.Restore(bytes.NewReader(st.Broker), codec{})
}

// codec writes cells and positions into broker saves.
type codec struct{}

func (codec) EncodeAddr(c Cell) string         { return fmtCell(c) }
func (codec) EncodePos(p Vec3) string          { return fmtVec(p) }
func (codec) DecodePos(t string) (Vec3, error) { return parseVec(t) }
func (codec) DecodeAddr(t string) (Cell, error) {
	v, err := parseVec(t)
	return Cell{int(v.X), int(v.Y), int(v.Z)}, err
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

// emit makes a sound: the game plays it, NPCs hear it (but not their own).
func (s *Sim) emit(pos Vec3, loud float64, kind string, byNPC int) {
	st := Stimulus{Stim: StimNoise, Origin: pos, Medium: "sound", Intensity: loud, Extra: map[string]any{"sound": kind}}
	if byNPC >= 0 {
		r := s.npcR(byNPC)
		st.Source = &r
	}
	s.Stimulate(st)
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
	s.B.Step() // timed properties run out, sources emit (sight among them)
	s.tickSpills()
	s.tickFire()
	s.tickBombs()
	for i := range s.NPCs {
		s.tickNPC(i)
	}
	s.checkObjectives()
}

// FloorSurface is the footstep surface under someone standing at pos.
func (s *Sim) FloorSurface(pos Vec3) string {
	if s.liquidAt(pos.Cell()) != "" {
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
		if o.Lit() && (!o.HasLight || o.LightCell != o.Pos.Add(Vec3{0, 0.5, 0}).Cell()) {
			s.setLight(i, true)
		}
	}
	s.Visibility = s.visibility()
	if p.Carrying >= 0 {
		n := s.NPCs[p.Carrying]
		n.Body.Teleport(p.Pos.X, p.Pos.Y, p.Pos.Z)
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
	if p.Dead() {
		return
	}
	h := max(0, p.Health()-dmg)
	s.B.Set(p.r(), "health", h)
	s.event(Event{Kind: EvHurt, Loud: float64(dmg)})
	if h == 0 {
		s.B.Set(p.r(), "dead", true)
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
		o.HasLight = false
		s.unlight(o.LightCell)
	}
	if !on {
		return
	}
	c := o.Pos.Cell()
	if (o.Kind == KindTorch || o.Kind == KindLamp) && !o.Fallen() {
		c = o.Pos.Add(Vec3{0, 0.5, 0}).Cell()
	}
	if o.Kind == KindLamp && o.Name == StreetLamp && !o.Fallen() { // the post's head
		c = o.Pos.Add(Vec3{Y: StreetLampHead}).Cell()
	}
	if b := s.W.Block(c.X, c.Y, c.Z); b == BlockAir || b == BlockLight {
		s.W.SetBlock(c.X, c.Y, c.Z, BlockLight)
		o.LightCell, o.HasLight = c, true
	}
}

// unlight clears the light block at c unless another light still burns
// there (a torch lying on the rug it set alight, oil fire).
func (s *Sim) unlight(c Cell) {
	for _, o := range s.Objects {
		if o.HasLight && o.LightCell == c && !o.Gone {
			return
		}
	}
	if _, burning := s.fireAt(c); burning || s.OilBurning(c.Add(0, -1, 0)) {
		return
	}
	if s.W.Block(c.X, c.Y, c.Z) == BlockLight {
		s.W.SetBlock(c.X, c.Y, c.Z, BlockAir)
	}
}

// setDoor swings a door open or shut. The door lies in the plane of the
// wall around it: walls east and west hang it across X, else across Z.
func (s *Sim) setDoor(i int, open bool) {
	o := s.Objects[i]
	s.B.Set(s.objR(i), "open", open)
	c := o.doorCells()[0]
	facing := "east"
	if s.W.Solid(c.X-1, c.Y, c.Z) && s.W.Solid(c.X+1, c.Y, c.Z) {
		facing = "south"
	}
	for k, c := range o.doorCells() {
		half := "lower"
		if k == 1 {
			half = "upper"
		}
		s.W.SetBlock(c.X, c.Y, c.Z, fmt.Sprintf("%s[facing=%s,half=%s,hinge=left,open=%t,powered=false]", BlockDoor, facing, half, open))
	}
}

// DoorAt returns the door object index at a cell, or -1.
func (s *Sim) DoorAt(c Cell) int {
	if i, ok := s.doors[c]; ok {
		return i
	}
	return -1
}

// tickObjects: thrown torches in flight, noisemakers. (Fire, faucets and
// what lies in water are the broker's sources: stimuli.toml.)
func (s *Sim) tickObjects() {
	for i, o := range s.Objects {
		if !o.Gone && o.Flight > 0 {
			s.flyTorch(i)
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

// floorBelow finds where an object dropped at c comes to rest: the first
// cell at or below it with a solid block under it. Unlike groundBelow,
// door cells count (an object can lie in a doorway).
func (s *Sim) floorBelow(c Cell, depth int) (Cell, bool) {
	for i := 0; i <= depth; i++ {
		if !s.W.Solid(c.X, c.Y, c.Z) && s.W.Solid(c.X, c.Y-1, c.Z) {
			return c, true
		}
		c.Y--
	}
	return c, false
}

func (s *Sim) tickBombs() {
	for i := 0; i < len(s.Bombs); i++ {
		b := &s.Bombs[i]
		if b.Flying {
			b.Flight++
			if s.fly(&b.Pos, &b.Prev, &b.Vel, &b.Hard) {
				b.Flying = false
			}
		}
		if b.Fuse--; b.Fuse%10 == 0 && b.Fuse > 0 {
			s.emit(b.Pos, 18, "fuse", -1)
		}
		switch {
		case b.Fuse > 0:
		case b.Flash:
			s.Flash(b.Pos)
		default:
			s.Explode(b.Pos)
		}
	}
	s.Bombs = slices.DeleteFunc(s.Bombs, func(b bomb) bool { return b.Fuse <= 0 })
}

// Flash bursts a flash bomb at pos: a blinding light (StimFlash) for
// whoever sees it and a sharp bang. Nothing breaks or burns.
func (s *Sim) Flash(pos Vec3) {
	pos = pos.Add(Vec3{Y: 0.5})
	s.Stimulate(at(StimFlash, pos, 14, 8*TicksPerSecond))
	s.emit(pos, 45, "flash", -1)
	s.event(Event{Kind: EvFlash, Pos: pos})
}

// Explode is TNT going off: its blast destroys blocks (blastBlock), objects
// and people around pos.
func (s *Sim) Explode(pos Vec3) {
	s.Stimulate(at(StimBlast, pos, 3, 1))
	s.Stimulate(at(StimFire, pos, 4.5, 0))
	s.emit(pos, 95, "explosion", -1)
	s.event(Event{Kind: EvBlast, Pos: pos})
	s.init() // doors and walls changed: rebuild the room graph
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
		if !o.Fallen() {
			dx, dz := o.WallDir() // a wall torch hangs 0.5 toward its wall
			p = p.Add(Vec3{0.3 * float64(dx), 0, 0.3 * float64(dz)})
			return p.Add(Vec3{-0.25, 0.1, -0.25}), p.Add(Vec3{0.25, 0.9, 0.25})
		}
		return p.Add(Vec3{-0.4, 0, -0.4}), p.Add(Vec3{0.4, 0.25, 0.4})
	case KindLamp:
		if o.Fallen() {
			return p.Add(Vec3{-0.5, 0, -0.5}), p.Add(Vec3{0.5, 0.35, 0.5})
		}
		if o.Name == StreetLamp {
			return p.Add(Vec3{-0.3, 0, -0.3}), p.Add(Vec3{0.3, 3.2, 0.3})
		}
		return p.Add(Vec3{-0.25, 0, -0.25}), p.Add(Vec3{0.25, 0.8, 0.25})
	case KindChest:
		return p.Add(Vec3{-0.45, 0, -0.45}), p.Add(Vec3{0.45, 0.8, 0.45})
	case KindFlammable:
		return p.Add(Vec3{-0.5, 0, -0.5}), p.Add(Vec3{0.5, 0.15, 0.5})
	case KindFaucet:
		return p.Add(Vec3{-0.35, 0, -0.35}), p.Add(Vec3{0.35, 1.3, 0.35})
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
		if o.Gone || o.Held() || o.Kind == KindDecor {
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
		case o.Locked() && s.hasKey(o.Key):
			return name + " (locked - you have the key)"
		case o.Locked():
			return name + " (locked - hold E to pick)"
		case o.Open():
			return name + " (close)"
		}
		return name + " (open)"
	case KindLoot:
		return name + " (" + itoa(o.Value) + ")"
	case KindChest:
		switch {
		case s.Player.Carrying >= 0:
			return name + " (hide body)"
		case o.Locked() && !s.hasKey(o.Key):
			return name + " (locked - hold E to pick)"
		case o.Value > 0:
			return name + " (" + itoa(o.Value) + " inside)"
		}
		return name + " (empty)"
	case KindTorch, KindLamp:
		if o.Name != StreetLamp && s.HeldTorch() < 0 {
			return name + " (take)"
		}
	case KindFaucet:
		if o.Running() {
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

// Frob uses the target: a frob stimulus to it (or, with nothing in reach,
// to the body on the player's shoulder: drop it). held is true on the
// ticks after the first while the use key stays down (lockpicking).
func (s *Sim) Frob(t Target, held bool) {
	var r Receiver
	switch {
	case t.NPC >= 0:
		r = s.npcR(t.NPC)
	case t.Obj >= 0:
		r = s.objR(t.Obj)
	case s.Player.Carrying >= 0:
		r = s.npcR(s.Player.Carrying)
	default:
		return
	}
	pr := s.playerR()
	s.B.Stimulate(Stimulus{Stim: StimFrob, Intensity: 1, Target: &r, Source: &pr, Origin: s.Player.Pos, Extra: map[string]any{"held": held}})
}

// Frob responses (stimuli.toml).

// pickLock: the key opens it, else held use picks it. An NPC only comes
// to doors it has the key of (newNav) and goes through.
func (s *Sim) pickLock(i int, d Delivery, _ []string) bool {
	o := s.Objects[i]
	if d.St.Source != nil && s.npcIndex(*d.St.Source) >= 0 {
		return o.Kind == KindDoor && !o.Open() && s.openDoor(i, d, nil)
	}
	if held, _ := d.St.Extra["held"].(bool); s.hasKey(o.Key) {
		if !held {
			s.B.Set(s.objR(i), "locked", false)
			s.emit(o.Pos, 12, "unlock", -1)
			s.message("Unlocked with the key.")
		}
		return true
	}
	o.Pick++
	if o.Pick%10 == 1 {
		s.emit(o.Pos, 14, "lockpick", -1)
	}
	if o.Pick >= 3*TicksPerSecond {
		s.B.Set(s.objR(i), "locked", false)
		s.emit(o.Pos, 14, "unlock", -1)
		s.message("Lock picked.")
	}
	return true
}

// openDoor swings a door; whoever frobbed it makes the noise.
func (s *Sim) openDoor(i int, d Delivery, _ []string) bool {
	o := s.Objects[i]
	if o.Open() && s.doorBlocked(i) {
		return false
	}
	by := -1
	if d.St.Source != nil {
		by = s.npcIndex(*d.St.Source)
	}
	s.setDoor(i, !o.Open())
	s.emit(o.Pos.Add(Vec3{0, 1, 0}), 28, "door", by)
	return true
}

// frobChest hides the carried body in it, else unlocks, loots or opens it.
func (s *Sim) frobChest(i int, d Delivery, _ []string) bool {
	o, p, r := s.Objects[i], &s.Player, s.objR(i)
	held, _ := d.St.Extra["held"].(bool)
	if p.Carrying >= 0 {
		if held {
			return false
		}
		n := s.NPCs[p.Carrying]
		n.Hidden, n.CarriedBy = true, false
		o.Hidden = append(o.Hidden, p.Carrying)
		p.Carrying = -1
		s.B.Set(r, "open", true)
		s.emit(o.Pos, 16, "body", -1)
		s.message("Body hidden.")
		return true
	}
	if o.Locked() {
		return s.pickLock(i, d, nil)
	}
	if held {
		return false
	}
	if o.Value > 0 {
		p.Loot += o.Value
		s.message("Took " + itoa(o.Value) + " in valuables")
		o.Value = 0
		s.B.Set(r, "open", true)
		s.emit(o.Pos, 8, "loot", -1)
	} else {
		s.B.Set(r, "open", !o.Open())
		s.emit(o.Pos, 6, "door", -1)
	}
	return true
}

// take picks up loot, a key or ammo.
func (s *Sim) take(i int, _ Delivery, _ []string) bool {
	o, p := s.Objects[i], &s.Player
	switch o.Kind {
	case KindLoot:
		p.Loot += o.Value
		s.emit(o.Pos, 8, "loot", -1)
		s.message("Took " + o.Name + " (" + itoa(o.Value) + ")")
	case KindKey:
		p.Keys = append(p.Keys, o.Key)
		s.message("Took " + o.Name)
	case KindAmmo:
		p.Ammo[o.Name] += o.Count
		s.message("Took " + itoa(o.Count) + " " + o.Name)
	}
	s.B.Destroy(s.objR(i))
	return true
}

// takeTorch takes a torch or small lamp in hand (one at a time).
func (s *Sim) takeTorch(i int, _ Delivery, _ []string) bool {
	if s.HeldTorch() >= 0 {
		return false
	}
	o, r := s.Objects[i], s.objR(i)
	s.B.Set(r, "held", true)
	s.B.Set(r, "fallen", false)
	s.B.Set(r, "flying", false)
	o.Flight = 0
	s.emit(o.Pos, 6, "swish", -1)
	return true
}

// frobBody picks up an unconscious or dead NPC, or drops the one carried.
func (s *Sim) frobBody(k int, _ Delivery, _ []string) bool {
	if s.Player.Carrying == k {
		s.DropBody()
		return true
	}
	s.pickUpBody(k)
	return true
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
		if !o.Gone && o.Held() {
			return i
		}
	}
	return -1
}

// ThrowTorch lobs the held torch along the view about dist blocks
// (0 drops it). It tumbles through the air (flyTorch) and, where it lands,
// clatters, which draws guards over, and sets flammables alight; thrown
// hard, it bashes the first thing it hits.
func (s *Sim) ThrowTorch(eye, dir Vec3, dist float64, hard bool) {
	i := s.HeldTorch()
	if i < 0 {
		return
	}
	o, r := s.Objects[i], s.objR(i)
	s.B.Set(r, "held", false)
	s.B.Set(r, "fallen", true)
	s.B.Set(r, "flying", true)
	o.Flight, o.Hard = 1, hard
	o.Yaw = math.Atan2(dir.X, -dir.Z)
	o.Pos, o.Vel = s.launch(eye, dir, dist)
	o.Prev = o.Pos
	s.setLight(i, o.Lit())
}

// launch is where and how fast something thrown dist blocks along dir
// leaves the hand: a little ahead of the eye unless that is in a wall.
func (s *Sim) launch(eye, dir Vec3, dist float64) (pos, vel Vec3) {
	pos = eye
	if c := eye.Add(dir.Scale(0.4)).Cell(); !s.W.Solid(c.X, c.Y, c.Z) {
		pos = eye.Add(dir.Scale(0.4))
	}
	return pos, dir.Scale(dist / 12).Add(Vec3{Y: 0.1 * min(dist, 1)})
}

// torchGravity matches the arrows' (projectile.Gravity), blocks/tick².
const torchGravity = 0.05

// fly moves something thrown one tick: it glances off walls and ceilings
// and comes to rest on the first floor it falls onto. Thrown hard, its
// first impact, on an NPC or decoration in its path or a block, bashes.
// It reports whether it landed this tick.
func (s *Sim) fly(pos, prev, vel *Vec3, hard *bool) bool {
	*prev = *pos
	*vel = vel.Add(Vec3{Y: -torchGravity}).Scale(0.99)
	impact := func(p Vec3) {
		if *hard {
			*hard = false
			s.Stimulate(at(StimBash, p, 0, 1))
		}
	}
	if *hard && s.Stimulate(at(StimBash, *pos, 0, 1)) { // struck someone or something: drops off it
		*hard = false
		vel.X, vel.Z = vel.X*-0.3, vel.Z*-0.3
	}
	solid := func(p Vec3) bool { c := p.Cell(); return s.W.Solid(c.X, c.Y, c.Z) }
	if n := pos.Add(Vec3{X: vel.X}); solid(n) {
		impact(n)
		vel.X *= -0.3
	} else {
		*pos = n
	}
	if n := pos.Add(Vec3{Z: vel.Z}); solid(n) {
		impact(n)
		vel.Z *= -0.3
	} else {
		*pos = n
	}
	n := pos.Add(Vec3{Y: vel.Y})
	switch {
	case !solid(n):
		*pos = n
	case vel.Y > 0: // ceiling
		impact(n)
		vel.Y = 0
	default: // landed: rest on the block's top
		impact(n)
		pos.Y = math.Floor(n.Y) + 1
		*vel = Vec3{}
		s.emit(*pos, 22, "clatter", -1)
		return true
	}
	return false
}

// flyTorch moves a thrown torch one tick (fly); a lit one landing starts a
// small flame, and a lantern spills its oil first, which that flame lights.
func (s *Sim) flyTorch(i int) {
	o := s.Objects[i]
	o.Flight++
	if s.fly(&o.Pos, &o.Prev, &o.Vel, &o.Hard) {
		o.Flight = 0
		s.B.Set(s.objR(i), "flying", false)
		if o.Kind == KindTorch && o.Name == "Lantern" { // it cracks: its oil runs out over the floor
			s.Stimulate(at(StimOil, o.Pos, 1, 3))
		}
		if o.Lit() && o.Kind == KindTorch { // a small flame: catches rugs and fuel, doesn't burn people
			s.Stimulate(at(StimFire, o.Pos, 1.2, 0.5))
		}
	}
	if o.Lit() && (!o.HasLight || o.LightCell != o.Pos.Cell()) {
		s.setLight(i, true)
	}
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
			if s.reachable(npc) {
				s.hit(s.npcR(npc), Stimulus{Stim: StimPierce, Origin: pos, Intensity: math.Ceil(npcHealth / 4.0), Extra: map[string]any{"how": "shot", "head": head}})
			}
			return
		}
		if !s.Stimulate(at(StimPierce, pos, 0.01, 1)) {
			s.emit(pos, 26, "arrow", -1)
		}
	case AmmoWater:
		s.emit(pos, 18, "splash", -1)
		// The water runs down whatever it hit and pools on the floor below:
		// nine cells, hugging the wall when it struck one.
		s.Stimulate(at(StimWater, pos, 0.9, 5))
	case AmmoNoise:
		s.Noisemakers = append(s.Noisemakers, noisemaker{Pos: pos})
	case AmmoRope:
		if npc < 0 {
			s.emit(pos, 26, "arrow", -1)
		}
	}
}

// Rope hangs from a rope arrow stuck in the terrain.
type Rope struct {
	Tip        Vec3    // the arrow's tip, in the block
	Yaw, Pitch float64 // the arrow's flight
	Top        Vec3    // the nock, where the rope is tied
	Len        float64
}

// AddRope ties a rope to the nock of a rope arrow stuck at tip and lets it
// down to a little above the floor. False when there is no room for one
// (the arrow is in the floor, or its nock in a block).
func (s *Sim) AddRope(tip Vec3, yaw, pitch float64) bool {
	fx, fz := forward(yaw)
	cp := math.Cos(pitch)
	top := tip.Sub(Vec3{fx * cp, math.Sin(pitch), fz * cp}.Scale(0.78)) // arrow length, nock included
	c := top.Cell()
	if s.W.Solid(c.X, c.Y, c.Z) {
		return false
	}
	for y := c.Y - 1; y > c.Y-40; y-- {
		if s.W.Solid(c.X, y, c.Z) {
			if l := top.Y - float64(y+1) - 0.3; l >= 1 {
				s.Ropes = append(s.Ropes, Rope{Tip: tip, Yaw: yaw, Pitch: pitch, Top: top, Len: l})
				return true
			}
			return false
		}
	}
	return false
}

// Throw lobs a flash bomb along the view, flying like a thrown torch (fly)
// at dist/12 blocks a tick; it goes off 3 seconds after leaving the hand,
// wherever it is. Thrown hard, its first impact bashes.
func (s *Sim) Throw(eye, dir Vec3, dist float64, hard bool) bool {
	p := &s.Player
	if p.Ammo[AmmoExplosive] <= 0 {
		return false
	}
	p.Ammo[AmmoExplosive]--
	pos, vel := s.launch(eye, dir, dist)
	s.Bombs = append(s.Bombs, bomb{Pos: pos, Prev: pos, Vel: vel, Fuse: 3 * TicksPerSecond, Flash: true, Flying: true, Hard: hard})
	return true
}

// PlaceTNT sets lit TNT down on the floor in reach along the view; it goes
// off after 4 seconds, like Minecraft's.
func (s *Sim) PlaceTNT(eye, dir Vec3) bool {
	p := &s.Player
	if p.Ammo[AmmoTNT] <= 0 {
		return false
	}
	p.Ammo[AmmoTNT]--
	land := s.landing(eye, dir, 2.5)
	s.Bombs = append(s.Bombs, bomb{Pos: land, Fuse: 4 * TicksPerSecond})
	s.emit(land, 8, "fuse", -1)
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
	if g, ok := s.floorBelow(land.Cell(), 8); ok {
		land = g.Floor()
	}
	return land
}

// Melee swings the blackjack or the blade at the NPC in front of the
// player, else at decoration or a window. It reports whether it hit.
func (s *Sim) Melee(tool string) bool {
	p := &s.Player
	st := Stimulus{Stim: StimBash, Intensity: 1, Medium: "radius"}
	if tool == "blade" {
		st = Stimulus{Stim: StimPierce, Intensity: 4, Medium: "radius", Extra: map[string]any{"how": "stabbed"}}
	}
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
	if best >= 0 {
		n := s.NPCs[best]
		if tool == "blade" {
			s.emit(n.pos(), 24, "stab", -1)
		}
		st.Origin = n.pos()
		return s.hit(s.npcR(best), st)
	}
	s.emit(p.Pos, 6, "swish", -1)
	// Else the swing hits the first decoration or block within reach.
	eye := p.Eye()
	fx, fz := forward(p.Yaw)
	cp := math.Cos(p.Pitch)
	dir := Vec3{fx * cp, math.Sin(p.Pitch), fz * cp}
	for t := 0.2; t < 2.2; t += 0.1 {
		st.Origin = eye.Add(dir.Scale(t))
		c := st.Origin.Cell()
		hit := s.W.Solid(c.X, c.Y, c.Z)
		for i, o := range s.Objects {
			if lo, hi := s.ObjectBox(i); o.Kind == KindDecor && boxDist(st.Origin, lo, hi) == 0 {
				hit = true
			}
		}
		if hit {
			return s.Stimulate(st)
		}
	}
	return false
}

// hurtNPC damages an NPC and makes it fight back or run.
func (s *Sim) hurtNPC(k, dmg int, how string) {
	n := s.NPCs[k]
	if s.B.Add(s.npcR(k), "health", -float64(dmg)); n.Health() <= 0 {
		s.killNPC(k, "player", how)
		return
	}
	s.emit(n.pos(), 30, "pain", k)
	s.alertNPC(k, s.Player.Pos)
}

// killNPC kills an NPC; by is "player" or "accident".
func (s *Sim) killNPC(k int, by, how string) {
	n := s.NPCs[k]
	if n.State() == StateDead {
		return
	}
	n.setState(StateDead)
	s.B.Set(s.npcR(k), "down", true)
	n.Path, n.Death = nil, by
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
				if n.ID == ob.Target && n.State() == StateDead {
					done = true
				}
			}
		case "accident":
			for _, n := range s.NPCs {
				if n.ID == ob.Target && n.State() == StateDead && n.Death == "accident" {
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
