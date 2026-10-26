package main

import (
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/voxel/mesh"
	"github.com/memmaker/voxellib/voxel/render"
)

// blockMesh is a block state's mesh on its own (drawn falling, or cracked),
// built once per state.
func (g *game) blockMesh(name string) *render.BlockMesh {
	if g.blockMeshes == nil {
		g.blockMeshes = map[string]*render.BlockMesh{}
	}
	if b, ok := g.blockMeshes[name]; ok {
		return b
	}
	t := g.reg.Table()
	b := render.NewBlockMesh(mesh.Block(t[g.reg.Parse(name)], t[0], g.eng.Renderer.MeshConfig()), true)
	g.blockMeshes[name] = b
	return b
}

// drawCollapse draws structures coming down and the cracks in strained
// supports.
func (g *game) drawCollapse(alpha float64) {
	s := g.sim
	var items []render.BlockItem
	for _, p := range s.Falls {
		lag := (p.Drop - p.PrevDrop) * (1 - alpha)
		for i, st := range p.States {
			b := g.blockMesh(st)
			if b == nil {
				continue
			}
			x, y, z := p.At(i)
			sky, blk := g.light(x+0.5, y+0.5+lag, z+0.5)
			items = append(items, render.BlockItem{Mesh: b, X: x, Y: y + lag, Z: z, M: mgl32.Ident4(), Sky: sky, Block: blk})
		}
	}
	g.eng.Renderer.DrawBlockItems(&g.cam, items)
	for _, st := range s.Mission.Structures {
		for _, c := range st.Supports {
			if k := s.Strain(c); k > 0 {
				if b := g.blockMesh(g.world.Block(c.X, c.Y, c.Z)); b != nil {
					g.eng.Renderer.DrawBlockDamage(&g.cam, b, [3]int{c.X, c.Y, c.Z}, min(9, int(k*10)))
				}
			}
		}
	}
}
