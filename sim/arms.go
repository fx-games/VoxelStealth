package sim

// NPC arms: a guard's weapon (a long sword, a halberd or a crossbow,
// chosen in the editor or at random) and the torch an NPC may carry on
// its rounds, lighting its way. Down, it drops both: the weapon lies
// about for the player to take (KindWeapon), the torch burns on where it
// fell.

import (
	"math"

	"github.com/memmaker/stimres"
)

// KindWeapon is a weapon lying about (Name: a Weapons key): take it.
const KindWeapon = "weapon"

// Weapon is what an NPC or the player fights with.
type Weapon struct {
	Name   string
	Reach  float64 // blocks, feet to feet
	Swing  int     // ticks an attack takes; its hit lands halfway (the upswing)
	Damage float64 // to the player: pierce if Sharp, else bash
	Sharp  bool    // cuts; thrown, it runs a body through and may pin it
	Ranged bool    // shoots bolts (aimBolt)
}

// Weapons are the arms by key, as NPCDef.Weapon and KindWeapon names them.
var Weapons = map[string]Weapon{
	"sword":    {Name: "long sword", Reach: 2.4, Swing: 12, Damage: 7, Sharp: true},
	"halberd":  {Name: "halberd", Reach: 3.2, Swing: 18, Damage: 10, Sharp: true},
	"crossbow": {Name: "crossbow", Reach: 2.0, Swing: 14, Damage: 3, Ranged: true},
}

// WeaponKeys are the Weapons in a fixed order, for random picks.
var WeaponKeys = []string{"sword", "halberd", "crossbow"}

// fists is an unarmed NPC's punch.
var fists = Weapon{Name: "fists", Reach: 2.0, Swing: 12, Damage: 3}

const (
	BoltDamage = 8  // a crossbow bolt in the player
	SpanTicks  = 28 // spanning a crossbow: its charge and the use beyond
	boltRange  = 24 // the farthest a guard shoots
	aimTicks   = 10 // aiming a spanned crossbow before the shot
)

// weaponOf is the weapon NPC k fights with.
func (s *Sim) weaponOf(k int) Weapon {
	if w, ok := Weapons[s.NPCs[k].Weapon]; ok {
		return w
	}
	return fists
}

// arm gives a new NPC its weapon and torch: Weapon "random" (or none, for
// a guard) is one of WeaponKeys.
func (s *Sim) arm(n *NPC, d *NPCDef) {
	w := d.Weapon
	if w == "" && d.Kind == "guard" || w == "random" {
		w = WeaponKeys[s.rng.Intn(len(WeaponKeys))]
	}
	if _, ok := Weapons[w]; ok {
		n.Weapon = w
	}
	n.Loaded = n.Weapon == "crossbow"
	if d.Torch {
		o := &Object{ID: d.ID + "_torch", Kind: KindTorch, Name: "Torch", Pos: n.pos(), On: "floor", StartLit: true}
		n.TorchObj = s.addObject(o) + 1
		s.B.Set(s.objR(n.TorchObj-1), "carried", true)
	}
}

// addObject adds o to the world and returns its index.
func (s *Sim) addObject(o *Object) int {
	o.R = s.B.NewObject(archetypeOf(o))
	if o.StartLit {
		s.B.Set(stimres.Object[Cell](o.R), "lit", true)
	}
	s.Objects = append(s.Objects, o)
	i := len(s.Objects) - 1
	o.b = s.B
	if s.byID != nil {
		s.byID[o.R] = i
	}
	return i
}

// carryTorch keeps NPC k's torch in its left hand, its light with it.
func (s *Sim) carryTorch(k int) {
	n := s.NPCs[k]
	if n.TorchObj == 0 {
		return
	}
	i := n.TorchObj - 1
	o := s.Objects[i]
	lx, lz := forward(n.Yaw - math.Pi/2)
	o.Pos = n.pos().Add(Vec3{lx * 0.35, 0.6, lz * 0.35}) // its light at chest height, as the player's
	if o.Lit() && (!o.HasLight || o.LightCell != o.Pos.Add(Vec3{0, 0.5, 0}).Cell()) {
		s.setLight(i, true)
	}
}

// disarm drops what NPC k carries as it goes down: the weapon to the
// floor at its feet, the torch to burn on where it falls.
func (s *Sim) disarm(k int) {
	n := s.NPCs[k]
	at := n.pos()
	if g, ok := s.groundBelow(at.Cell(), 8); ok {
		at.Y = float64(g.Y)
	}
	if n.TorchObj != 0 {
		i := n.TorchObj - 1
		o, r := s.Objects[i], s.objR(i)
		n.TorchObj = 0
		s.B.Clear(r, "carried")
		s.B.Set(r, "fallen", true)
		lx, lz := forward(n.Yaw - math.Pi/2)
		o.Pos = Vec3{at.X + lx*0.5, at.Y, at.Z + lz*0.5}
		s.setLight(i, o.Lit())
	}
	if n.Weapon != "" {
		rx, rz := forward(n.Yaw + math.Pi/2)
		s.addObject(&Object{ID: n.ID + "_" + n.Weapon, Kind: KindWeapon, Name: n.Weapon, Pos: Vec3{at.X + rx*0.4, at.Y, at.Z + rz*0.4}, Yaw: n.Yaw})
		s.emit(at, 18, "clatter", -1)
		n.Weapon, n.Loaded, n.Span, n.AimT = "", false, 0, 0
	}
}

// takeWeapon is the player taking a weapon lying about: a blade to wield
// (Player.Weapons); a crossbow, as the player has one, for its bolts.
func (s *Sim) takeWeapon(i int, _ Delivery, _ []string) bool {
	o, p := s.Objects[i], &s.Player
	if o.Name == "crossbow" {
		p.Ammo[AmmoBroadhead] += 3
		s.message("Took 3 broadhead bolts from the crossbow")
	} else {
		p.Weapons[o.Name] = true
		s.message("Took the " + Weapons[o.Name].Name)
	}
	s.emit(o.Pos, 8, "swish", -1)
	s.B.Destroy(s.objR(i))
	return true
}

// aimBolt is a crossbowman's turn in a fight at d blocks from the player:
// span the crossbow, aim and shoot (EvShoot) while the player is in sight
// and not too near. False leaves it to close in and strike.
func (s *Sim) aimBolt(k int, d float64) bool {
	n, p := s.NPCs[k], &s.Player
	if n.LastSeen != 0 || d < 3 || d > boltRange {
		n.AimT = 0
		return false
	}
	s.turn(n, yawTo(n.pos(), p.Pos), 0.3)
	if !n.Loaded {
		if n.Span++; n.Span >= SpanTicks {
			n.Loaded, n.Span = true, 0
		}
		return true
	}
	if n.AimT++; n.AimT < aimTicks {
		return true
	}
	from, to := n.eye(), p.Pos.Add(Vec3{Y: 1.1})
	if !lineOfSight(s.W, from, to) {
		return false
	}
	n.Loaded, n.AimT = false, 0
	dist := from.DistXZ(to)
	t := dist / boltSpeed // ticks in flight; the drop over them aimed above
	pitch := math.Atan2(to.Y-from.Y+0.5*boltGravity*t*t, dist)
	s.event(Event{Kind: EvShoot, Pos: from, Yaw: yawTo(from, to), Pitch: pitch, NPC: k})
	s.emit(from, 20, "twang", k)
	return true
}

// A bolt's flight, as the game flies it (projectile.Speed, Gravity).
const (
	boltSpeed   = 3.15
	boltGravity = 0.05
)

// BoltHit is a guard's bolt (EvShoot from shooter) striking the player, NPC
// npc (head: in its head), or else the world at pos.
func (s *Sim) BoltHit(shooter int, pos Vec3, player bool, npc int, head bool, yaw float64) {
	if player {
		me := s.npcR(shooter)
		s.hit(s.playerR(), Stimulus{Stim: StimPierce, Intensity: BoltDamage, Origin: pos, Source: &me,
			Extra: map[string]any{"why": "Shot by " + s.NPCs[shooter].Name + "."}})
		return
	}
	s.ArrowHit(AmmoBroadhead, pos, npc, head, yaw)
}
