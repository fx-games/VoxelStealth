#!/bin/sh
# Build and run the game. Builds the demo mission and the NPC model on first
# run if they are missing. Extra args go to the game:
#   ./build_n_run.sh -edit      ./build_n_run.sh -mute -pos 10.5,64,60
set -e
cd "$(dirname "$0")"
if [ ! -d missions/manor/world ]; then go run ./cmd/buildmanor; fi
if [ ! -f assets/models/player.gltf ]; then go run ./cmd/buildplayer; fi
go build -o voxelstealth ./cmd/voxelstealth
exec ./voxelstealth "$@"
