# Assets to bring in

Everything below is improvised in code today (box models in
`cmd/voxelstealth/draw.go`, synthesized audio in `cmd/voxelstealth/sound.go`).
Already real and licensed: block textures, NPC models, crossbow, item icons,
bolt model (see ASSETS.md).

**License rule:** commercial use + redistribution allowed (CC0, CC-BY,
CC-BY-SA, MIT/BSD/zlib, OFL). No NC, no ND, no personal-use-only, no
Minecraft or CubePets material. Note CC-BY-SA spreads to derivatives. Log every
file in ASSETS.md (source URL, author, license, modifications).

Voice lines: see TODO-VOICE.md.

## Props (replace box models)

- [x] Wall torch: lit (animated flame) and unlit
- [x] Standing electric lamp: on (lit shade) and off; also lying knocked over — Watoosh lamp post glTF, own texture
- [ ] Faucet + running water stream (animated)
- [x] Chest with opening lid — Stripped (Redone) Chests texture; the lid swings open
- [x] Lantern (torch variant, floor or ceiling) — WaterColourCG glTF, own texture
- [ ] Rug / tapestry / hay (flammables) and their burnt versions
- [x] Fire: animated flames (sprite sheet or particles) — particles (fx.go)
- [x] ~~Puddle decal~~ — puddles are `water[level=7]` blocks, meshed with Minecraft's sloped corner heights (VoxelLib); live puddles spark (particles)
- [ ] Flash bomb (thrown item and in hand)
- [ ] Noisemaker arrow head
- [ ] Blackjack (first person)
- [ ] Blade / dagger (first person)
- [ ] Hand-held torch (first person, ideally with a hand)

## Characters

- [ ] NPC models fitting a manor: guard, guard captain, servant (male and
      female), noble lord. Currently borrowed fantasy mobs (templar, city
      guardian, peasant, fire archmage).
- [ ] Animations: death, knockout, carried over the shoulder, searching,
      alerted/combat, opening doors, cowering. Bodies now tip over rigidly.
- [ ] First-person player hands/arms holding each weapon

## Effects

- [x] Glass shards (window breaking)
- [x] Fire and smoke particles
- [x] Water splash (water arrow, puddles)
- [x] Sparks / electric arcs
- [ ] Flash bomb blast (now only a white screen flash)
- [x] Blood (hits)
- [x] Bolt impact dust / chips

## UI

- [x] Light gem artwork — DKZeuss Gemstones emerald, dimmed by visibility
- [ ] Font, period style but readable (OFL); now Go's 7x13 bitmap font
- [ ] Main menu, loading/briefing screen, end screen artwork
- [ ] Health bar and ammo icons

## Sound effects (replace synthesized sounds)

Mono WAV or OGG, 44.1 kHz. Several variations each where noted, so repeats
don't grate.

- [x] Footsteps, 4+ variations per surface: stone, wood, carpet, grass,
      water/puddle; walking, running, crouching — Dynamic Surroundings (9–11 each; run/crouch differ only in loudness)
- [x] Crossbow: shot (twang) — Dynamic Surroundings; [x] reload — freesound/MadPanCake (ballista draw), freesound/nikerk (ratchet crank)
- [x] Bolt impacts: stone (2, Angband), flesh (2, Angband); [x] wood (3) — freesound/JoeDinesSound, Twisted_Euphoria, plantmonkey
- [x] Blackjack: swing (sword swing, Dynamic Surroundings), hit (Angband)
- [x] Blade: swing (Dynamic Surroundings), stab (Angband)
- [x] Body: falling to the floor, dropped (Angband); hidden in a chest uses the same
- [x] Door: open creak (4, Angband); close uses the same (one sim sound)
- [x] Lock: lockpicking clicks, unlock (Angband)
- [x] Loot pickup: coins (3, Angband)
- [x] Clatter: thrown torch / dropped object (Angband)
- [x] Lamp crashing over (Angband ceramic breaks)
- [x] Glass shattering (4, Angband)
- [x] Water arrow splash (Dynamic Surroundings); torch doused hiss (Angband spray — check it)
- [x] Faucet running (2 chunks) — freesound/DigPro120, danhelbling; squeaky tap handle (2) — freesound/brunoboselli
- [x] Fire: crackle (4 cuts, freesound/Dynamicell), ignition whoosh + match strike (freesound)
- [x] Electric zap (Angband)
- [x] Flash bomb: blast (Dynamic Surroundings); [x] fuse hiss — freesound/elonen
- [x] Noisemaker arrow: repeating chirp/rattle — freesound/monotraum (toy ratchet)
- [x] Ambience: outdoor night wind + crickets loops (Dynamic Surroundings); [x] indoor room tone loop — freesound/kyles

## Music

- [ ] Quiet tense exploration track (loopable)
- [ ] Alert/search layer or track
- [ ] Mission complete / failed stingers
