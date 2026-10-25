package manager

import (
	"context"
	"github.com/SnipeZilla/szm/internal/config"
	"github.com/SnipeZilla/szm/internal/events"
	"github.com/SnipeZilla/szm/internal/query"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPatchGameinfo(t *testing.T) {
	p := filepath.Join(t.TempDir(), "gameinfo.gi")
	os.WriteFile(p, []byte("\t\t{\r\n\t\t\tGame_LowViolence\tcsgo_lv // Perfect World content override\r\n\t\t\tGame\tcsgo\r\n"), 0o644)
	changed, err := PatchGameinfo(p, "csgo/addons/metamod")
	if err != nil || !changed {
		t.Fatal(changed, err)
	}
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), "csgo_lv // Perfect World content override\r\n\t\t\tGame\tcsgo/addons/metamod\r\n\t\t\tGame\tcsgo") {
		t.Fatalf("%q", b)
	}
	if changed, _ := PatchGameinfo(p, "csgo/addons/metamod"); changed {
		t.Fatal("patched twice")
	}
}

func TestCopyTree(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	write := func(dir, rel, s string) {
		p := filepath.Join(dir, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(s), 0o644)
	}
	write(src, "cstrike/cfg/server.cfg", "default")
	write(src, "cstrike/bin/server.dll", "new")
	write(src, "steamapps/appmanifest.acf", "x")
	write(dst, "cstrike/cfg/server.cfg", "mine")
	write(dst, "cstrike/bin/server.dll", "old")
	old := time.Now().Add(-time.Hour)
	os.Chtimes(filepath.Join(dst, "cstrike/bin/server.dll"), old, old)
	st, err := CopyTree(context.Background(), src, dst, ExcludeMatcher([]string{"server.cfg"}))
	if err != nil || st.Copied != 1 || st.Kept != 1 {
		t.Fatal(st, err)
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "cstrike/cfg/server.cfg")); string(b) != "mine" {
		t.Fatal("excluded file overwritten")
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "cstrike/bin/server.dll")); string(b) != "new" {
		t.Fatal("not copied")
	}
	if _, err := os.Stat(filepath.Join(dst, "steamapps")); err == nil {
		t.Fatal("steamapps copied")
	}
	st, _ = CopyTree(context.Background(), src, dst, ExcludeMatcher(nil))
	if st.Copied != 1 || st.Same != 1 { // server.cfg differs, server.dll same now
		t.Fatal(st)
	}
}

func TestExcludeMatcher(t *testing.T) {
	m := ExcludeMatcher([]string{"server.cfg", "gamemode_*.cfg", "Server_*", "csgo/addons", "tf/cfg/*.txt", " ", "[bad"})
	for rel, want := range map[string]bool{
		"cstrike/cfg/server.cfg":                        true,
		"game/csgo/cfg/gamemode_casual.cfg":             true,
		"game/csgo/cfg/gamemode_competitive_server.cfg": true,
		"cstrike/cfg/server_ffa.cfg":                    true,
		"cstrike/SERVER_1v1.cfg":                        true,
		"csgo/addons/metamod/x.dll":                     true,
		"tf/cfg/mapcycle.txt":                           true,
		"tf/cfg/sub/mapcycle.txt":                       false,
		"game/csgo/cfg/gamemodes.txt":                   false,
		"game/bin/win64/server.dll":                     false,
		"cstrike/steam.inf":                             false,
	} {
		if got := m(rel); got != want {
			t.Errorf("%s: %v, want %v", rel, got, want)
		}
	}
}

func TestPatchGameinfoWithoutLowViolence(t *testing.T) {
	p := filepath.Join(t.TempDir(), "gameinfo.gi")
	os.WriteFile(p, []byte("\t\tSearchPaths\n\t\t{\n\t\t\tGame\tcsgo // the game\n\t\t\tGame\tcore\n"), 0o644)
	if changed, err := PatchGameinfo(p, "csgo/addons/metamod"); err != nil || !changed {
		t.Fatal(changed, err)
	}
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), "{\n\t\t\tGame\tcsgo/addons/metamod\n\t\t\tGame\tcsgo // the game") {
		t.Fatalf("%q", b)
	}
}

func TestArchivePrefix(t *testing.T) {
	for want, names := range map[string][]string{
		"":                {"addons/metamod/bin/server.dll", "addons/metamod.vdf"},
		"mmsource-2.0.0/": {"mmsource-2.0.0/addons/metamod/bin/server.dll", "mmsource-2.0.0/README.txt"},
		"game/csgo/":      {"game/csgo/addons/counterstrikesharp/api/x.dll", "game/csgo/addons/metamod/counterstrikesharp.vdf"},
		"sourcemod-1.12/": {"sourcemod-1.12/addons/sourcemod/bin/x.dll", "sourcemod-1.12/cfg/sourcemod/sm.cfg"},
	} {
		if got := archivePrefix(names); got != want {
			t.Errorf("%v: %q, want %q", names, got, want)
		}
	}
}

// A push leaves a protected folder the server has as it is: a plugin removed from it does not come back.
func TestKeepTree(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	write := func(dir, rel, s string) {
		p := filepath.Join(dir, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(s), 0o644)
	}
	for _, f := range []string{"plugins/admin.smx", "plugins/funcommands.smx", "plugins/disabled/x.smx", "configs/core.cfg", "bin/sm.so", "data/a.db"} {
		write(src, "addons/sourcemod/"+f, "new "+f)
	}
	keep := ExcludeMatcher([]string{"addons/sourcemod/plugins", "addons/sourcemod/configs", "*.db"})
	// a first push: the server has nothing, everything comes
	st, err := KeepTree(context.Background(), src, dst, keep)
	if err != nil || st.Copied != 6 {
		t.Fatal(st, err)
	}
	// the admin removes a plugin, edits a config; a new version of the package comes
	os.Remove(filepath.Join(dst, "addons/sourcemod/plugins/funcommands.smx"))
	write(dst, "addons/sourcemod/configs/core.cfg", "mine")
	os.Remove(filepath.Join(dst, "addons/sourcemod/data/a.db"))
	for _, f := range []string{"plugins/admin.smx", "plugins/funcommands.smx", "plugins/new.smx", "configs/core.cfg", "configs/new.cfg", "bin/sm.so"} {
		write(src, "addons/sourcemod/"+f, "v2 "+f)
	}
	st, err = KeepTree(context.Background(), src, dst, keep)
	if err != nil {
		t.Fatal(err)
	}
	for f, want := range map[string]string{"plugins/admin.smx": "new plugins/admin.smx", "plugins/funcommands.smx": "", "plugins/new.smx": "",
		"configs/core.cfg": "mine", "configs/new.cfg": "", "bin/sm.so": "v2 bin/sm.so", "data/a.db": "new data/a.db"} {
		b, err := os.ReadFile(filepath.Join(dst, "addons/sourcemod", f))
		if (want == "") != (err != nil) || string(b) != want {
			t.Errorf("%s: %q %v", f, b, err)
		}
	}
	if st.Copied != 2 || st.Kept != 6 { // bin/sm.so and the missing a.db; the plugins (4) and configs (2) kept
		t.Fatal(st)
	}
}

// The copy says how far it is: bytes done (copied or already up to date) of the total, ending at the total.
func TestCopyProgress(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	_ = os.MkdirAll(filepath.Join(src, "game", "maps"), 0o755)
	_ = os.WriteFile(filepath.Join(src, "game", "big.vpk"), make([]byte, 3<<20), 0o644)
	_ = os.WriteFile(filepath.Join(src, "game", "maps", "a.bsp"), make([]byte, 1000), 0o644)
	_ = os.MkdirAll(filepath.Join(src, "steamapps"), 0o755)
	_ = os.WriteFile(filepath.Join(src, "steamapps", "x.acf"), make([]byte, 5000), 0o644) // not copied, not counted
	var last [3]int64
	calls := 0
	st, err := copyTree(context.Background(), src, dst, ExcludeMatcher(nil), false, func(done, copied, total int64) {
		last = [3]int64{done, copied, total}
		calls++
	})
	if err != nil || st.Copied != 2 {
		t.Fatal(st, err)
	}
	if want := int64(3<<20 + 1000); last != [3]int64{want, want, want} || calls < 2 {
		t.Fatal(last, calls)
	}
	// again: nothing to copy, all done at once
	st, _ = copyTree(context.Background(), src, dst, ExcludeMatcher(nil), false, func(done, copied, total int64) { last = [3]int64{done, copied, total} })
	if st.Same != 2 || last[0] != last[2] || last[1] != 0 {
		t.Fatal(st, last)
	}
}

// One server copies at a time; a server waiting for its turn says so, and can be cancelled.
func TestCopyTurn(t *testing.T) {
	a, b := newInstance("a"), newInstance("b")
	done, err := waitCopyTurn(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	got := make(chan error, 1)
	go func() {
		next, err := waitCopyTurn(ctx, b)
		if err == nil {
			next()
		}
		got <- err
	}()
	time.Sleep(50 * time.Millisecond)
	b.mu.Lock()
	stage := b.stage
	b.mu.Unlock()
	if !strings.Contains(stage, "its turn") {
		t.Fatal(stage)
	}
	cancel()
	if err := <-got; err == nil {
		t.Fatal("not cancelled")
	}
	done()
	next, err := waitCopyTurn(context.Background(), b) // free again
	if err != nil {
		t.Fatal(err)
	}
	next()
}

// Two servers updating from one shared copy: both warned at once; the second, waiting for its turn, tells its players
// it follows, then that its update begins.
func TestWarnWhileQueued(t *testing.T) {
	dir := t.TempDir()
	m := &Manager{DataDir: dir, Log: events.New(filepath.Join(dir, "logs")), Cfg: &config.Config{}}
	m.Cfg.Updates.SayEvery = 1
	var mu sync.Mutex
	said := map[string][]string{}
	old := playerSay
	playerSay = func(_ *Manager, s config.Server, msg string) {
		mu.Lock()
		said[s.ID] = append(said[s.ID], msg)
		mu.Unlock()
	}
	defer func() { playerSay = old }()
	up := func(id string) *Instance {
		i := newInstance(id)
		i.pid, i.info = 1, &query.Info{Players: 3}
		return i
	}
	a, b := up("a"), up("b")
	start := time.Now()
	gotA := make(chan func(), 1)
	go func() {
		next, err := m.warnWhileQueued(context.Background(), a, config.Server{ID: "a"}, true, 4)
		if err != nil {
			t.Error(err)
		}
		gotA <- next
	}()
	time.Sleep(100 * time.Millisecond) // a has the turn
	doneB := make(chan error, 1)
	go func() {
		next, err := m.warnWhileQueued(context.Background(), b, config.Server{ID: "b"}, true, 4)
		if err == nil {
			next()
		}
		doneB <- err
	}()
	nextA := <-gotA // after its countdown
	if el := time.Since(start); el < 4*time.Second {
		t.Fatal("a stopped before its countdown ended:", el)
	}
	time.Sleep(2 * time.Second) // b: its countdown over too, waiting for a's copy
	select {
	case <-doneB:
		t.Fatal("b went before a gave the turn")
	default:
	}
	nextA()
	if err := <-doneB; err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(said["a"]) == 0 || !strings.HasPrefix(said["a"][0], "Server updates in") || said["a"][len(said["a"])-1] != sayNow {
		t.Fatalf("a: %q", said["a"])
	}
	queued := 0
	for _, m := range said["b"] {
		if m == sayQueued {
			queued++
		}
	}
	if !strings.HasPrefix(said["b"][0], "Server updates in") || queued < 1 || said["b"][len(said["b"])-1] != sayNow {
		t.Fatalf("b: %q", said["b"])
	}
}
