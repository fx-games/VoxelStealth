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

# More freesound previews (all CC0 1.0, verified on each sound's page):
#   660765 "Loading a Ballista" (MadPanCake)        812058/812059 "Ratchet strap-8/-9" (nikerk)
#   534956 "ARROW_WOOD_IMPACT_SINGLE_ARCHERY_01.wav" (JoeDinesSound)
#   205938 "Arrow Impact" (Twisted_Euphoria)        410751 "Arrow strike.mp3" (plantmonkey)
#   45650 "fuse_burning_48khz.wav" (elonen)         558859 "Faucet running.WAV" (DigPro120)
#   272389 "Sink faucet Running.wav" (danhelbling)  441096 "Squeak Rusty Metal Water Tap" (brunoboselli)
#   376195 "toy ratchet.wav" (monotraum)            637808 "room tone small space quiet.flac" (kyles)
for p in 660/660765_5334162 812/812058_2039537 812/812059_2039537 534/534956_579267 \
	205/205938_3842302 410/410751_182472 45/45650_45914 558/558859_7414526 272/272389_4783367 \
	441/441096_300738 376/376195_1756543 637/637808_612689; do
	curl -sfL -m 120 -A "Mozilla/5.0" "https://cdn.freesound.org/previews/$p-hq.mp3" -o "$fs/$(basename "$p").mp3"
done
# cut NAME-N IN EFFECTS...: one sox pass (44.1 kHz mono 16-bit) then Ogg q4.
# (downmix first: norm after an implicit stereo->mono mix undershoots)
cut() {
	sox "$2" -r 44100 -c 1 -b 16 "$tmp/mid.wav" channels 1 "${@:3}"
	oggenc -Q -q 4 -o "$out/$1.ogg" "$tmp/mid.wav"
}
trimsil='silence 1 0.005 0.1% reverse silence 1 0.05 0.1% reverse'
# reload: ballista string being drawn; two ratchet-strap cranks joined (gaps >0.15 s removed).
cut reload-1 "$fs/660765_5334162.mp3" $trimsil norm -3
sox "$fs/812058_2039537.mp3" "$fs/812059_2039537.mp3" -r 44100 -c 1 -b 16 "$tmp/mid.wav" channels 1 silence -l 1 0.005 0.1% -1 0.15 0.1% norm -3
oggenc -Q -q 4 -o "$out/reload-2.ogg" "$tmp/mid.wav"
echo "reload: 2"
# arrow-wood: longbow arrow into a wooden shed; synthesized wood impact; ruler twang (the wobble).
cut arrow-wood-1 "$fs/534956_579267.mp3" trim 0.08 0.6 fade t 0 0.6 0.2 norm -3
cut arrow-wood-2 "$fs/205938_3842302.mp3" $trimsil norm -3
cut arrow-wood-3 "$fs/410751_182472.mp3" trim 0.75 1.5 fade t 0.01 1.5 0.3 norm -3
echo "arrow-wood: 3"
# fuse: 0.5 s chunk (sim repeats it every 0.5 s); water: 2.5 s chunks (every 2.25 s).
cut fuse-1 "$fs/45650_45914.mp3" trim 3 0.5 fade t 0.05 0.5 0.05 norm -6
echo "fuse: 1"
cut water-1 "$fs/558859_7414526.mp3" trim 4 2.5 fade t 0.1 2.5 0.1 norm -6
cut water-2 "$fs/272389_4783367.mp3" trim 2 2.5 fade t 0.1 2.5 0.1 norm -6
echo "water: 2"
# squeak: two single turns of the rusty tap; noisemaker: 0.4 s of toy-ratchet clicking (every 0.4 s).
cut squeak-1 "$fs/441096_300738.mp3" trim 0.98 0.5 fade t 0.01 0.5 0.05 norm -3
cut squeak-2 "$fs/441096_300738.mp3" trim 2.88 0.4 fade t 0.01 0.4 0.05 norm -3
echo "squeak: 2"
cut noisemaker-1 "$fs/376195_1756543.mp3" trim 6 0.4 fade t 0.01 0.4 0.01 norm -3
echo "noisemaker: 1"
# roomtone: 30 s from the steady middle, 0.5 s fades so the loop seam is silent.
cut roomtone-1 "$fs/637808_612689.mp3" trim 20 30 fade t 0.5 30 0.5 norm -12
echo "roomtone: 1"
