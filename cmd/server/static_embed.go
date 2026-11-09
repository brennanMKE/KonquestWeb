//go:build embed

package main

import (
	"io/fs"

	"parallax/web"
)

// static returns the embedded web client.
func static() fs.FS { return web.Dist() }
