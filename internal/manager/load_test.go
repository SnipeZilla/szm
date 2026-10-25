package manager

import (
	"runtime"
	"testing"
	"time"
)

func TestMachineLoad(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("/proc")
	}
	m := &Manager{}
	if m.MachineLoad() != nil {
		t.Fatal("a measure before any")
	}
	m.measureLoad(time.Now())
	end := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(end) { // some CPU to see
	}
	m.measureLoad(time.Now())
	l := m.MachineLoad()
	if l == nil || l.CPU <= 0 || l.CPU > 100 || !l.Net || l.In < 0 {
		t.Fatalf("%+v", l)
	}
	t.Logf("%+v", l)
}
