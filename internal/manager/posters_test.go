package manager

import (
	"context"
	"sync"
	"testing"
	"time"
)

// Each destination posts in order; a slow one does not hold another.
func TestPosters(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := newPosters(ctx)
	var mu sync.Mutex
	var got []string
	add := func(s string) { mu.Lock(); got = append(got, s); mu.Unlock() }
	block := make(chan struct{})
	p.post("slow", func() { <-block; add("slow1") })
	p.post("slow", func() { add("slow2") })
	p.post("fast", func() { add("fast1") })
	time.Sleep(200 * time.Millisecond)
	mu.Lock()
	if len(got) != 1 || got[0] != "fast1" {
		t.Fatalf("%v", got)
	}
	mu.Unlock()
	close(block)
	time.Sleep(1300 * time.Millisecond) // one a second per destination
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 3 || got[1] != "slow1" || got[2] != "slow2" {
		t.Fatalf("%v", got)
	}
}
