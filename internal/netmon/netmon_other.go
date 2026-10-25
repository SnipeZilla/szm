//go:build !linux && (!windows || !(amd64 || arm64))

package netmon

// Start is not available here: Windows (64-bit) and Linux only.
func Start() (*Monitor, error) { return nil, ErrUnsupported }
