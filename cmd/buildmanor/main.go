// Command buildmanor builds the demo mission, Ashworth Manor, into
// missions/manor: the voxels as Minecraft region files in world/ and the
// mission records next to them. Run it again to reset the mission; after
// that, edit it in the game's editor (F6).
//
//	go run ./cmd/buildmanor [-out missions/manor]
package main

import (
	"flag"
	"log"
	"math"
	"os"
	"path/filepath"

	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/anvil"
	"github.com/memmaker/voxelstealth/level"
	"github.com/memmaker/voxelstealth/sim"
)

// Heights: ground floor feet at G (floor blocks at G-1), upper floor feet
// at U, cellar feet at C.
const (
	G = 64
	U = 69
	C = 58
)

type builder struct {
	w   *voxel.World
	reg *voxel.Registry
}

func (b *builder) set(x, y, z int, name string) {
	p := voxel.ChunkPos{X: x >> 4, Z: z >> 4}
	c := b.w.Chunk(p)
	if c == nil {
		c = b.w.NewChunk(p)
		b.w.AddChunk(c)
	}
	c.SetRaw(x&15, y, z&15, b.reg.Parse(name))
}

func (b *builder) fill(x0, y0, z0, x1, y1, z1 int, name string) {
	for x := min(x0, x1); x <= max(x0, x1); x++ {
		for y := min(y0, y1); y <= max(y0, y1); y++ {
			for z := min(z0, z1); z <= max(z0, z1); z++ {
				b.set(x, y, z, name)
			}
		}
	}
}

func (b *builder) solid(x, y, z int) bool { return b.reg.Info(b.w.Block(x, y, z)).Collides }
func (b *builder) wall(x, y, z int) bool {
	return b.solid(x, y, z) && !sim.IsGlass(b.reg.Info(b.w.Block(x, y, z)).Name)
}

func (b *builder) air(x0, y0, z0, x1, y1, z1 int) { b.fill(x0, y0, z0, x1, y1, z1, "air") }

func (b *builder) tree(x, z int) {
	b.fill(x, G, z, x, G+4, z, "oak_log[axis=y]")
	for dx := -2; dx <= 2; dx++ {
		for dz := -2; dz <= 2; dz++ {
			for dy := 3; dy <= 6; dy++ {
				r := math.Abs(float64(dx)) + math.Abs(float64(dz)) + math.Abs(float64(dy)-4.5)*0.8
				if r < 3.6 && !(dx == 0 && dz == 0 && dy <= 4) {
					b.set(x+dx, G+dy, z+dz, "oak_leaves[distance=1,persistent=true,waterlogged=false]")
				}
			}
		}
	}
}

func main() {
	out := flag.String("out", "missions/manor", "mission directory to write")
	flag.Parse()
	reg := voxel.NewRegistry()
	level.RegisterBlocks(reg)
	b := &builder{w: voxel.NewWorld(reg, -64, 384), reg: reg}

	// Land: a grassy plateau (area: everything the level occupies) with dirt under it.
	b.fill(-24, 56, -24, 87, 56, 87, "bedrock")
	b.fill(-24, 57, -24, 87, 62, 87, "dirt")
	b.fill(-24, 63, -24, 87, 63, 87, "grass_block[snowy=false]")

	// Outer wall with a gate in the south, gravel drive to the front door.
	b.fill(0, G, 0, 63, G+3, 0, "stone_bricks")
	b.fill(0, G, 63, 63, G+3, 63, "stone_bricks")
	b.fill(0, G, 0, 0, G+3, 63, "stone_bricks")
	b.fill(63, G, 0, 63, G+3, 63, "stone_bricks")
	b.fill(0, G+4, 0, 63, G+4, 0, "cobblestone")
	b.fill(0, G+4, 63, 63, G+4, 63, "cobblestone")
	b.fill(0, G+4, 0, 0, G+4, 63, "cobblestone")
	b.fill(63, G+4, 0, 63, G+4, 63, "cobblestone")
	b.air(30, G, 63, 33, G+4, 63)
	b.fill(29, G, 63, 29, G+5, 64, "stone_bricks") // gate pillars
	b.fill(34, G, 63, 34, G+5, 64, "stone_bricks")
	b.fill(30, G-1, 42, 33, G-1, 70, "gravel")
	for _, t := range [][2]int{{6, 6}, {57, 8}, {6, 56}, {56, 56}, {7, 30}, {57, 31}, {-8, 70}, {72, 66}, {-6, 20}, {74, 12}} {
		b.tree(t[0], t[1])
	}
	for x := 8; x <= 24; x++ { // hedges along the drive
		b.set(x, G, 50, "oak_leaves[distance=1,persistent=true,waterlogged=false]")
		b.set(63-x, G, 50, "oak_leaves[distance=1,persistent=true,waterlogged=false]")
	}
	b.fill(48, G, 52, 52, G, 56, "hay_block") // hay by the stable wall

	// Cellar block, carved below.
	b.fill(13, 56, 9, 50, 62, 42, "cobblestone")
	b.air(38, C, 11, 48, C+4, 29) // C1 wine cellar
	b.air(27, C, 11, 36, C+3, 20) // C2 vault
	b.fill(27, C-1, 11, 36, C-1, 20, "stone_bricks")
	b.fill(38, C-1, 11, 48, C-1, 29, "stone")
	b.fill(31, C, 15, 31, C, 15, "gold_block") // pedestal
	for z := 12; z <= 28; z += 3 {
		b.fill(48, C, z, 48, C+2, z+1, "spruce_planks") // racks
	}

	// The house shell: x 14..49, z 10..41.
	b.fill(14, G-1, 10, 49, G+9, 41, "stone_bricks")
	b.air(15, G, 11, 48, G+3, 40)                       // ground floor
	b.air(15, U, 11, 48, U+3, 40)                       // upper floor
	b.fill(14, U+4, 10, 49, U+4, 41, "dark_oak_planks") // roof
	b.fill(15, G-1, 11, 48, G-1, 40, "stone")           // ground floor floor
	b.fill(15, U-1, 11, 48, U-1, 40, "oak_planks")      // upper floor floor
	b.fill(15, G-1, 11, 36, G-1, 29, "oak_planks")      // dining: wood
	b.fill(38, G-1, 11, 48, G-1, 29, "spruce_planks")   // library
	b.fill(28, G-1, 32, 35, G-1, 39, "red_wool")        // hall rug
	b.fill(15, U-1, 11, 29, U-1, 25, "red_wool")        // bedroom carpet
	b.fill(33, U-1, 11, 34, U-1, 40, "blue_wool")       // gallery runner
	b.fill(15, U-1, 27, 29, U-1, 40, "calcite")         // bath tiles
	// Ground floor walls and doors.
	b.fill(15, G, 30, 36, G+3, 30, "bricks")
	b.fill(26, G, 31, 26, G+3, 40, "bricks")
	b.fill(37, G, 11, 37, G+3, 40, "bricks")
	b.fill(38, G, 30, 48, G+3, 30, "bricks")
	// Upper floor walls.
	b.fill(15, U, 26, 29, U+3, 26, "spruce_planks")
	b.fill(30, U, 11, 30, U+3, 40, "spruce_planks")
	b.fill(37, U, 11, 37, U+3, 40, "spruce_planks")
	// Windows.
	for _, y := range []int{G + 1, G + 2, U + 1, U + 2} {
		for x := 18; x <= 46; x += 7 {
			b.set(x, y, 10, "glass")
			if x != 32 {
				b.set(x, y, 41, "glass")
			}
		}
		for z := 14; z <= 38; z += 8 {
			b.set(14, y, z, "glass")
			b.set(49, y, z, "glass")
		}
	}
	// Stairs up in the library (x 40..44, z 12..13) and down to the cellar
	// (x 47..43, z 27..28).
	for i := 0; i < 4; i++ {
		b.fill(40+i, G, 12, 40+i, G+i, 13, "spruce_planks")
	}
	b.air(40, U-1, 12, 43, U-1, 13)
	b.air(43, G-1, 27, 47, G-1, 28)
	for i := 0; i < 5; i++ {
		x := 47 - i
		b.fill(x, C, 27, x, C+4-i, 28, "cobblestone")
		b.air(x, C+5-i, 27, x, G-1, 28)
	}
	// Furniture.
	b.fill(19, G, 19, 33, G, 21, "oak_planks") // dining table
	b.fill(15, G, 37, 15, G, 40, "furnace")    // kitchen
	b.fill(16, G, 40, 19, G, 40, "crafting_table")
	b.fill(38, G, 39, 41, G, 40, "spruce_planks") // guard room table
	b.fill(48, G, 15, 48, G+3, 26, "bookshelf")   // library shelves
	b.fill(38, G, 14, 38, G+2, 26, "bookshelf")
	b.fill(15, U, 11, 17, U, 14, "white_wool") // bed
	b.fill(15, U, 11, 17, U, 11, "dark_oak_planks")
	b.fill(46, U, 17, 47, U, 24, "dark_oak_planks") // study desk
	b.fill(48, U, 28, 48, U+3, 40, "bookshelf")
	b.fill(21, U, 28, 23, U, 28, "calcite")      // wash basin
	b.fill(15, U, 36, 17, U, 40, "quartz_block") // bath tub (rim)
	b.fill(16, U, 37, 16, U, 39, "light_blue_wool")

	b.save(filepath.Join(*out, "world"))
	writeMission(b, *out)
}

func (b *builder) save(dir string) {
	if err := os.RemoveAll(filepath.Join(dir, "region")); err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "region"), 0o755); err != nil {
		log.Fatal(err)
	}
	st := anvil.OpenStore(dir, true)
	defer st.Close()
	// Light it here so the game loads it lit: the level plus an empty ring
	// (the game's Bounds), with a second ring so the first has neighbours.
	lo, hi := area.Min.X>>4, area.Max.X>>4
	loZ, hiZ := area.Min.Z>>4, area.Max.Z>>4
	for cx := lo - 2; cx <= hi+2; cx++ {
		for cz := loZ - 2; cz <= hiZ+2; cz++ {
			if p := (voxel.ChunkPos{X: cx, Z: cz}); b.w.Chunk(p) == nil {
				b.w.AddChunk(b.w.NewChunk(p))
			}
		}
	}
	n := 0
	for cx := lo - 1; cx <= hi+1; cx++ {
		for cz := loZ - 1; cz <= hiZ+1; cz++ {
			p := voxel.ChunkPos{X: cx, Z: cz}
			c := b.w.Chunk(p)
			c.RecomputeHeights(b.reg.Table())
			if !b.w.TryLightChunk(p) {
				log.Fatalf("chunk %v: neighbours missing", p)
			}
		}
	}
	for cx := lo - 1; cx <= hi+1; cx++ {
		for cz := loZ - 1; cz <= hiZ+1; cz++ {
			c := b.w.Chunk(voxel.ChunkPos{X: cx, Z: cz})
			if err := st.Write(cx, cz, anvil.EncodeChunk(b.w, c)); err != nil {
				log.Fatal(err)
			}
			n++
		}
	}
	log.Printf("wrote %d lit chunks to %s", n, dir)
}

// area is the cells the level occupies (the land's extent).
var area = box(-24, 56, -24, 87, 80, 87)

func v(x, y, z float64) sim.Vec3 { return sim.Vec3{X: x, Y: y, Z: z} }

func wp(x, y, z float64, wait int, face ...float64) sim.Waypoint {
	w := sim.Waypoint{Pos: v(x, y, z), Wait: wait * sim.TicksPerSecond}
	if len(face) > 0 {
		w.Yaw, w.Face = face[0]*math.Pi/180, true
	}
	return w
}

func box(x0, y0, z0, x1, y1, z1 int) sim.Box {
	return sim.Box{Min: sim.Cell{X: x0, Y: y0, Z: z0}, Max: sim.Cell{X: x1, Y: y1, Z: z1}}
}

func writeMission(b *builder, dir string) {
	m := &sim.Mission{
		Title: "Ashworth Manor",
		Briefing: "Lord Ashworth has grown fat on other people's misery. Tonight he dies, and his famous chalice " +
			"leaves the vault with you. Take enough of his silver to make it worth the trip, and be gone through the gate before dawn.",
		Start: v(31.5, G, 72.5), StartYaw: 0, StartTime: 22*3600 + 30*60, TimeScale: 8, LootQuota: 700,
		Exit: box(24, G-1, 66, 39, G+6, 76), Area: area,
		Ammo: map[string]int{sim.AmmoBroadhead: 6, sim.AmmoWater: 4, sim.AmmoNoise: 2, sim.AmmoExplosive: 1},
		Objectives: []sim.Objective{
			{Kind: "kill", Target: "ashworth", Text: "Kill Lord Ashworth"},
			{Kind: "steal", Target: "chalice", Text: "Steal the Ashworth Chalice from the cellar vault"},
			{Kind: "loot", Text: "Steal at least 700 in valuables"},
			{Kind: "exit", Text: "Escape through the front gate"},
			{Kind: "accident", Target: "ashworth", Text: "Make Lord Ashworth's death look like an accident", Optional: true},
			{Kind: "nokill", Text: "Kill no one but Lord Ashworth", Optional: true},
			{Kind: "ghost", Text: "Never get into a fight", Optional: true},
		},
	}
	m.NPCs = []*sim.NPCDef{
		{ID: "aldric", Name: "Guard Aldric", Kind: "guard", Model: "guard", Voice: "generic-guard-a", Pos: v(10.5, G, 58.5), Patrol: []sim.Waypoint{
			wp(10.5, G, 58.5, 3), wp(9.5, G, 8.5, 4, 90), wp(53.5, G, 8.5, 3), wp(53.5, G, 47.5, 4, 270), wp(40.5, G, 46.5, 2), wp(22.5, G, 46.5, 2)}},
		{ID: "bertram", Name: "Guard Bertram", Kind: "guard", Model: "guard", Voice: "generic-guard-b", Pos: v(31.5, G, 36.5), Patrol: []sim.Waypoint{
			wp(31.5, G, 36.5, 6, 180), wp(31.5, G, 26.5, 1), wp(20.5, G, 15.5, 4, 0), wp(34.5, G, 15.5, 4, 90), wp(31.5, G, 26.5, 1)}},
		{ID: "cuthbert", Name: "Guard Cuthbert", Kind: "guard", Model: "guard", Voice: "generic-guard-a", Pos: v(33.5, U, 13.5), Patrol: []sim.Waypoint{
			wp(33.5, U, 13.5, 4), wp(33.5, U, 38.5, 4), wp(42.5, U, 32.5, 5, 90), wp(33.5, U, 25.5, 1)}},
		{ID: "dunstan", Name: "Captain Dunstan", Kind: "guard", Model: "captain", Voice: "unique-captain", Pos: v(43.5, G, 35.5), Keys: []string{"vault"}, Patrol: []sim.Waypoint{
			wp(43.5, G, 35.5, 10, 180), wp(41.5, G, 24.5, 1), wp(41.5, C, 20.5, 2), wp(38.5, C, 15.5, 6, 270), wp(44.5, C, 24.5, 1)}},
		{ID: "edith", Name: "Maid Edith", Kind: "civilian", Model: "servant", Voice: "generic-servant-a", Pos: v(20.5, G, 36.5), Patrol: []sim.Waypoint{
			wp(20.5, G, 36.5, 12, 270), wp(18.5, G, 32.5, 3), wp(25.5, G, 17.5, 8, 180), wp(20.5, G, 26.5, 1)}},
		{ID: "fenwick", Name: "Butler Fenwick", Kind: "civilian", Model: "servant", Voice: "generic-servant-b", Pos: v(22.5, U, 18.5), Patrol: []sim.Waypoint{
			wp(22.5, U, 18.5, 8), wp(33.5, U, 30.5, 4), wp(42.5, U, 36.5, 8, 0), wp(33.5, U, 20.5, 1)}},
		{ID: "ashworth", Name: "Lord Ashworth", Kind: "target", Model: "lord", Voice: "unique-lord", Pos: v(44.5, U, 22.5), Patrol: []sim.Waypoint{
			wp(44.5, U, 22.5, 30, 90), wp(22.5, U, 30.5, 20, 0), wp(20.5, U, 16.5, 25, 270), wp(25.5, G, 13.5, 20, 0), wp(39.5, G, 16.5, 1)}},
	}
	torch := func(id string, x, y, z float64) *sim.Object {
		o := &sim.Object{ID: id, Kind: sim.KindTorch, Name: "Torch", Pos: v(x, y, z), Lit: true}
		o.On = sim.TorchOn(b.wall, o.Pos.Cell())
		return o
	}
	lantern := func(id string, x, y, z float64) *sim.Object { // hangs from the ceiling when there is one
		o := &sim.Object{ID: id, Kind: sim.KindTorch, Name: "Lantern", Pos: v(x, y, z), Lit: true}
		o.On = sim.LanternOn(b.solid, o.Pos.Cell())
		return o
	}
	loot := func(id, name string, value int, x, y, z float64) *sim.Object {
		return &sim.Object{ID: id, Kind: sim.KindLoot, Name: name, Value: value, Pos: v(x, y, z)}
	}
	door := func(id string, x, y, z float64) *sim.Object {
		return &sim.Object{ID: id, Kind: sim.KindDoor, Name: "Door", Pos: v(x, y, z)}
	}
	m.Objects = []*sim.Object{
		// Doors.
		door("front_door", 31.5, G, 41.5), door("kitchen_back", 14.5, G, 35.5),
		door("kitchen_dining", 20.5, G, 30.5), door("hall_dining", 31.5, G, 30.5), door("kitchen_hall", 26.5, G, 35.5),
		door("dining_library", 37.5, G, 20.5), door("hall_guardroom", 37.5, G, 35.5), door("library_guardroom", 43.5, G, 30.5),
		door("bed_bath", 22.5, U, 26.5), door("bed_gallery", 30.5, U, 18.5), door("bath_gallery", 30.5, U, 33.5), door("gallery_study", 37.5, U, 25.5),
		{ID: "vault_door", Kind: sim.KindDoor, Name: "Vault door", Pos: v(37.5, C, 15.5), Locked: true, Key: "vault"},
		// Torches: gate, front door, rooms. Dark corners are left on purpose.
		torch("t_gate_w", 29.5, G+1, 65.5), torch("t_gate_e", 34.5, G+1, 65.5),
		torch("t_front_w", 29.5, G+1, 42.5), torch("t_front_e", 33.5, G+1, 42.5),
		torch("t_hall", 27.5, G+1, 31.5), torch("t_hall2", 35.5, G+1, 39.5),
		torch("t_dining_n", 26.5, G+1, 11.5), torch("t_dining_w", 15.5, G+1, 23.5),
		torch("t_kitchen", 25.5, G+1, 39.5), torch("t_guard", 47.5, G+1, 39.5),
		torch("t_library", 42.5, G+1, 28.5), torch("t_cellar", 42.5, C+1, 11.5), torch("t_vault", 35.5, C+1, 18.5),
		torch("t_bedroom", 18.5, U+1, 24.5), torch("t_gallery_n", 35.5, U+1, 11.5), torch("t_gallery_s", 31.5, U+1, 39.5),
		torch("t_study", 38.5, U+1, 38.5), torch("t_bath", 28.5, U+1, 39.5),
		// Lanterns hang from the ceilings of the big rooms.
		lantern("l_hall", 31.5, G+3, 35.5), lantern("l_dining", 25.5, G+3, 20.5), lantern("l_library", 43.5, G+3, 20.5),
		lantern("l_gallery", 33.5, U+3, 25.5), lantern("l_cellar", 43.5, C+4, 20.5),
		// The accident kit.
		{ID: "faucet", Kind: sim.KindFaucet, Name: "Wash basin tap", Pos: v(22.5, U, 29.5)},
		{ID: "bath_lamp", Kind: sim.KindLamp, Name: "Electric lamp", Pos: v(24.5, U, 31.5), Lit: true},
		{ID: "study_lamp", Kind: sim.KindLamp, Name: "Electric lamp", Pos: v(45.5, U, 21.5), Lit: true},
		{ID: "bed_lamp", Kind: sim.KindLamp, Name: "Electric lamp", Pos: v(18.5, U, 15.5), Lit: true},
		{ID: "dining_rug", Kind: sim.KindFlammable, Name: "Rug", Pos: v(25.5, G, 12.5)},
		{ID: "bed_tapestry", Kind: sim.KindFlammable, Name: "Tapestry", Pos: v(18.5, U, 23.5)},
		{ID: "hay", Kind: sim.KindFlammable, Name: "Hay", Pos: v(47.5, G, 54.5)},
		// Loot.
		{ID: "chalice", Kind: sim.KindLoot, Name: "Ashworth Chalice", Value: 300, Pos: v(31.5, C+1, 15.5)},
		loot("plate1", "Silver plate", 50, 21.5, G+1, 20.5), loot("plate2", "Silver plate", 50, 26.5, G+1, 20.5),
		loot("candelabrum", "Gold candelabrum", 75, 30.5, G+1, 20.5), loot("silverware", "Silverware", 60, 17.5, G+1, 40.5),
		loot("purse", "Coin purse", 40, 39.5, G+1, 40.5), loot("gems", "Gemstones", 150, 46.5, U+1, 19.5),
		loot("idol", "Golden idol", 120, 35.5, U, 22.5), loot("ring", "Signet ring", 90, 47.5, U+1, 23.5),
		loot("wine", "Vintage wine", 45, 47.5, C, 20.5), loot("necklace", "Pearl necklace", 110, 16.5, U+1, 12.5),
		{ID: "jewel_box", Kind: sim.KindChest, Name: "Jewellery chest", Value: 200, Pos: v(28.5, U, 12.5), Locked: true, Key: "jewels"},
		{ID: "vault_chest", Kind: sim.KindChest, Name: "Strongbox", Value: 150, Pos: v(28.5, C, 19.5)},
		{ID: "footlocker", Kind: sim.KindChest, Name: "Footlocker", Pos: v(46.5, G, 32.5)},
		{ID: "pantry", Kind: sim.KindChest, Name: "Pantry chest", Pos: v(16.5, G, 31.5)},
		// Keys and supplies.
		{ID: "vault_key", Kind: sim.KindKey, Name: "Vault key", Key: "vault", Pos: v(40.5, G+1, 39.5)},
		{ID: "jewel_key", Kind: sim.KindKey, Name: "Small brass key", Key: "jewels", Pos: v(46.5, U+1, 22.5)},
		{ID: "ammo_water", Kind: sim.KindAmmo, Name: sim.AmmoWater, Count: 2, Pos: v(18.5, G, 39.5)},
		{ID: "ammo_broad", Kind: sim.KindAmmo, Name: sim.AmmoBroadhead, Count: 4, Pos: v(38.5, G+1, 39.5)},
		{ID: "ammo_noise", Kind: sim.KindAmmo, Name: sim.AmmoNoise, Count: 1, Pos: v(50.5, G, 57.5)},
	}
	m.Rooms = []*sim.Room{
		{ID: 1, Name: "Entrance hall", Boxes: []sim.Box{box(27, G, 31, 36, G+3, 40)}},
		{ID: 2, Name: "Dining hall", Boxes: []sim.Box{box(15, G, 11, 36, G+3, 29)}},
		{ID: 3, Name: "Kitchen", Boxes: []sim.Box{box(15, G, 31, 25, G+3, 40)}},
		{ID: 4, Name: "Guard room", Boxes: []sim.Box{box(38, G, 31, 48, G+3, 40)}},
		{ID: 5, Name: "Library", Boxes: []sim.Box{box(38, G, 11, 48, G+3, 29), box(40, U-1, 12, 43, U-1, 13), box(43, G-1, 27, 47, G-1, 28)}},
		{ID: 6, Name: "Bedroom", Boxes: []sim.Box{box(15, U, 11, 29, U+3, 25)}},
		{ID: 7, Name: "Bathroom", Boxes: []sim.Box{box(15, U, 27, 29, U+3, 40)}},
		{ID: 8, Name: "Gallery", Boxes: []sim.Box{box(31, U, 11, 36, U+3, 40)}},
		{ID: 9, Name: "Study", Boxes: []sim.Box{box(38, U, 11, 48, U+3, 40)}},
		{ID: 10, Name: "Wine cellar", Boxes: []sim.Box{box(38, C, 11, 48, C+4, 29), box(43, C, 27, 47, G-2, 28)}},
		{ID: 11, Name: "Vault", Boxes: []sim.Box{box(27, C, 11, 36, C+3, 20)}},
	}
	if err := m.Save(dir); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote mission %q to %s", m.Title, dir)
}
