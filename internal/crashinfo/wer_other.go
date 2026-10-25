//go:build !windows

package crashinfo

import "errors"

// EnableDumps: Windows only (on Linux, the kernel's record names the module).
func EnableDumps(folder string, programs []string, keep int) error {
	return errors.New("crash dumps are set up on Windows only")
}
