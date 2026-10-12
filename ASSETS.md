# Asset Ledger

Every art/audio/font file shipped with the game. Allowed: CC0, CC-BY, CC-BY-SA, MIT/BSD/zlib, OFL.
Rejected: NC, ND, personal-use-only, unknown source.

| File(s) | Source URL | Author | License | Added | Modifications |
|---|---|---|---|---|---|
| VoxelLib `assets/rpg16pack/` (textures) | see VoxelLib assets/rpg16pack/CREDITS.txt | Hugues Alexandre Ross | CC BY-SA 4.0 | 2026-10-12 | none (derivatives must stay CC BY-SA) |
| VoxelLib `assets/cleannrooster/` (models, anims) | see VoxelLib assets/cleannrooster/README.md | cleannrooster | MIT | 2026-10-12 | none; placeholder NPCs; crossbow = `repeater_crossbow_arrow` bone of `bb/artmob.bbmodel` |
| VoxelLib `assets/items/` (icons) | Ironchest Games "RPG Items" | Ironchest Games | CC0 | 2026-10-12 | none |
| `assets/voice/*/*.ogg` (108 NPC voice lines, 6 speakers) | https://huggingface.co/hexgrad/Kokoro-82M (weights + voicepacks bm_george, bm_lewis, bm_fable, am_onyx, bf_lily, bm_daniel; voices from mlx-community/Kokoro-82M-bf16 mirror); engine `kokoro` 0.7.4 python pkg; generated offline by `tools/voices/generate.sh` | hexgrad (Kokoro-82M) | Apache-2.0 (model, voicepacks, code); generated audio has no extra terms in the license | 2026-10-12 | TTS synthesis, sox pitch shift per speaker, silence trim, normalize -3 dBFS, Ogg Vorbis q4 (oggenc). Note: G2P uses misaki + espeak-ng (GPL-3.0) as a tool only; nothing GPL shipped. Training-data provenance of voicepacks not documented by author — low risk, verify before release |

Never ship: Minecraft textures/models, CubePets creatures (redistribution forbidden).
