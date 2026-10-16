//go:build js

package main

// The browser build reads its assets out of the binary. os on js/wasm reads
// files through Node's fs API; play/index.html implements it in JavaScript
// over the table init() hands it here: every embedded file's path and where
// its bytes sit in wasm memory. Nothing is copied. Files the game writes
// (quicksaves) live in the page's memory until reload.

import (
	"os"
	"syscall/js"
	"unsafe"

	"github.com/memmaker/voxelstealth/web"
)

// embedFile is how the toolchain lays out an embed.FS entry: a name (ending in
// "/" for a directory) and the bytes as a string in the data segment.
type embedFile struct {
	name, data string
	hash       [16]byte
}

func init() {
	files := **(**[]embedFile)(unsafe.Pointer(&web.Assets))
	table := make([]any, 0, 3*len(files))
	for _, f := range files {
		table = append(table, f.name, uintptr(unsafe.Pointer(unsafe.StringData(f.data))), len(f.data))
	}
	js.Global().Call("__mountFS", table)

	// The embedded tree is mounted at the working directory, so the defaults
	// for -textures, -mission and friends hold; VoxelLib's assets are copied
	// under web/ by build_web.sh.
	os.Args = append(os.Args, "-assets=vlassets")
	// The page's query string is the command line: ?pos=10,70,10 runs as -pos=10,70,10.
	params := js.Global().Get("Array").Call("from", js.Global().Get("URLSearchParams").New(js.Global().Get("location").Get("search")))
	for i := 0; i < params.Length(); i++ {
		os.Args = append(os.Args, "-"+params.Index(i).Index(0).String()+"="+params.Index(i).Index(1).String())
	}
}
