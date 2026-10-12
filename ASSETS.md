# Asset Ledger

Every art/audio/font file shipped with the game. Allowed: CC0, CC-BY, CC-BY-SA, MIT/BSD/zlib, OFL.
Rejected: NC, ND, personal-use-only, unknown source.

| File(s) | Source URL | Author | License | Added | Modifications |
|---|---|---|---|---|---|
| VoxelLib `assets/rpg16pack/` (textures) | see VoxelLib assets/rpg16pack/CREDITS.txt | Hugues Alexandre Ross | CC BY-SA 4.0 | 2026-10-12 | none (derivatives must stay CC BY-SA) |
| VoxelLib `assets/cleannrooster/` (models, anims) | see VoxelLib assets/cleannrooster/README.md | cleannrooster | MIT | 2026-10-12 | none; placeholder NPCs; crossbow = `repeater_crossbow_arrow` bone of `bb/artmob.bbmodel` |
| VoxelLib `assets/items/` (icons) | Ironchest Games "RPG Items" | Ironchest Games | CC0 | 2026-10-12 | none |
| `assets/voice/*/*.ogg` (108 NPC voice lines, 6 speakers) | https://huggingface.co/hexgrad/Kokoro-82M (weights + voicepacks bm_george, bm_lewis, bm_fable, am_onyx, bf_lily, bm_daniel; voices from mlx-community/Kokoro-82M-bf16 mirror); engine `kokoro` 0.7.4 python pkg; generated offline by `tools/voices/generate.sh` | hexgrad (Kokoro-82M) | Apache-2.0 (model, voicepacks, code); generated audio has no extra terms in the license | 2026-10-12 | TTS synthesis, sox pitch shift per speaker, silence trim, normalize -3 dBFS, Ogg Vorbis q4 (oggenc). Note: G2P uses misaki + espeak-ng (GPL-3.0) as a tool only; nothing GPL shipped. Training-data provenance of voicepacks not documented by author — low risk, verify before release |
| `assets/sfx/{door,lockpick,unlock,loot,shatter,crash,clatter,body,thud,stab,hit,arrow,zap,hiss}-N.ogg` (36 files) | http://www.dubtrain.com/angband/ (dasp_v31.zip, Dubtrain Angband Sound Pack v3.1.0) | Pete / Dubtrain Records | CC BY 4.0 (relicensed on the site 2023-12-23; the readme inside the zip still says BY-NC-SA and is superseded) | 2026-10-12 | `tools/sfx/import.sh`: mono 16-bit 44.1 kHz, silence trim, peak -3 dBFS, Ogg Vorbis q4. Credit "Dubtrain Records" in the game credits |
| `assets/sfx/{step-*,twang,swish,explosion,splash,ambient,crickets}-N.ogg` (92 files) | https://www.curseforge.com/minecraft/texture-packs/dynamic-surroundings-remastered (project 993859, file DynamicSurroundings_R24.zip, 2026-09-19), only its `Remastered/` folder | AS12 / Dynamic Surroundings Sounds Team; original sounds from Dynamic Surroundings (OreCruncher), Presence Footsteps (Sollace) | Per the pack page: "All assets follow the MIT License, GNU License, and CC0"; project license GPLv3. Per-file provenance is not given by the pack, and some are said to come from "royalty-free sound libraries" — accepted on the user's decision (2026-10-12); verify before release. The pack's non-`Remastered/` folders mirror Minecraft's own sound paths and were NOT used | 2026-10-12 | `tools/sfx/import.sh`: mono 44.1 kHz, silence trim, peak -3 dBFS (footsteps -8, loops -6, explosions -1), Ogg Vorbis q4 |

Never ship: Minecraft textures/models, CubePets creatures (redistribution forbidden).
