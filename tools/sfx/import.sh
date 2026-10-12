#!/bin/bash
# Imports recorded sounds into assets/sfx as mono 44.1 kHz Ogg Vorbis (see
# ASSETS.md for sources and licenses, sound.go loadSFX for naming):
#   tools/sfx/import.sh dasp_v31.zip DynamicSurroundings_R24.zip
# dasp_v31.zip: Dubtrain Angband Sound Pack, http://www.dubtrain.com/angband/
# DynamicSurroundings_R24.zip: Dynamic Surroundings Remastered, CurseForge
#   project 993859 (only its Remastered/ folder is used).
set -e
ang=${1:?usage: $0 dasp_v31.zip DynamicSurroundings_R24.zip}
ds=${2:?usage: $0 dasp_v31.zip DynamicSurroundings_R24.zip}
out=$(cd "$(dirname "$0")/../.." && pwd)/assets/sfx
tmp=$(mktemp -d)
trap 'chmod -R u+w "$tmp"; rm -rf "$tmp"' EXIT
unzip -q "$ang" -d "$tmp/ang"
unzip -q "$ds" -d "$tmp/ds" 'assets/minecraft/sounds/Remastered/*'
angdir=$(dirname "$(find "$tmp/ang" -name "*.wav" -print -quit)")
dsdir=$tmp/ds/assets/minecraft/sounds/Remastered
mkdir -p "$out"
rm -f "$out"/*.ogg

# conv NAME DB FILE...: FILEs become NAME-1.ogg, NAME-2.ogg, ... at peak DB.
conv() {
	name=$1 db=$2
	shift 2
	i=0
	for f in "$@"; do
		i=$((i + 1))
		sox "$f" -r 44100 -c 1 -b 16 "$tmp/mid.wav" \
			silence 1 0.005 0.1% reverse silence 1 0.05 0.1% reverse norm "$db"
		# very short clicks get trimmed to nothing: keep them untrimmed
		if [ "$(soxi -s "$tmp/mid.wav")" -lt 1000 ]; then
			sox "$f" -r 44100 -c 1 -b 16 "$tmp/mid.wav" norm "$db"
		fi
		oggenc -Q -q 4 -o "$out/$name-$i.ogg" "$tmp/mid.wav"
	done
	echo "$name: $i"
}
# seqfiles PREFIX N: PREFIX1.ogg .. PREFIXN.ogg
seqfiles() { for n in $(seq 1 "$2"); do echo "$1$n.ogg"; done; }

# (the pack folder has spaces in its name, so work inside it)
a() { (cd "$angdir" && conv "$1" -3 $(for f in "${@:2}"; do echo "$f.wav"; done)); }
a door plm_door_creak plm_door_open plm_door_opening plm_door_rusty
a lockpick plm_click_dry plm_click_switch plm_click_wood
a unlock plm_lock_case plm_door_echolock plm_chest_unlatch
a loot plm_coins_light plm_coins_shake plm_coins_pour
a shatter plm_glass_smashing plm_break_glass plm_break_shatter plm_glass_breaking
a crash plm_break_plates plm_bang_ceramic plm_break_canister
a clatter plm_drop_boot plm_wood_thud plm_metal_clank
a body mco_thud_crash plc_hit_hay
a thud plc_hit_body plc_hit_hay
a stab mco_squish_hit plc_hit_body
a hit plc_hit_body plc_hit_hay
a arrow plc_hit_arrow plc_miss_arrow2
a zap pls_tone_elec
a hiss mco_attack_spray mco_spray_long

cd "$dsdir"
conv step-stone -8 $(seqfiles blocks/step/stone/stone_walk 11)
conv step-wood -8 $(seqfiles blocks/step/wood/wood_walk 11)
conv step-rug -8 $(seqfiles blocks/step/rug/rug_walk 11)
conv step-grass -8 $(seqfiles blocks/step/grass/grass_walk 10)
conv step-gravel -8 $(seqfiles blocks/step/gravel/gravel_walk 11)
conv step-metal -8 $(seqfiles blocks/step/metal/metalbox_walk 9)
conv step-water -8 $(seqfiles blocks/step/water/water_through 11)
conv twang -3 $(seqfiles items/bow_loose 4)
conv swish -3 $(seqfiles items/sword_swing 4)
conv explosion -1 ambient/effect/explode1.ogg ambient/effect/explode2.ogg ambient/effect/explode3.ogg ambient/effect/explode6.ogg
conv splash -3 $(seqfiles ambient/effect/splash 4)
conv ambient -6 ambient/wind/windnormal.ogg
conv crickets -6 ambient/cricket/cricket-night.ogg

# Freesound originals of the fire sounds Minecraft credits (previews: the
# full files need a login; the -hq preview is 128 kb/s MP3, fine for q4 Ogg).
fs=$tmp/fs
mkdir -p "$fs"
for p in 17/17548_47052 33/33943_298165 65/65174_7037; do
	curl -sfL -m 120 -A "Mozilla/5.0" "https://cdn.freesound.org/previews/$p-hq.mp3" -o "$fs/$(basename "$p").mp3"
done
# Fire_Forest_Inferno (Dynamicell, CC BY 4.0): 2 s crackle cuts from its
# steady middle; the sim plays "fire" every 1.5 s while something burns.
i=0
for t in 10 14 18 22; do
	i=$((i + 1))
	sox "$fs/17548_47052.mp3" -r 44100 -c 1 -b 16 "$tmp/mid.wav" trim "$t" 2 fade t 0.3 2 0.5 norm -6
	oggenc -Q -q 4 -o "$out/fire-$i.ogg" "$tmp/mid.wav"
done
echo "fire: $i"
# flamewind (scarbelly25, CC0): bursting into flame; match_strike (tim.kahn
# a.k.a. Corsica_S, CC BY 4.0).
sox "$fs/33943_298165.mp3" -r 44100 -c 1 -b 16 "$tmp/mid.wav" trim 0 3 fade t 0 3 1 norm -3
oggenc -Q -q 4 -o "$out/ignite-1.ogg" "$tmp/mid.wav"
sox "$fs/65174_7037.mp3" -r 44100 -c 1 -b 16 "$tmp/mid.wav" silence 1 0.005 0.5% reverse silence 1 0.05 0.5% reverse norm -3
oggenc -Q -q 4 -o "$out/ignite-2.ogg" "$tmp/mid.wav"
echo "ignite: 2"
