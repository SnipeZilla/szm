package events

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The crashes of the last 24 h: those added, those of linked machines, and those of the files after a restart.
func TestCrashes24h(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	// what an earlier run wrote: yesterday (one 30 h ago, one 2 h ago at most) and today
	write := func(e Event) {
		f, _ := os.OpenFile(filepath.Join(dir, e.Time.Format("2006-01-02")+".log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		b, _ := json.Marshal(e)
		f.Write(append(b, '\n'))
		f.Close()
	}
	write(Event{Time: now.Add(-30 * time.Hour), Kind: Crash, Server: "a"}) // too old
	write(Event{Time: now.Add(-23 * time.Hour), Kind: Crash, Server: "a"})
	write(Event{Time: now.Add(-1 * time.Hour), Kind: Start, Server: "a"})
	write(Event{Time: now.Add(-1 * time.Hour), Kind: Crash, Server: "b"})
	l := New(dir)
	if n := l.Crashes24h(nil); n != 2 {
		t.Fatal("from the files:", n)
	}
	l.Add(Crash, "c", "C", "crashed")
	l.Add(Info, "c", "C", "something")
	l.AddRemote(Event{Time: time.Now(), Kind: Crash, Server: "d", Machine: "eu"})
	if n := l.Crashes24h(nil); n != 4 {
		t.Fatal(n)
	}
	if n := l.Crashes24h(func(e Event) bool { return e.Server == "b" }); n != 1 {
		t.Fatal("of the servers a user sees:", n)
	}
}
