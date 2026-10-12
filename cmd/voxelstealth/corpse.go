package main

// Exact arrow hits on NPCs and their deaths, copied and adapted from VoxelLib
// cmd/explorer/crossbow.go: arrows test the NPC's animated triangles in the
// pose last drawn, stick in the bone they hit and ride it. A killed (or
// knocked out) NPC freezes in its pose and topples over the way the arrow
// was flying, 90° over a second. Unlike the explorer, bodies stay: they can
// be carried and hidden.

import (
	"math"
	"strings"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/go-gl/mathgl/mgl64"
	"github.com/memmaker/voxellib/model"
	"github.com/memmaker/voxellib/voxel/collide"
	"github.com/memmaker/voxellib/voxel/render"
)

const deathTicks = 20 // falling over

// shapeInfo is per-model data for exact hits.
type shapeInfo struct {
	head   []bool  // per bone: the "head" bone or one of its children
	radius float64 // bounding sphere around the feet origin
}

func newShape(m *model.Model) *shapeInfo {
	si := &shapeInfo{head: make([]bool, len(m.Bones))}
	for i, b := range m.Bones {
		si.head[i] = strings.EqualFold(b.Name, "head") || (b.Parent >= 0 && si.head[b.Parent])
	}
	lo, hi := m.Bounds()
	for _, v := range []mgl32.Vec3{lo, hi} {
		si.radius = math.Max(si.radius, float64(v.Len()))
	}
	si.radius *= 1.5 // room for animation to swing limbs out
	return si
}

// npcTarget lets arrows hit an NPC's body.
type npcTarget struct {
	g *game
	k int

	dying   int     // ticks since the NPC went down (0 = standing)
	fallYaw float64 // direction it falls

	// As last drawn, for exact hits and arrows riding bones.
	drawn                        bool
	drawX, drawY, drawZ, drawYaw float64
	drawTilt                     float64
	drawFrame, poseFrame         int
	pose                         *model.Pose
}

func (t *npcTarget) skin() *npcSkin {
	n := t.g.sim.NPCs[t.k]
	if sk := t.g.skins[n.Model]; sk != nil {
		return sk
	}
	return t.g.skins["guard"]
}

func (t *npcTarget) shape() *shapeInfo {
	sk := t.skin()
	if sk.shape == nil {
		sk.shape = newShape(sk.model)
	}
	return sk.shape
}

// bones is the skin matrices of the pose last drawn (nil before).
func (t *npcTarget) bones() []mgl32.Mat4 {
	if t.k < len(t.g.npcAnims) && t.g.npcAnims[t.k] != nil {
		return t.g.npcAnims[t.k].skin
	}
	return nil
}

func (t *npcTarget) Box() collide.AABB { return t.g.sim.NPCs[t.k].Body.Box() }

func (t *npcTarget) Frame() (x, y, z, yaw float64) {
	n := t.g.sim.NPCs[t.k]
	return n.Body.X, n.Body.Y, n.Body.Z, n.Yaw
}

// Reach is the box the animated mesh can occupy, around both the drawn and
// the current position.
func (t *npcTarget) Reach() collide.AABB {
	r := t.shape().radius
	b := &t.g.sim.NPCs[t.k].Body
	x0, y0, z0 := b.X, b.Y, b.Z
	x1, y1, z1 := x0, y0, z0
	if t.drawn {
		x1, y1, z1 = t.drawX, t.drawY, t.drawZ
	}
	return collide.AABB{MinX: min(x0, x1) - r, MinY: min(y0, y1) - r, MinZ: min(z0, z1) - r,
		MaxX: max(x0, x1) + r, MaxY: max(y0, y1) + r, MaxZ: max(z0, z1) + r}
}

// creatureMatrix places a model's space in the world, as the model
// renderer does, tipped over by tilt toward tiltYaw.
func creatureMatrix(x, y, z, yaw, tilt, tiltYaw float64) mgl32.Mat4 {
	return mgl32.Translate3D(float32(x), float32(y), float32(z)).Mul4(render.TiltMatrix(tilt, tiltYaw)).
		Mul4(mgl32.HomogRotate3DY(float32(math.Pi - yaw)))
}

func (t *npcTarget) boneMatrix(b int) mgl32.Mat4 {
	if s := t.bones(); b >= 0 && b < len(s) {
		return s[b]
	}
	return mgl32.Ident4()
}

// world is the NPC's current model-to-world matrix.
func (t *npcTarget) world() mgl32.Mat4 {
	n := t.g.sim.NPCs[t.k]
	return creatureMatrix(n.Body.X, n.Body.Y, n.Body.Z, n.Yaw, t.deathTilt(1), t.fallYaw)
}

// Joint maps bone b's bind-pose space to the world.
func (t *npcTarget) Joint(b int) mgl64.Mat4 { return mat64(t.world().Mul4(t.boneMatrix(b))) }

func mat64(m mgl32.Mat4) (o mgl64.Mat4) {
	for i := range m {
		o[i] = float64(m[i])
	}
	return o
}

// Intersect tests the segment against the mesh as last drawn, then carries
// the hit point into the current frame so the arrow sticks where it was
// seen to hit.
func (t *npcTarget) Intersect(x0, y0, z0, x1, y1, z1 float64) (x, y, z float64, part int, ok bool) {
	n := t.g.sim.NPCs[t.k]
	dx, dy, dz, dyaw, tilt := t.drawX, t.drawY, t.drawZ, t.drawYaw, t.drawTilt
	skin := t.bones()
	if !t.drawn {
		dx, dy, dz, dyaw, tilt = n.Body.X, n.Body.Y, n.Body.Z, n.Yaw, t.deathTilt(1)
		skin = nil
	}
	if !segNearSphere(x0-dx, y0-dy, z0-dz, x1-dx, y1-dy, z1-dz, t.shape().radius) {
		return 0, 0, 0, -1, false
	}
	inv := creatureMatrix(dx, dy, dz, dyaw, tilt, t.fallYaw).Inv()
	o := mgl32.TransformCoordinate(mgl32.Vec3{float32(x0), float32(y0), float32(z0)}, inv)
	e := mgl32.TransformCoordinate(mgl32.Vec3{float32(x1), float32(y1), float32(z1)}, inv)
	m := t.skin().model
	if t.pose == nil || t.poseFrame != t.drawFrame || !t.drawn {
		t.pose = m.Pose(skin, t.pose)
		t.poseFrame = t.drawFrame
	}
	h, ok := m.RaycastPose(t.pose, o, e.Sub(o), 1)
	if !ok {
		return 0, 0, 0, -1, false
	}
	w := mgl32.TransformCoordinate(h.Pos, t.world())
	return float64(w[0]), float64(w[1]), float64(w[2]), h.Bone, true
}

// segNearSphere reports whether the segment a→b passes within r of the
// origin.
func segNearSphere(ax, ay, az, bx, by, bz, r float64) bool {
	dx, dy, dz := bx-ax, by-ay, bz-az
	s := 0.0
	if l := dx*dx + dy*dy + dz*dz; l > 0 {
		s = math.Max(0, math.Min(1, -(ax*dx+ay*dy+az*dz)/l))
	}
	px, py, pz := ax+dx*s, ay+dy*s, az+dz*s
	return px*px+py*py+pz*pz <= r*r
}

// isHead reports whether bone part is the head.
func (t *npcTarget) isHead(part int) bool {
	h := t.shape().head
	return part >= 0 && part < len(h) && h[part]
}

// fall starts the NPC toppling over toward yaw.
func (t *npcTarget) fall(yaw float64) {
	if t.dying == 0 {
		t.dying, t.fallYaw = 1, yaw
	}
}

// tickFall advances the fall; NPCs that went down without an arrow topple
// away from the player who struck them (blade, blackjack), or backwards
// when nobody is near (fire).
func (t *npcTarget) tickFall() {
	n := t.g.sim.NPCs[t.k]
	switch {
	case n.Alive():
		t.dying = 0
	case t.dying == 0:
		yaw := n.Yaw + math.Pi
		if d := n.Pos().Sub(t.g.sim.Player.Pos); d.X*d.X+d.Z*d.Z < 9 {
			yaw = math.Atan2(d.X, -d.Z) // yaw convention: 0 faces -Z
		}
		t.fall(yaw)
	case t.dying <= deathTicks:
		t.dying++
	}
}

// deathTilt is how far the NPC has fallen over (radians).
func (t *npcTarget) deathTilt(alpha float64) float64 {
	if t.dying == 0 {
		return 0
	}
	f := math.Min(1, (float64(t.dying-1)+alpha)/deathTicks)
	return math.Sqrt(f) * math.Pi / 2
}
