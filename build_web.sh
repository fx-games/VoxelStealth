#!/usr/bin/env bash
# Build the browser version (WebAssembly + WebGL2) into the site's play/ folder.
# The page itself, play/index.html, lives in the site and is not touched here.
set -euo pipefail
cd "$(dirname "$0")"
OUT="${OUT:-/Users/felix/Projects/fx-games/site/voxel-stealth/play}"
mkdir -p "$OUT" web/vlassets web/assets web/missions/manor
# go:embed cannot reach outside the module: mirror what the build needs under web/.
rsync -a --delete --exclude .DS_Store ../VoxelLib/assets/cleannrooster/ web/vlassets/cleannrooster/
rsync -a --delete --exclude .DS_Store --exclude '*.aseprite' assets/ web/assets/
rsync -a --delete --exclude .DS_Store missions/manor/ web/missions/manor/
GOOS=js GOARCH=wasm go build -trimpath -ldflags="-s -w" -o "$OUT/voxelstealth.wasm" ./cmd/voxelstealth
gzip -9 -k -f "$OUT/voxelstealth.wasm"   # nginx gzip_static serves this one
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" "$OUT/"
ls -l "$OUT"/voxelstealth.wasm*
