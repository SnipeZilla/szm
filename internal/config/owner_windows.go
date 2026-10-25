package config

import "os"

// keepOwner: nothing to do on Windows (see owner_unix.go).
func keepOwner(string, os.FileInfo) {}
