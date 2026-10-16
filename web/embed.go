//go:build js

// Package web embeds everything the browser build reads: the game's assets,
// the demo mission and VoxelLib's cleannrooster models, which build_web.sh
// copies to vlassets/ (gitignored) because go:embed cannot leave the module.
package web

import "embed"

//go:embed assets missions vlassets
var Assets embed.FS
