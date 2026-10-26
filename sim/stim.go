package sim

import (
	_ "embed"
	"math"
	"strings"

	"github.com/memmaker/stimres"
)

// Every effect on the world goes through stimres, Dark Engine style:
// whatever acts emits a Stimulus, and objects, NPCs, the player, blocks
// and liquids respond only through the receptrons in stimuli.toml, so
// whatever emits water, fire or noise has the same effect on everything it
// reaches, and effects chain (a pierced barrel's water douses the torch
// beside it). The Go responses below are the bespoke ones the rules call.

//go:embed stimuli.toml
var stimuliTOML string

type (
	Trace    = stimres.Trace[Cell, Vec3]
	Broker   = stimres.Broker[Cell, Vec3]
	Receiver = stimres.Receiver[Cell]
	Stimulus = stimres.Stimulus[Cell, Vec3]
	Delivery = stimres.Delivery[Cell, Vec3]
)

// Stim is a kind of stimulus (stimuli.toml).
type Stim = string

const (
	StimWater  Stim = "water"  // douses fire; Intensity >= 1 pools as a puddle of that many cells
	StimOil    Stim = "oil"    // pools as a burnable puddle
	StimFire   Stim = "fire"   // ignites; > 0 is open flame (catches blocks), >= 1 flames that burn people
	StimShock  Stim = "shock"  // electricity: live in water, kills
	StimPierce Stim = "pierce" // broadheads, the blade (Intensity: damage)
	StimBash   Stim = "bash"   // the blackjack
	StimKick   Stim = "kick"   // the player's kick (Extra "yaw": the way it sends things)
	StimSlam   Stim = "slam"   // a thrown body striking something (Intensity: m/s; Source: the body)
	StimFall   Stim = "fall"   // a body landing (Intensity: blocks fallen)
	StimBlast  Stim = "blast"  // explosions
	StimFlash  Stim = "flash"  // flash bombs: blinds whoever sees it (Intensity: ticks)
	StimNoise  Stim = "noise"  // sound through the room graph (Intensity: loudness, Extra "sound")
	StimFrob   Stim = "frob"   // deliberate use (Extra "held": the key is held down; "revive": a comrade's shake)
	StimStep   Stim = "step"   // a footstep on a cell (Intensity: the stride's loudness factor)
	StimMop    Stim = "mop"    // wiping up a puddle cell
)

// Mediums: "radius" reaches everything whose box is within Range of the
// Origin, "sound" every NPC that can hear (the room graph attenuates it
// in hear), "sight" every NPC that can look.

// newBroker makes the sim's broker with its mediums and responses.
func (s *Sim) newBroker() *Broker {
	b := stimres.New[Cell, Vec3](simWorld{s})
	b.Medium("radius", stimres.MediumFunc[Cell, Vec3](s.reachRadius))
	b.Medium("sound", stimres.MediumFunc[Cell, Vec3](s.reachSound))
	b.Medium("sight", stimres.MediumFunc[Cell, Vec3](s.reachSight))
	for name, r := range responses {
		b.Respond(name, func(b *Broker, d Delivery, args []string) bool { return r(s, d, args) })
	}
	if err := b.Load(strings.NewReader(stimuliTOML)); err != nil {
		panic("stimuli.toml: " + err.Error())
	}
	return b
}

// Stimulate delivers st now and reports whether anything reacted to it.
func (s *Sim) Stimulate(st Stimulus) bool { return len(s.B.Stimulate(st)) > 0 }

// at is a radius stimulus at pos.
func at(stim Stim, pos Vec3, radius, intensity float64) Stimulus {
	return Stimulus{Stim: stim, Origin: pos, Medium: "radius", Range: radius, Intensity: intensity}
}

// hit delivers st to one receiver alone (an arrow in a body).
func (s *Sim) hit(r Receiver, st Stimulus) bool {
	st.Target = &r
	return s.Stimulate(st)
}

// Receivers.

func (s *Sim) objR(i int) Receiver { return stimres.Object[Cell](s.Objects[i].R) }
func (s *Sim) npcR(k int) Receiver { return stimres.Object[Cell](s.NPCs[k].R) }
func (s *Sim) playerR() Receiver   { return stimres.Object[Cell](s.Player.R) }
func cellR(c Cell) Receiver        { return stimres.At(c) }

// objIndex and npcIndex find the object or NPC behind a receiver (-1: not one).
func (s *Sim) objIndex(r Receiver) int {
	if i, ok := s.byID[r.Obj]; ok && r.Obj != 0 && i >= 0 {
		return i
	}
	return -1
}

func (s *Sim) npcIndex(r Receiver) int {
	if k, ok := s.byID[r.Obj]; ok && r.Obj != 0 && k < 0 {
		return -k - 1
	}
	return -1
}

// archetypeOf is an object's archetype: its Kind, or a more specific one
// by Name (decoration such as barrels, lanterns, the street lamp).
func archetypeOf(o *Object) string {
	switch {
	case o.Kind == KindTorch && o.Name == "Lantern":
		return "lantern"
	case o.Kind == KindLamp && o.Name == StreetLamp:
		return "street_lamp"
	case o.Kind == KindDecor && decorArch[o.Name] != "":
		return decorArch[o.Name]
	}
	return o.Kind
}

// npcArchetype is an NPC's archetype by Kind; other kinds are plain NPCs.
func npcArchetype(kind string) string {
	switch kind {
	case "guard", "civilian", "target":
		return kind
	}
	return "npc"
}

// decorArch are the decorations that do something, by Name.
var decorArch = map[string]string{"Barrel": "barrel", "Oil barrel": "oil_barrel"}

// simWorld is the sim as stimres sees it.
type simWorld struct{ s *Sim }

func (w simWorld) ArchetypeAt(c Cell) string {
	s := w.s
	if l := s.Liquid[c]; l != "" {
		return l + "_puddle"
	}
	if s.supportOf(c) >= 0 {
		return "support"
	}
	switch b := s.W.Block(c.X, c.Y, c.Z); {
	case IsGlass(b):
		return "glass"
	case b == BlockAir || b == BlockLight:
		return "air"
	case b == BlockTNT:
		return "tnt"
	case IsSpikes(b):
		return "spikes"
	case s.burnable(c) && (strings.Contains(b, "_stairs") || IsLadder(b)):
		return "creaky"
	case s.burnable(c):
		return "flammable_block"
	}
	return "solid"
}

func (w simWorld) Locate(r Receiver) Vec3 {
	s := w.s
	if r.Obj == 0 {
		return r.Addr.Floor().Add(Vec3{Y: 0.1})
	}
	if r.Obj == s.Player.R {
		return s.Player.Pos
	}
	if k := s.npcIndex(r); k >= 0 {
		return s.NPCs[k].pos()
	}
	return s.Objects[s.objIndex(r)].Pos
}

func (w simWorld) Spawned(stimres.ID, Vec3) {}

// Destroyed takes what the destroy effect removed out of the world.
func (w simWorld) Destroyed(r Receiver) {
	s := w.s
	if r.Obj == 0 {
		c := r.Addr
		switch w.ArchetypeAt(c) {
		case "oil_puddle": // burnt away
			delete(s.Liquid, c)
			s.unlightAbove(c)
			if s.W.Block(c.X, c.Y, c.Z) == BlockOil {
				s.W.SetBlock(c.X, c.Y, c.Z, BlockAir)
			}
		case "water_puddle":
			delete(s.Liquid, c)
			if s.W.Block(c.X, c.Y, c.Z) == BlockPuddle {
				s.W.SetBlock(c.X, c.Y, c.Z, BlockAir)
			}
		default: // a shattered window is an opening now: rebuild the room graph
			s.W.SetBlock(c.X, c.Y, c.Z, BlockAir)
			s.init()
		}
		return
	}
	i := s.objIndex(r)
	if i < 0 {
		return
	}
	o := s.Objects[i]
	s.setLight(i, false)
	if o.Kind == KindChest { // what was hidden in it lies in the ashes
		for _, k := range o.Hidden {
			n := s.NPCs[k]
			n.Hidden = false
			n.Body.Teleport(o.Pos.X, o.Pos.Y, o.Pos.Z)
		}
		o.Hidden = nil
	}
	if o.Kind == KindDoor { // it leaves its opening
		for _, dc := range o.doorCells() {
			s.W.SetBlock(dc.X, dc.Y, dc.Z, BlockAir)
			delete(s.doors, dc)
		}
	}
	o.Gone = true
}

// Mediums.

func (s *Sim) reachRadius(st Stimulus, yield func(Receiver, float64)) {
	p, r := st.Origin, st.Range
	for i, o := range s.Objects {
		if lo, hi := s.ObjectBox(i); !o.Gone && boxDist(p, lo, hi) <= r {
			yield(s.objR(i), boxDist(p, lo, hi))
		}
	}
	for k, n := range s.NPCs {
		b := n.Body.Box()
		if d := boxDist(p, Vec3{b.MinX, b.MinY, b.MinZ}, Vec3{b.MaxX, b.MaxY, b.MaxZ}); d <= r && s.reachable(k) {
			yield(s.npcR(k), d)
		}
	}
	if pp := s.Player.Pos; !s.Player.Dead() {
		if d := boxDist(p, pp.Sub(Vec3{0.3, 0, 0.3}), pp.Add(Vec3{0.3, 1.8, 0.3})); d <= r {
			yield(s.playerR(), d)
		}
	}
	lo, hi := p.Sub(Vec3{r, r, r}).Cell(), p.Add(Vec3{r, r, r}).Cell()
	for x := lo.X; x <= hi.X; x++ {
		for y := lo.Y; y <= hi.Y; y++ {
			for z := lo.Z; z <= hi.Z; z++ {
				if d := boxDist(p, Vec3{float64(x), float64(y), float64(z)}, Vec3{float64(x + 1), float64(y + 1), float64(z + 1)}); d <= r {
					yield(cellR(Cell{x, y, z}), d)
				}
			}
		}
	}
}

// reachSound plays the sound and reaches every NPC but its maker; hear
// works out how loud it is where they stand.
func (s *Sim) reachSound(st Stimulus, yield func(Receiver, float64)) {
	by := -1
	if st.Source != nil {
		by = s.npcIndex(*st.Source)
	}
	kind, _ := st.Extra["sound"].(string)
	s.event(Event{Kind: EvSound, Sound: kind, Pos: st.Origin, Loud: st.Intensity, NPC: by})
	f := s.Rooms.spread(st.Origin, s.doorClosed) // once for every listener
	for k := range s.NPCs {
		if k != by && s.reachable(k) && s.NPCs[k].Blind() == 0 {
			if d, from := s.Rooms.pathLen(f, s.NPCs[k].eye()); d*SoundFalloff < st.Intensity {
				s.NPCs[k].heardFrom = from // for hearNoise, which runs right after this
				yield(s.npcR(k), d)
			}
		}
	}
}

// reachSight reaches every NPC that is looking (but its source); the
// receptron decides what it makes out.
func (s *Sim) reachSight(st Stimulus, yield func(Receiver, float64)) {
	for k, n := range s.NPCs {
		if s.reachable(k) && n.Blind() == 0 && (st.Source == nil || *st.Source != s.npcR(k)) {
			yield(s.npcR(k), n.eye().Dist(st.Origin))
		}
	}
}

// reachable reports whether stimuli reach NPC k: conscious and out in the open.
func (s *Sim) reachable(k int) bool {
	n := s.NPCs[k]
	return n.alive() && !n.Hidden && !n.CarriedBy
}

// boxDist is how far p is from the box lo..hi (0 inside).
func boxDist(p, lo, hi Vec3) float64 {
	d := p.Sub(Vec3{clamp(p.X, lo.X, hi.X), clamp(p.Y, lo.Y, hi.Y), clamp(p.Z, lo.Z, hi.Z)})
	return math.Sqrt(d.X*d.X + d.Y*d.Y + d.Z*d.Z)
}

// Responses: the Go side of stimuli.toml's "call" effects.
type response func(s *Sim, d Delivery, args []string) bool

var responses map[string]response

func init() { // in init: responses stimulate in turn
	obj := func(f func(s *Sim, i int, d Delivery, args []string) bool) response {
		return func(s *Sim, d Delivery, args []string) bool { return f(s, s.objIndex(d.To), d, args) }
	}
	npc := func(f func(s *Sim, k int, d Delivery, args []string) bool) response {
		return func(s *Sim, d Delivery, args []string) bool { return f(s, s.npcIndex(d.To), d, args) }
	}
	cell := func(f func(s *Sim, c Cell, d Delivery) bool) response {
		return func(s *Sim, d Delivery, _ []string) bool { return f(s, d.To.Addr, d) }
	}
	responses = map[string]response{
		"douse":       obj((*Sim).douse),
		"kindle":      obj((*Sim).kindle),
		"light_off":   obj(func(s *Sim, i int, _ Delivery, _ []string) bool { s.setLight(i, false); return true }),
		"short_out":   obj((*Sim).shortOut),
		"take_torch":  obj((*Sim).takeTorch),
		"pick_lock":   obj((*Sim).pickLock),
		"open_door":   obj((*Sim).openDoor),
		"frob_chest":  obj((*Sim).frobChest),
		"take":        obj((*Sim).take),
		"take_weapon": obj((*Sim).takeWeapon),
		"toggle": func(s *Sim, d Delivery, args []string) bool {
			s.B.Set(d.To, args[0], !s.B.Bool(d.To, args[0]))
			return true
		},
		"spill":     (*Sim).spillFrom,
		"wound":     npc((*Sim).wound),
		"knock_out": npc((*Sim).knockOut),
		"kill": npc(func(s *Sim, k int, _ Delivery, args []string) bool {
			s.killNPC(k, args[0], strings.Join(args[1:], " "))
			return true
		}),
		"hear":        npc((*Sim).hearNoise),
		"blind":       npc((*Sim).blindNPC),
		"see":         npc((*Sim).see),
		"notice_body": npc((*Sim).noticeBody),
		"notice_fire": npc((*Sim).noticeFire),
		"blast_block": cell(blastBlock),
		"shove":       npc((*Sim).shove),
		"pin":         npc((*Sim).pin),
		"impale":      impale,
		"strike": func(s *Sim, d Delivery, _ []string) bool {
			why, _ := d.St.Extra["why"].(string)
			s.HurtPlayer(int(d.Intensity), why)
			return true
		},
		"frob_body": npc((*Sim).frobBody),
		"recover":   npc(func(s *Sim, k int, _ Delivery, _ []string) bool { s.recover(k); return true }),
		"hurt": func(s *Sim, _ Delivery, args []string) bool {
			s.HurtPlayer(atoi(args[0]), strings.Join(args[1:], " "))
			return true
		},
		"fall_player": func(s *Sim, d Delivery, _ []string) bool { // Minecraft: a point per block past three
			s.HurtPlayer(int(math.Ceil(d.Intensity-3)), "Fell to your death.")
			return true
		},
		"dazzle": (*Sim).dazzlePlayer,
		"caught_in_blast": func(s *Sim, d Delivery, _ []string) bool {
			s.HurtPlayer(max(1, int(24-d.St.Origin.Dist(s.Player.Pos)*5)), "Caught in the blast.")
			return true
		},
		"pour": func(s *Sim, d Delivery, _ []string) bool {
			if d.To.Addr != d.St.Origin.Add(Vec3{0, 0.3, 0}).Cell() { // it pools where it lands, not all over
				return false
			}
			s.pourOut(d.St.Stim, d.To.Addr, int(d.St.Intensity))
			return true
		},
		"douse_fire":    cell(douseFire),
		"catch_fire":    cell(catchFire),
		"burn_block":    cell(burnBlock),
		"prime_tnt":     cell(primeTNT),
		"fire_tick":     cell(fireTick),
		"seep":          cell(seep),
		"electrify":     cell(func(s *Sim, c Cell, _ Delivery) bool { s.electrify(c); return true }),
		"ignite_oil":    cell((*Sim).igniteOil),
		"unlight_above": cell(func(s *Sim, c Cell, _ Delivery) bool { s.unlightAbove(c); return true }),
	}
	addReactions()
}

// Object responses.

// douse puts out a lit torch or a fire.
func (s *Sim) douse(i int, _ Delivery, _ []string) bool {
	o, r := s.Objects[i], s.objR(i)
	s.B.Set(r, "lit", false)
	s.B.Clear(r, "burning")
	s.B.Clear(r, "smoulder")
	s.setLight(i, false)
	s.emit(o.Pos, 12, "hiss", -1)
	return true
}

// kindle lights a torch or a flammable's flames.
func (s *Sim) kindle(i int, _ Delivery, _ []string) bool {
	o := s.Objects[i]
	if o.Kind == KindTorch {
		s.B.Set(s.objR(i), "lit", true)
	}
	s.setLight(i, true)
	s.emit(o.Pos, 35, "ignite", -1)
	return true
}

// shortOut is water reaching a lit lamp. A street lamp's head fizzles out
// in a shower of sparks. A small lamp dropped or thrown dies and its current goes into the
// water. (A small lamp standing or in hand is high and dry.)
func (s *Sim) shortOut(i int, _ Delivery, _ []string) bool {
	o, r := s.Objects[i], s.objR(i)
	if o.Name == StreetLamp {
		s.B.Set(r, "lit", false)
		s.setLight(i, false)
		s.emit(o.Pos.Add(Vec3{Y: StreetLampHead}), 16, "zap", -1)
		return true
	}
	if !o.Fallen() {
		return false
	}
	s.B.Set(r, "lit", false)
	s.setLight(i, false)
	s.B.Enqueue(at(StimShock, o.Pos, 0.2, 1))
	return true
}

// spillFrom pours a receiver's liquid out around its foot: "spill AMOUNT RADIUS".
func (s *Sim) spillFrom(d Delivery, args []string) bool {
	liquid := s.B.String(d.To, "liquid")
	amount, radius := float64(atoi(args[0])), parseF(args[1])
	s.B.Enqueue(at(liquid, s.B.World.Locate(d.To), radius, amount))
	return true
}

func parseF(s string) float64 {
	var f float64
	for i, part := range strings.SplitN(s, ".", 2) {
		if i == 0 {
			f = float64(atoi(part))
			continue
		}
		f += float64(atoi(part)) / math.Pow(10, float64(len(part)))
	}
	return f
}

// NPC responses.

// wound is a pierce: a head hit kills, so does a stab in the back of the
// unaware; anything else does its Intensity.
func (s *Sim) wound(k int, d Delivery, _ []string) bool {
	n := s.NPCs[k]
	dmg := int(d.Intensity)
	how, _ := d.St.Extra["how"].(string)
	if head, _ := d.St.Extra["head"].(bool); head || how == "stabbed" && n.State() != StateCombat && n.Suspicion() < 55 {
		dmg = 99
	}
	s.hurtNPC(k, dmg, how)
	return true
}

// blindNPC is a flash in sight: the NPC staggers about blind for
// Intensity ticks, unable to see, chase or fight (stagger).
func (s *Sim) blindNPC(k int, d Delivery, _ []string) bool {
	n := s.NPCs[k]
	if !dazzled(s, n.eye(), n.Yaw, d.St.Origin) {
		return false
	}
	s.B.SetFor(s.npcR(k), "blind", true, max(n.Blind(), int(d.Intensity)))
	n.Path, n.AttackT, n.Attacking = nil, 0, false
	n.LKP = d.St.Origin
	s.bark(k, pick(k, "Argh, my eyes!", "I can't see!", "Aagh!"))
	return true
}

// dazzlePlayer: the player knew it was coming.
func (s *Sim) dazzlePlayer(d Delivery, _ []string) bool {
	p := &s.Player
	if !dazzled(s, p.Eye(), p.Yaw, d.St.Origin) {
		return false
	}
	s.B.SetFor(s.playerR(), "blind", true, max(p.Blind(), int(d.Intensity)/3))
	return true
}

// dazzled reports whether a flash at p blinds eyes at e facing yaw: in
// sight, and looked at unless it is close.
func dazzled(s *Sim, e Vec3, yaw float64, p Vec3) bool {
	return lineOfSight(s.W, e, p) && (e.Dist(p) < 3 || angleDiff(yawTo(e, p), yaw) < 1.4)
}

// knockOut is a bash: the unaware (or blinded) drop, the fighting only notice.
// "knock_out hard" drops even the fighting (a slam into a wall).
func (s *Sim) knockOut(k int, _ Delivery, args []string) bool {
	n := s.NPCs[k]
	if !n.alive() {
		return false
	}
	if n.State() == StateCombat && n.Blind() == 0 && len(args) == 0 {
		s.emit(n.pos(), 20, "thud", -1)
		s.alertNPC(k, s.Player.Pos)
		return true
	}
	n.setState(StateUnconscious)
	s.B.Set(s.npcR(k), "down", true)
	s.disarm(k)
	n.Path = nil
	s.Stats.Knockouts++
	s.emit(n.pos(), 18, "body", -1)
	s.bark(k, "Ugh...")
	return true
}

// Cell responses.

// blastBlock: TNT is the one way to break voxels. A blast breaks every
// block within 2.6 of it, short of bedrock and obsidian; doors go with the
// objects, glass shatters and TNT primes (their own receptrons).
func blastBlock(s *Sim, c Cell, d Delivery) bool {
	name := s.W.Block(c.X, c.Y, c.Z)
	if _, door := s.doors[c]; door || c.Center().Dist(d.St.Origin) > 2.6 || name == BlockAir || name == "minecraft:bedrock" || name == "minecraft:obsidian" {
		return false
	}
	s.W.SetBlock(c.X, c.Y, c.Z, BlockAir)
	delete(s.Liquid, c)
	s.B.Clear(cellR(c), "burning")
	return true
}

// electrify puts current into the puddle containing c.
func (s *Sim) electrify(c Cell) {
	seen := map[Cell]bool{c: true}
	queue := []Cell{c}
	for len(queue) > 0 {
		x := queue[0]
		queue = queue[1:]
		s.B.SetFor(cellR(x), "live", true, 3*TicksPerSecond)
		for _, d := range []Cell{{1, 0, 0}, {-1, 0, 0}, {0, 0, 1}, {0, 0, -1}} {
			n := x.Add(d.X, 0, d.Z)
			if s.Liquid[n] == StimWater && !seen[n] {
				seen[n] = true
				queue = append(queue, n)
			}
		}
	}
	s.emit(c.Floor(), 55, "zap", -1)
	s.event(Event{Kind: EvSpark, Pos: c.Floor()})
}

// igniteOil sets an oil cell burning (stimuli.toml times it), lighting the cell above.
func (s *Sim) igniteOil(c Cell, _ Delivery) bool {
	if a := c.Add(0, 1, 0); s.W.Block(a.X, a.Y, a.Z) == BlockAir {
		s.W.SetBlock(a.X, a.Y, a.Z, BlockLight)
	}
	s.emit(c.Floor(), 35, "ignite", -1)
	return true
}

func (s *Sim) unlightAbove(c Cell) { s.unlight(c.Add(0, 1, 0)) }

// Liquids.

// LiquidCells are the cells holding liquid l.
func (s *Sim) LiquidCells(l Stim) []Cell {
	var out []Cell
	for c, x := range s.Liquid {
		if x == l {
			out = append(out, c)
		}
	}
	return out
}

// Live reports whether the water at c carries current.
func (s *Sim) Live(c Cell) bool { return s.B.Bool(cellR(c), "live") }

// OilBurning reports whether the oil at c is alight.
func (s *Sim) OilBurning(c Cell) bool { return s.Liquid[c] == StimOil && s.B.Bool(cellR(c), "burning") }

// pourOut pools amount cells of liquid from c: a wet spot at once, the
// rest seeps out a cell at a time (seep).
func (s *Sim) pourOut(liquid Stim, c Cell, amount int) {
	s.pour(liquid, c, 14)
	if n := amount - 1; n > 0 {
		r := cellR(c)
		s.B.Set(r, "spill", liquid)
		s.B.Add(r, "spill_left", float64(n))
	}
}

const spillEvery = TicksPerSecond / 3 // a puddle grows a cell at a time: the seep period in stimuli.toml

// seep pours the next cell of a spill still spreading from c.
func seep(s *Sim, c Cell, _ Delivery) bool {
	r := cellR(c)
	s.pour(s.B.String(r, "spill"), c, 14)
	if s.B.Add(r, "spill_left", -1); s.B.Float(r, "spill_left") <= 0 {
		s.B.Clear(r, "spill")
		s.B.Clear(r, "spill_left")
	}
	return true
}

var liquidBlock = map[Stim]string{StimWater: BlockPuddle, StimOil: BlockOil}

// pour adds one cell of liquid next to the puddle around start (or start
// itself), up to maxCells in the puddle. Another liquid blocks it.
func (s *Sim) pour(liquid Stim, start Cell, maxCells int) {
	if !s.walkable(start) {
		g, ok := s.groundBelow(start, 8) // it falls until it lands
		if !ok {
			return
		}
		start = g
	}
	seen := map[Cell]bool{start: true}
	queue := []Cell{start}
	for n := 0; len(queue) > 0; {
		c := queue[0]
		queue = queue[1:]
		if s.Liquid[c] == "" {
			s.Liquid[c] = liquid
			s.B.Set(cellR(c), "pooled", s.Now) // its sources run from now
			if s.W.Block(c.X, c.Y, c.Z) == BlockAir {
				s.W.SetBlock(c.X, c.Y, c.Z, liquidBlock[liquid])
			}
			return
		}
		if s.Liquid[c] != liquid {
			continue
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

// liquidAt is the liquid lying in c, or "".
func (s *Sim) liquidAt(c Cell) Stim { return s.Liquid[c] }
