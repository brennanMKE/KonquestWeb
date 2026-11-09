package main

import "io/fs"

// static returns the embedded web client. The client arrives in milestone 3;
// until then the server shows a placeholder page.
func static() fs.FS { return nil }
