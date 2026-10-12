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

- [ ] Wall torch: lit (animated flame) and unlit
- [ ] Standing electric lamp: on (lit shade) and off; also lying knocked over
- [ ] Faucet + running water stream (animated)
- [ ] Chest with opening lid
- [ ] Rug / tapestry / hay (flammables) and their burnt versions
- [ ] Fire: animated flames (sprite sheet or particles)
- [ ] Puddle decal, and electrified puddle (arcs)
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

- [ ] Glass shards (window breaking)
- [ ] Fire and smoke particles
- [ ] Water splash (water arrow, puddles)
- [ ] Sparks / electric arcs
- [ ] Flash bomb blast (now only a white screen flash)
- [ ] Blood (hits)
- [ ] Bolt impact dust / chips

## UI

- [ ] Light gem artwork (now drawn per pixel)
- [ ] Font, period style but readable (OFL); now Go's 7x13 bitmap font
- [ ] Main menu, loading/briefing screen, end screen artwork
- [ ] Health bar and ammo icons

## Sound effects (replace synthesized sounds)

Mono WAV or OGG, 44.1 kHz. Several variations each where noted, so repeats
don't grate.

- [ ] Footsteps, 4+ variations per surface: stone, wood, carpet, grass,
      water/puddle; walking, running, crouching (footsteps are muted now)
- [ ] Crossbow: shot (twang), reload
- [ ] Bolt impacts: stone, wood, flesh (3 variations each)
- [ ] Blackjack: swing, hit (head knock)
- [ ] Blade: swing, stab
- [ ] Body: falling to the floor, dropped, hidden in a chest
- [ ] Door: open creak, close thud (wood)
- [ ] Lock: lockpicking clicks (loopable), unlock
- [ ] Loot pickup: coins, metal objects, gems
- [ ] Clatter: thrown torch / dropped object
- [ ] Lamp crashing over
- [ ] Glass shattering (very loud; window)
- [ ] Water arrow splash; torch doused hiss
- [ ] Faucet running (loop); squeaky tap handle
- [ ] Fire: crackle loop, ignition whoosh
- [ ] Electric zap
- [ ] Flash bomb: fuse hiss, blast
- [ ] Noisemaker arrow: repeating chirp/rattle
- [ ] Ambience: outdoor night wind/crickets loop, indoor room tone loop

## Music

- [ ] Quiet tense exploration track (loopable)
- [ ] Alert/search layer or track
- [ ] Mission complete / failed stingers
