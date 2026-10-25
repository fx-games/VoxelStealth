// Command buildplayer writes assets/models/player.gltf: the reference's
// player model (PlayerModel with wide arms, 64×64 skin, with the second
// layer), built cube by cube as ModelPart builds it, with Minecraft's walk and idle limb swings, over a
// placeholder skin. The game puts the NPCs' own skins on it
// (assets/skins/<model>.png).
//
//	go run ./cmd/buildplayer
package main

import (
	"image"
	"image/color"
	"log"
	"math"
	"os"
	"path/filepath"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/model"
	"github.com/memmaker/voxellib/model/gltf"
)

// A box is a ModelPart cube in the reference's model space (y down,
// facing -Z, the right arm towards -X): from (x, y, z) relative to its
// part's pivot, w×h×d, at texture offset (u, v), grown by grow.
type box struct {
	x, y, z, w, h, d, u, v, grow float32
}

// parts are PlayerModel's (wide arms): HumanoidModel's pivots and boxes,
// each with its second-layer box.
var parts = []struct {
	name  string
	pivot mgl32.Vec3
	boxes []box
}{
	{"body", mgl32.Vec3{0, 0, 0}, []box{{-4, 0, -2, 8, 12, 4, 16, 16, 0}, {-4, 0, -2, 8, 12, 4, 16, 32, 0.25}}},
	{"head", mgl32.Vec3{0, 0, 0}, []box{{-4, -8, -4, 8, 8, 8, 0, 0, 0}, {-4, -8, -4, 8, 8, 8, 32, 0, 0.5}}},
	{"rightArm", mgl32.Vec3{-5, 2, 0}, []box{{-3, -2, -2, 4, 12, 4, 40, 16, 0}, {-3, -2, -2, 4, 12, 4, 40, 32, 0.25}}},
	{"leftArm", mgl32.Vec3{5, 2, 0}, []box{{-1, -2, -2, 4, 12, 4, 32, 48, 0}, {-1, -2, -2, 4, 12, 4, 48, 48, 0.25}}},
	{"rightLeg", mgl32.Vec3{-1.9, 12, 0}, []box{{-2, 0, -2, 4, 12, 4, 0, 16, 0}, {-2, 0, -2, 4, 12, 4, 0, 32, 0.25}}},
	{"leftLeg", mgl32.Vec3{1.9, 12, 0}, []box{{-2, 0, -2, 4, 12, 4, 16, 48, 0}, {-2, 0, -2, 4, 12, 4, 0, 48, 0.25}}},
}

// ours takes a point from the reference's model space to the game's (y
// up, feet at 0, in blocks, facing +Z so that the renderer's half turn
// faces it the sim's way): the reference's renderer flips X and Y and
// stands it 24 pixels up; our yaw is the reference's plus a half turn.
func ours(p mgl32.Vec3) mgl32.Vec3 { return mgl32.Vec3{p[0], 24 - p[1], -p[2]}.Mul(1. / 16) }

func main() {
	m := &model.Model{Name: "player", Textures: []image.Image{placeholder()}, Clips: map[string]*model.Clip{}, Tags: map[string]bool{}}
	me := model.Mesh{Texture: 0}
	for bi, p := range parts {
		piv := ours(p.pivot)
		m.Bones = append(m.Bones, model.Bone{Name: p.name, Parent: -1, T: piv, R: mgl32.QuatIdent(), S: mgl32.Vec3{1, 1, 1},
			InvBind: mgl32.Translate3D(-piv[0], -piv[1], -piv[2])})
		for _, b := range p.boxes {
			cube(&me, uint16(bi), p.pivot, b)
		}
	}
	m.Meshes = []model.Mesh{me}
	swing(m)
	data, err := gltf.Encode(m)
	if err != nil {
		log.Fatal(err)
	}
	out := filepath.Join("assets", "models", "player.gltf")
	if err := os.WriteFile(out, data, 0o644); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote %s (%d bones, clips %v)", out, len(m.Bones), m.ClipNames())
}

// cube adds box b of the part pivoted at pivot as the reference's
// ModelPart.Cube builds it: its six polygons with their vertex order and
// box UVs (the second layer grown, its UVs not).
func cube(me *model.Mesh, bone uint16, pivot mgl32.Vec3, b box) {
	lo := pivot.Add(mgl32.Vec3{b.x - b.grow, b.y - b.grow, b.z - b.grow})
	hi := pivot.Add(mgl32.Vec3{b.x + b.w + b.grow, b.y + b.h + b.grow, b.z + b.d + b.grow})
	v0, v1, v2, v3 := mgl32.Vec3{lo[0], lo[1], lo[2]}, mgl32.Vec3{hi[0], lo[1], lo[2]}, mgl32.Vec3{hi[0], hi[1], lo[2]}, mgl32.Vec3{lo[0], hi[1], lo[2]}
	v4, v5, v6, v7 := mgl32.Vec3{lo[0], lo[1], hi[2]}, mgl32.Vec3{hi[0], lo[1], hi[2]}, mgl32.Vec3{hi[0], hi[1], hi[2]}, mgl32.Vec3{lo[0], hi[1], hi[2]}
	u, v, w, h, d := b.u, b.v, b.w, b.h, b.d
	f4, f5, f6, f7, f8, f9 := u, u+d, u+d+w, u+d+w+w, u+d+w+d, u+d+w+d+w
	f10, f11, f12 := v, v+d, v+d+h
	polys := []struct {
		vs             [4]mgl32.Vec3
		u1, v1, u2, v2 float32
		normal         mgl32.Vec3
	}{
		{[4]mgl32.Vec3{v5, v1, v2, v6}, f6, f11, f8, f12, mgl32.Vec3{1, 0, 0}},  // east
		{[4]mgl32.Vec3{v0, v4, v7, v3}, f4, f11, f5, f12, mgl32.Vec3{-1, 0, 0}}, // west
		{[4]mgl32.Vec3{v5, v4, v0, v1}, f5, f10, f6, f11, mgl32.Vec3{0, -1, 0}}, // down
		{[4]mgl32.Vec3{v2, v3, v7, v6}, f6, f11, f7, f10, mgl32.Vec3{0, 1, 0}},  // up
		{[4]mgl32.Vec3{v1, v0, v3, v2}, f5, f11, f6, f12, mgl32.Vec3{0, 0, -1}}, // north
		{[4]mgl32.Vec3{v4, v5, v6, v7}, f8, f11, f9, f12, mgl32.Vec3{0, 0, 1}},  // south
	}
	for _, p := range polys {
		uvs := [4]mgl32.Vec2{{p.u2, p.v1}, {p.u1, p.v1}, {p.u1, p.v2}, {p.u2, p.v2}}
		n := ours(p.normal).Sub(ours(mgl32.Vec3{})).Normalize()
		base := uint32(len(me.Verts))
		for i, c := range p.vs {
			vx := model.Vertex{Pos: ours(c), Normal: n, UV: uvs[i].Mul(1. / 64)}
			vx.Joints[0], vx.Weights[0] = bone, 1
			me.Verts = append(me.Verts, vx)
		}
		// Counter-clockwise from outside, whichever way the polygon runs.
		a, b2, c := me.Verts[base].Pos, me.Verts[base+1].Pos, me.Verts[base+2].Pos
		if b2.Sub(a).Cross(c.Sub(a)).Dot(n) > 0 {
			me.Indices = append(me.Indices, base, base+1, base+2, base, base+2, base+3)
		} else {
			me.Indices = append(me.Indices, base, base+2, base+1, base, base+3, base+2)
		}
	}
}

// swing adds the reference's limb swings (HumanoidModel.setupAnim): walking
// swings the legs and the opposite arms (cos(t·0.6662)·1.4·amount and
// ·amount, at a walk's amount), idling sways the arms out (zRot
// cos(age·0.09)·0.05 + 0.05).
func swing(m *model.Model) {
	const n = 16
	clip := func(name string, length float32, rot func(bone string, t float64) mgl32.Vec3) {
		c := &model.Clip{Name: name, Length: length}
		for _, b := range []string{"rightArm", "leftArm", "rightLeg", "leftLeg"} {
			k := &model.Keyframes{Quat: true}
			for i := 0; i <= n; i++ {
				t := float64(i) / n
				r := rot(b, t)
				q := mgl32.AnglesToQuat(r[2], r[1], r[0], mgl32.ZYX)
				k.Times = append(k.Times, float32(t)*length)
				k.Values = append(k.Values, mgl32.Vec4{q.V[0], q.V[1], q.V[2], q.W})
			}
			c.Channels = append(c.Channels, model.Channel{Bone: m.Bone(b), Target: model.Rotation, Track: k})
		}
		m.Clips[name] = c
	}
	const amount = 0.55
	clip("walking", 1, func(b string, t float64) mgl32.Vec3 {
		ph := 2 * math.Pi * t
		x := map[string]float64{
			"rightLeg": math.Cos(ph) * 1.4 * amount, "leftLeg": math.Cos(ph+math.Pi) * 1.4 * amount,
			"rightArm": math.Cos(ph+math.Pi) * amount, "leftArm": math.Cos(ph) * amount,
		}[b]
		return mgl32.Vec3{float32(x), 0, 0}
	})
	clip("idle", 3.5, func(b string, t float64) mgl32.Vec3 {
		z := math.Cos(2*math.Pi*t)*0.05 + 0.05 // the reference's zRot; ours turns the other way
		switch b {
		case "rightArm":
			return mgl32.Vec3{0, 0, -float32(z)}
		case "leftArm":
			return mgl32.Vec3{0, 0, float32(z)}
		}
		return mgl32.Vec3{}
	})
}

// placeholder is a plain skin until the real ones come: face, a dark coat,
// trousers and boots; the second layer empty.
func placeholder() image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	fill := func(x0, y0, x1, y1 int, c color.NRGBA) {
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				img.SetNRGBA(x, y, c)
			}
		}
	}
	skin, hair, coat, trousers, boots := color.NRGBA{196, 150, 120, 255}, color.NRGBA{60, 40, 25, 255},
		color.NRGBA{70, 60, 50, 255}, color.NRGBA{45, 45, 55, 255}, color.NRGBA{35, 25, 20, 255}
	fill(0, 0, 32, 16, skin) // head
	fill(8, 0, 16, 8, hair)  // top
	fill(0, 8, 32, 10, hair) // hairline all round
	fill(8, 12, 10, 13, color.NRGBA{40, 40, 60, 255})
	fill(14, 12, 16, 13, color.NRGBA{40, 40, 60, 255}) // eyes
	fill(16, 16, 40, 32, coat)                         // body
	fill(40, 16, 56, 32, coat)                         // right arm
	fill(32, 48, 48, 64, coat)                         // left arm
	fill(40, 29, 56, 32, skin)                         // right hand
	fill(32, 61, 48, 64, skin)                         // left hand
	fill(0, 16, 16, 32, trousers)
	fill(16, 48, 32, 64, trousers)
	fill(0, 29, 16, 32, boots)
	fill(16, 61, 32, 64, boots)
	return img
}
