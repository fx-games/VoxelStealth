package sim

import (
	"math"

	"github.com/memmaker/voxellib/ragdoll"
)

// Kicking, Dark Messiah style: the kick is a stimulus like any blow; an
// NPC it reaches (alive, out cold or dead) flies off (shove) and tumbles
// until it comes to rest. Whatever it slams into on the way gets a slam
// stimulus, and so does the body itself: spikes impale it, glass shatters,
// a wall or a long fall knocks it out, fire and live water do the rest
// through their own sources.

const (
	kickReach = 1.8
	kickPower = 0.75 // blocks per tick: a body flies about four blocks
	kickLift  = 0.3
	dazedFor  = 2 * TicksPerSecond // a kicked NPC getting up
)

// Kick kicks whatever is in front of the player: an NPC, else a door,
// window or thing within reach.
func (s *Sim) Kick() bool {
	p := &s.Player
	pr := s.playerR()
	st := Stimulus{Stim: StimKick, Intensity: 1, Source: &pr, Extra: map[string]any{"yaw": p.Yaw}}
	best, bestD := -1, kickReach
	for k, n := range s.NPCs {
		if n.Hidden || n.CarriedBy {
			continue
		}
		d := n.pos().DistXZ(p.Pos)
		if d < bestD && angleDiff(yawTo(p.Pos, n.pos()), p.Yaw) < 0.9 && math.Abs(n.pos().Y-p.Pos.Y) < 1.5 {
			best, bestD = k, d
		}
	}
	if best >= 0 {
		st.Origin = s.NPCs[best].pos()
		s.emit(st.Origin, 22, "thud", -1)
		return s.hit(s.npcR(best), st)
	}
	s.emit(p.Pos, 6, "swish", -1)
	fx, fz := forward(p.Yaw)
	st.Medium, st.Range = "radius", 0.3
	for t := 0.3; t < kickReach; t += 0.1 { // at knee height
		st.Origin = p.Pos.Add(Vec3{fx * t, 0.5, fz * t})
		c := st.Origin.Cell()
		hit := s.W.Solid(c.X, c.Y, c.Z)
		for i, o := range s.Objects {
			if lo, hi := s.ObjectBox(i); !o.Gone && boxDist(st.Origin, lo, hi) == 0 {
				hit = true
			}
		}
		if hit {
			return s.Stimulate(st)
		}
	}
	return false
}

// shove is the kick receptron: the NPC flies off the way the kicker faced.
func (s *Sim) shove(k int, d Delivery, _ []string) bool {
	n := s.NPCs[k]
	if n.Hidden || n.CarriedBy {
		return false
	}
	yaw, _ := d.St.Extra["yaw"].(float64)
	ragdoll.Kick(&n.Body, yaw, kickPower*d.Intensity, kickLift)
	n.Flight, n.Path = ragdoll.Flight{}, nil
	s.B.Set(s.npcR(k), "airborne", true)
	if n.alive() {
		s.bark(k, "Argh!")
	}
	return true
}

// tumble is an airborne NPC's tick: it flies, slams into things and,
// at rest, gets up dazed (blind until recover) if it can.
func (s *Sim) tumble(k int) {
	n, r := s.NPCs[k], s.npcR(k)
	imp, rest := n.Flight.Tick(&n.Body, bodyWorld{s})
	if imp.Speed > 0 {
		st := Stimulus{Stim: StimSlam, Intensity: imp.Speed * TicksPerSecond, Origin: n.pos(), Source: &r}
		s.emit(n.pos(), 10+st.Intensity*2, "thud", k)
		s.hit(r, st)
		for _, c := range imp.Cells {
			s.hit(cellR(Cell{c[0], c[1], c[2]}), st)
		}
	}
	if rest {
		s.B.Clear(r, "airborne")
		if n.alive() {
			s.B.SetFor(r, "blind", true, max(n.Blind(), dazedFor))
		}
	}
}

// Airborne reports whether NPC k is flying from a kick.
func (s *Sim) Airborne(k int) bool { return s.B.Bool(s.npcR(k), "airborne") }

// impale is the spikes' slam receptron: whatever fell on them is pierced.
func impale(s *Sim, d Delivery, _ []string) bool {
	if d.St.Source == nil {
		return false
	}
	return s.hit(*d.St.Source, Stimulus{Stim: StimPierce, Intensity: 20, Origin: d.St.Origin,
		Extra: map[string]any{"how": "impaled"}})
}
