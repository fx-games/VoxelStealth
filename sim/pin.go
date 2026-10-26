package sim

import (
	"math"

	"github.com/memmaker/voxellib/voxel/collide"
)

// Pinning, as VoxelRogue has it (Painkiller's stake gun): a sharp missile
// that kills an NPC with a wall close behind it carries the body back
// along its flight and pins it there, the shaft through it.

const (
	pinReach = 4   // blocks from the body to the wall at most
	pinSpeed = 1.0 // blocks per tick: the reference's four-tick flight
)

// pin is the pierce receptron's last effect: if the missile (flying along
// Extra yaw) killed NPC k and a wall is within pinReach behind it, the
// body flies on to the wall (pinning) and hangs there (pinned).
func (s *Sim) pin(k int, d Delivery, _ []string) bool {
	n, r := s.NPCs[k], s.npcR(k)
	yaw, ok := d.St.Extra["yaw"].(float64)
	if !ok || n.alive() || n.Hidden || n.CarriedBy || s.B.Bool(r, "pinned") || s.Airborne(k) {
		return false
	}
	ux, uz := forward(yaw)
	o := d.St.Origin
	for t := 0.0; t <= pinReach; t += 0.05 {
		if c := (Vec3{o.X + ux*t, o.Y, o.Z + uz*t}).Cell(); s.W.Solid(c.X, c.Y, c.Z) {
			s.B.SetFor(r, "pinning", true, int(math.Ceil(pinReach/pinSpeed))+2)
			s.B.Set(r, "pin_yaw", yaw)
			return true
		}
	}
	return false
}

// flyToPin carries a pinning body one tick on towards the wall; when the
// wall stops it, it is pinned there.
func (s *Sim) flyToPin(k int) {
	n, r := s.NPCs[k], s.npcR(k)
	b := &n.Body
	ux, uz := forward(s.B.Float(r, "pin_yaw"))
	dx, _, dz := collide.Move(bodyWorld{s}, b.Box(), ux*pinSpeed, 0, uz*pinSpeed)
	b.PrevX, b.PrevY, b.PrevZ = b.X, b.Y, b.Z
	b.X, b.Z = b.X+dx, b.Z+dz
	if math.Hypot(dx, dz) < pinSpeed*0.99 {
		s.B.Clear(r, "pinning")
		s.B.Set(r, "pinned", true)
		s.emit(n.pos(), 18, "body", -1)
	}
}

// Pinned reports whether NPC k's body is pinned to a wall, or on its way
// there, and the way it faces: the missile's flight.
func (s *Sim) Pinned(k int) (pinned, flying bool, yaw float64) {
	r := s.npcR(k)
	return s.B.Bool(r, "pinned"), s.B.Bool(r, "pinning"), s.B.Float(r, "pin_yaw")
}

// unpin frees a pinned body (carried off, kicked).
func (s *Sim) unpin(k int) {
	r := s.npcR(k)
	s.B.Clear(r, "pinned")
	s.B.Clear(r, "pinning")
}
