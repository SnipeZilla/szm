package manager

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SnipeZilla/szm/internal/config"
	"github.com/SnipeZilla/szm/internal/events"
)

func TestAppendCapped(t *testing.T) {
	p := filepath.Join(t.TempDir(), "memory.txt")
	for i := 0; i < 100; i++ {
		appendCapped(p, strings.Repeat("x", 20)+"\n", 500)
	}
	b, _ := os.ReadFile(p)
	if len(b) > 600 || len(b) < 200 || !strings.HasPrefix(string(b), "xxxx") {
		t.Fatal(len(b), string(b[:30]))
	}
}

func TestMemoryJump(t *testing.T) {
	dir := t.TempDir()
	m := &Manager{DataDir: dir, Cfg: &config.Config{}, Log: events.New(filepath.Join(dir, "logs")), Started: time.Now()}
	_ = os.MkdirAll(filepath.Join(dir, "logs"), 0o755)
	var mem atomic.Uint64
	mem.Store(20 << 20)
	oldSelf, oldCheck := selfMemory, memCheckEvery
	selfMemory, memCheckEvery = func() uint64 { return mem.Load() }, 20*time.Millisecond
	defer func() { selfMemory, memCheckEvery = oldSelf, oldCheck }()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { m.logMemory(ctx); close(done) }()
	time.Sleep(60 * time.Millisecond)
	m.Log.Add(events.Restart, "tf2-1", "TF2 - Server 1", "Restarting (running for 24 hours)")
	m.Note("GET /api/servers/tf2-1/files")
	mem.Store(52 << 20)
	time.Sleep(80 * time.Millisecond)
	cancel()
	<-done
	b, _ := os.ReadFile(filepath.Join(dir, "logs", "memory.txt"))
	s := string(b)
	if !strings.Contains(s, "JUMP +32.0 MB") || !strings.Contains(s, "TF2 - Server 1: Restarting") || !strings.Contains(s, "asked ") {
		t.Fatal(s)
	}
	if strings.Count(s, "JUMP") != 1 {
		t.Fatal("logged more than once:", s)
	}
}
