//go:build windows

package crashinfo

import (
	"path/filepath"
	"strings"
)

func equalFold(a, b string) bool { return strings.EqualFold(filepath.Base(a), filepath.Base(b)) }
