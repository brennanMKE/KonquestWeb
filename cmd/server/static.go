//go:build !embed

package main

import "io/fs"

// static returns nil without the embed tag, so the server shows a
// placeholder page. `make build` builds with the client embedded.
func static() fs.FS { return nil }
