package netmon

import (
	"testing"
	"time"
)

func TestCountAndRate(t *testing.T) {
	m := newMonitor()
	m.Watch([]uint32{100, 200})
	t0 := time.Now()
	m.sample(t0)
	if _, ok := m.Rate(100); ok {
		t.Fatal("a rate before two samples")
	}
	m.count(100, 1000, false)
	m.count(100, 3000, true)
	m.count(300, 5000, true) // not a server: ignored
	m.sample(t0.Add(2 * time.Second))
	r, ok := m.Rate(100)
	if !ok || r.In != 500 || r.Out != 1500 {
		t.Fatalf("rate %+v %v", r, ok)
	}
	if r, ok := m.Rate(200); !ok || r.In != 0 || r.Out != 0 {
		t.Fatalf("idle server %+v %v", r, ok)
	}
	if _, ok := m.Rate(300); ok {
		t.Fatal("unwatched program has a rate")
	}
	if seen, counted := m.Events(); seen != 3 || counted != 2 {
		t.Fatalf("events %d %d", seen, counted)
	}
	// Watching another set keeps the counts of the programs still watched
	c := (*m.watched.Load())[100]
	m.Watch([]uint32{100, 400})
	if (*m.watched.Load())[100] != c {
		t.Fatal("counter replaced")
	}
	m.Watch([]uint32{400, 100}) // same set: nothing changes
	m.count(400, 10, true)
	m.sample(t0.Add(4 * time.Second))
	if _, ok := m.Rate(400); ok {
		t.Fatal("a new program has a rate at its first sample")
	}
	if _, ok := m.Rate(200); ok {
		t.Fatal("a program no longer watched keeps a rate")
	}
	m.Close()
	m.Close()
}

// The machine's traffic: every program's, the servers' or not.
func TestTotals(t *testing.T) {
	m := newMonitor()
	m.total(1000, false)
	m.total(300, true)
	m.total(200, true)
	if in, out := m.Totals(); in != 1000 || out != 500 {
		t.Fatal(in, out)
	}
}

func TestLoopback(t *testing.T) {
	v4 := func(dst, src [4]byte) []byte {
		return append(append(make([]byte, 8), dst[:]...), append(src[:], 0, 0, 0, 0)...)
	}
	v6 := func(dst, src [16]byte) []byte { return append(append(make([]byte, 8), dst[:]...), src[:]...) }
	one := [16]byte{15: 1}
	other := [16]byte{0: 0x20, 1: 0x01, 15: 7}
	for _, c := range []struct {
		id   uint16
		d    []byte
		want bool
	}{
		{tcp4Sent, v4([4]byte{127, 0, 0, 1}, [4]byte{127, 0, 0, 1}), true},
		{udp4Received, v4([4]byte{10, 0, 0, 5}, [4]byte{93, 184, 216, 34}), false},
		{tcp6Received, v6(one, one), true},
		{udp6Sent, v6(other, other), false},
		{tcp4Sent, make([]byte, 8), false}, // too short to tell: counted
	} {
		if got := loopback(c.id, c.d); got != c.want {
			t.Errorf("%d %v: %v", c.id, c.d, got)
		}
	}
}
