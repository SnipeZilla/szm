//go:build linux

package crashinfo

import (
	"testing"
	"time"
)

// SystemRecords must come back at once, whether the kernel log can be read or not.
func TestSystemRecordsReturns(t *testing.T) {
	done := make(chan struct{})
	go func() {
		SystemRecords(1, []string{"srcds_linux"}, time.Now().Add(-time.Hour))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("SystemRecords waits")
	}
}
