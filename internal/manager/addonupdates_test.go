package manager

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SnipeZilla/szm/internal/config"
	"github.com/SnipeZilla/szm/internal/crashinfo"
	"github.com/SnipeZilla/szm/internal/events"
	"github.com/SnipeZilla/szm/internal/games"
)

func TestAddonVersions(t *testing.T) {
	for n, want := range map[string]string{
		"sourcemod-1.12.0-git7255-windows.zip":              "1.12.0-git7255",
		"mmsource-2.0.0-git1483-linux.tar.gz":               "2.0.0-git1483",
		"counterstrikesharp-with-runtime-linux-1.0.378.zip": "1.0.378",
		"latest.zip": "",
	} {
		if got := versionInName(n); got != want {
			t.Errorf("%s: %q, want %q", n, got, want)
		}
	}
	for _, c := range []struct {
		a, b  string
		newer bool
	}{
		{"1.12.0-git7260", "1.12.0-git7255", true},
		{"1.12.0-git7255", "1.12.0-git7255", false},
		{"1.12.0-git7255", "1.12.0-git7260", false},
		{"1.13.0-git7400", "1.12.0-git7260", true},
		{"v1.0.378", "1.0.378", false},
		{"v1.0.379", "1.0.378", true},
		{"1.0.378", "v1.0.400", false},
		{"2026-10-24 10:00 UTC", "2026-10-20 10:00 UTC", true}, // dates: told apart, not ordered
		{"1.2", "", false},
	} {
		if got := newerVersion(c.a, c.b); got != c.newer {
			t.Errorf("newer(%s, %s) = %v", c.a, c.b, got)
		}
	}
	// Installed not known: newer than the first seen
	st := addonPkgState{Seen: "1.0.1", Latest: &Release{Version: "1.0.1"}}
	if hasNew(st) {
		t.Fatal("the first seen is no news")
	}
	st.Latest.Version = "1.0.2"
	if !hasNew(st) {
		t.Fatal("newer than the first seen")
	}
}

func TestAddonSources(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/smdrop/1.12/sourcemod-latest-"+osName(), func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("sourcemod-1.12.0-git7260-" + osName() + ".zip\n"))
	})
	mux.HandleFunc("/repos/roflmuffin/CounterStrikeSharp/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"tag_name": "v1.0.378", "html_url": "https://example/rel", "published_at": "2026-10-23T14:22:00Z",
			"assets": []map[string]string{
				{"name": "counterstrikesharp-linux-1.0.378.zip", "browser_download_url": "https://dl/a"},
				{"name": "counterstrikesharp-windows-1.0.378.zip", "browser_download_url": "https://dl/b"},
				{"name": "counterstrikesharp-with-runtime-linux-1.0.378.zip", "browser_download_url": "https://dl/c"},
				{"name": "counterstrikesharp-with-runtime-windows-1.0.378.zip", "browser_download_url": "https://dl/d"},
			}})
	})
	mux.HandleFunc("/repos/a/b/releases", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{
			{"tag_name": "v3-draft", "draft": true},
			{"tag_name": "v2.1-beta", "prerelease": true, "assets": []map[string]string{{"name": "plug.zip", "browser_download_url": "https://dl/p"}}},
		})
	})
	mux.HandleFunc("/repos/limit/x/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(http.StatusForbidden)
	})
	// A "latest" link redirecting to a version's file, a pointer file, an archive with a date only
	mux.HandleFunc("/o/p/releases/latest/download/p.zip", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/o/p/releases/download/v4.5/p.zip", http.StatusFound)
	})
	mux.HandleFunc("/o/p/releases/download/v4.5/p.zip", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("PK")) })
	mux.HandleFunc("/drop/latest-file", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("thing-2.3.4-git99.tar.gz")) })
	mux.HandleFunc("/dated.zip", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Last-Modified", "Tue, 20 Oct 2026 14:00:00 GMT")
		w.Write([]byte("PK"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	smDrop, githubAPI = srv.URL+"/smdrop", srv.URL
	defer func() { smDrop, githubAPI = "https://sm.alliedmods.net/smdrop", "https://api.github.com" }()
	ctx := context.Background()

	r, err := latestRelease(ctx, &config.AddonSource{Kind: "sourcemod", Branch: "1.12"})
	if err != nil || r.Version != "1.12.0-git7260" || r.URL != srv.URL+"/smdrop/1.12/sourcemod-1.12.0-git7260-"+osName()+".zip" {
		t.Fatalf("sourcemod: %+v %v", r, err)
	}
	r, err = latestRelease(ctx, &config.AddonSource{Kind: "counterstrikesharp"})
	if err != nil || r.Version != "v1.0.378" || r.File != "counterstrikesharp-with-runtime-"+osName()+"-1.0.378.zip" || r.Published == 0 {
		t.Fatalf("css: %+v %v", r, err)
	}
	r, err = latestRelease(ctx, &config.AddonSource{Kind: "counterstrikesharp", NoRuntime: true})
	if err != nil || r.File != "counterstrikesharp-"+osName()+"-1.0.378.zip" {
		t.Fatalf("css without runtime: %+v %v", r, err)
	}
	if _, err := latestRelease(ctx, &config.AddonSource{Kind: "github", Repo: "roflmuffin/CounterStrikeSharp", Asset: "nothing-*.zip"}); err == nil || !strings.Contains(err.Error(), "has no archive matching") {
		t.Fatal("no match:", err)
	}
	r, err = latestRelease(ctx, &config.AddonSource{Kind: "github", Repo: "a/b", Pre: true})
	if err != nil || r.Version != "v2.1-beta" || r.File != "plug.zip" {
		t.Fatalf("pre-release: %+v %v", r, err)
	}
	if _, err := latestRelease(ctx, &config.AddonSource{Kind: "github", Repo: "limit/x"}); err == nil || !strings.Contains(err.Error(), "too many checks") {
		t.Fatal("rate limit:", err)
	}
	r, err = latestRelease(ctx, &config.AddonSource{Kind: "link", URL: srv.URL + "/o/p/releases/latest/download/p.zip"})
	if err != nil || r.Version != "v4.5" || !strings.HasSuffix(r.URL, "/releases/download/v4.5/p.zip") {
		t.Fatalf("link to a GitHub latest: %+v %v", r, err)
	}
	r, err = latestRelease(ctx, &config.AddonSource{Kind: "link", URL: srv.URL + "/drop/latest-file"})
	if err != nil || r.Version != "2.3.4-git99" || r.URL != srv.URL+"/drop/thing-2.3.4-git99.tar.gz" {
		t.Fatalf("pointer: %+v %v", r, err)
	}
	r, err = latestRelease(ctx, &config.AddonSource{Kind: "link", URL: srv.URL + "/dated.zip"})
	if err != nil || r.Version != "2026-10-20 14:00 UTC" {
		t.Fatalf("dated: %+v %v", r, err)
	}
}

func zipBytes(t *testing.T, files map[string]string) []byte {
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for n, c := range files {
		w, _ := z.Create(n)
		w.Write([]byte(c))
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func testAddonManager(t *testing.T) *Manager {
	dir := t.TempDir()
	cfg := config.Defaults()
	cfg.SetPath(filepath.Join(dir, "szm.json"))
	m := &Manager{DataDir: dir, Log: events.New(filepath.Join(dir, "logs")), Cfg: cfg, inst: map[string]*Instance{},
		state: loadState(filepath.Join(dir, "state.json")), addons: loadAddonStates(filepath.Join(dir, "addon-updates.json")), Started: time.Now()}
	return m
}

// A new version: told once (notify only), or installed (auto): the package updated, its configs kept.
func TestAddonCheckAndInstall(t *testing.T) {
	ver := "1.12.0-git7255"
	mux := http.NewServeMux()
	mux.HandleFunc("/smdrop/1.12/sourcemod-latest-"+osName(), func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("sourcemod-" + ver + "-" + osName() + ".zip"))
	})
	mux.HandleFunc("/smdrop/1.12/", func(w http.ResponseWriter, r *http.Request) {
		w.Write(zipBytes(t, map[string]string{
			"addons/sourcemod/bin/sourcemod.dll":      ver,
			"addons/sourcemod/configs/core.cfg":       "default",
			"cfg/sourcemod/sourcemod.cfg":             "default",
			"addons/sourcemod/extensions/new.ext.dll": "new",
		}))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	smDrop = srv.URL + "/smdrop"
	defer func() { smDrop = "https://sm.alliedmods.net/smdrop" }()

	m := testAddonManager(t)
	pkg, _ := m.PackagePath("sourcemod")
	os.MkdirAll(filepath.Join(pkg, "addons", "sourcemod", "configs"), 0o755)
	os.WriteFile(filepath.Join(pkg, "addons", "sourcemod", "configs", "core.cfg"), []byte("mine"), 0o644)
	c := m.Log.Subscribe()
	defer m.Log.Unsubscribe(c)
	ctx := context.Background()

	// first check: the version the package has is not known: no news
	u, err := m.CheckAddon(ctx, "sourcemod", "")
	if err != nil || u.Latest == nil || u.New || u.Source.Branch != "1.12" {
		t.Fatalf("%+v %v", u, err)
	}
	for m.addons.busyOf("sourcemod") != "" { // its files compared with the version out
		time.Sleep(10 * time.Millisecond)
	}
	ver = "1.12.0-git7260"
	if u, _ = m.CheckAddon(ctx, "sourcemod", ""); !u.New {
		t.Fatalf("a newer version: %+v", u)
	}
	told := 0
	drain := func() {
		for {
			select {
			case e := <-c:
				if e.Kind == events.Update && strings.Contains(e.Text, "The Addons page installs it") {
					told++
				}
			default:
				return
			}
		}
	}
	m.CheckAddon(ctx, "sourcemod", "") // the same: not told again
	drain()
	if told != 1 {
		t.Fatal("told", told)
	}

	for m.addons.busyOf("sourcemod") != "" {
		time.Sleep(10 * time.Millisecond)
	}
	// auto: installed
	if err := m.SetAddonSource("sourcemod", &config.AddonSource{Kind: "sourcemod", Auto: true}, "t"); err != nil {
		t.Fatal(err)
	}
	ver = "1.12.0-git7270"
	m.CheckAddon(ctx, "sourcemod", "")
	deadline := time.Now().Add(5 * time.Second)
	for m.addons.busyOf("sourcemod") != "" || m.addons.get("sourcemod").Installed == "" {
		if time.Now().After(deadline) {
			t.Fatal("not installed:", m.addons.busyOf("sourcemod"), m.addons.get("sourcemod").Error)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if st := m.addons.get("sourcemod"); st.Installed != "1.12.0-git7270" || st.Rollout != nil {
		t.Fatalf("%+v", st)
	}
	read := func(rel string) string {
		b, _ := os.ReadFile(filepath.Join(pkg, filepath.FromSlash(rel)))
		return string(b)
	}
	if read("addons/sourcemod/bin/sourcemod.dll") != "1.12.0-git7270" || read("addons/sourcemod/configs/core.cfg") != "mine" ||
		read("cfg/sourcemod/sourcemod.cfg") != "default" || read("addons/sourcemod/extensions/new.ext.dll") != "new" {
		t.Fatal("package:", read("addons/sourcemod/bin/sourcemod.dll"), read("addons/sourcemod/configs/core.cfg"))
	}
	if u := m.AddonUpdate("sourcemod"); u.New || !u.Set {
		t.Fatalf("%+v", u)
	}
	// An upload by hand: its version from its name
	m.noteArchive("sourcemod", "sourcemod-1.12.0-git7280-linux.tar.gz", PkgUpdate)
	if st := m.addons.get("sourcemod"); st.Installed != "1.12.0-git7280" {
		t.Fatal(st.Installed)
	}
	// a plugin's archive put into it: the addon's version stays
	m.noteArchive("sourcemod", "rtv-extended-2.3.zip", PkgUpdate)
	if st := m.addons.get("sourcemod"); st.Installed != "1.12.0-git7280" {
		t.Fatal("a plugin's archive:", st.Installed)
	}
	// replaced by another archive: not known any more
	m.noteArchive("sourcemod", "my-sm.zip", PkgReplace)
	if st := m.addons.get("sourcemod"); st.Installed != "" {
		t.Fatal("replaced:", st.Installed)
	}
}

// A crash an addon of a package caused starts the crash watch; the package holding the file is the one named.
func TestAddonCrashWatch(t *testing.T) {
	m := testAddonManager(t)
	for _, f := range []string{"sourcemod/addons/sourcemod/bin/sourcemod.logic.dll", "dhooks/addons/sourcemod/extensions/dhooks.ext.2.csgo.dll", "plain/addons/plain/x.dll"} {
		p := filepath.Join(m.AddonsRoot(), filepath.FromSlash(f))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, nil, 0o644)
	}
	pkgs := []string{"sourcemod", "dhooks", "plain"}
	if got := m.packageHolding(pkgs, `C:\srv\css\cstrike\addons\sourcemod\extensions\dhooks.ext.2.csgo.dll`); got != "dhooks" {
		t.Fatal(got)
	}
	if got := m.packageHolding(pkgs, `/srv/css/cstrike/addons/SourceMod/bin/other.so`); got != "sourcemod" {
		t.Fatal(got)
	}
	if got := m.packageHolding(pkgs, `/srv/x/addons/none/a.so`); got != "" {
		t.Fatal(got)
	}
	r := &CrashReport{Suspect: "SourceMod extension dhooks", Dump: &crashinfo.DumpAnalysis{Code: "0xC0000005", Suspect: "SourceMod extension dhooks", Sure: "likely",
		Chain: []crashinfo.Frame{{Module: "server.dll"}, {Module: "dhooks.ext.2.csgo.dll", Label: "SourceMod extension dhooks", Path: `C:\srv\cstrike\addons\sourcemod\extensions\dhooks.ext.2.csgo.dll`}}}}
	if p := blamedPath(r); !strings.HasSuffix(p, `dhooks.ext.2.csgo.dll`) {
		t.Fatal(p)
	}
	r.Dump.Sure = "unsure"
	if p := blamedPath(r); p != "" {
		t.Fatal("unsure blames nobody:", p)
	}
	at := time.Now()
	r = &CrashReport{Time: at, InARow: 2, Changes: []Change{{Time: at.Add(-time.Hour), Kind: "event", Text: "Game updated: CS2 1.41.2.3 (2 min)"}}}
	if !gameUpdatedBefore(r, 48*time.Hour) || gameUpdatedBefore(r, 30*time.Minute) {
		t.Fatal("game updated before")
	}
}

func TestUnpatchGameinfo(t *testing.T) {
	p := filepath.Join(t.TempDir(), "gameinfo.gi")
	in := "\t\tSearchPaths\r\n\t\t{\r\n\t\t\tGame_LowViolence\tcsgo_lv\r\n\t\t\tGame\tcsgo/addons/metamod\r\n\t\t\tGame\tcsgo\r\n"
	os.WriteFile(p, []byte(in), 0o644)
	if ch, err := UnpatchGameinfo(p, "csgo/addons/metamod"); !ch || err != nil {
		t.Fatal(ch, err)
	}
	b, _ := os.ReadFile(p)
	if strings.Contains(string(b), "metamod") || !strings.Contains(string(b), "\t\t\tGame\tcsgo\r\n") {
		t.Fatalf("%q", b)
	}
	if ch, _ := UnpatchGameinfo(p, "csgo/addons/metamod"); ch {
		t.Fatal("twice")
	}
	if ch, _ := PatchGameinfo(p, "csgo/addons/metamod"); !ch {
		t.Fatal("back")
	}
}

// Without its addons while it waits for a fix: Metamod set aside (a Source game: metamod*.vdf; CS2: gameinfo.gi),
// then back.
func TestVanilla(t *testing.T) {
	m := testAddonManager(t)
	dir := t.TempDir()
	add := filepath.Join(dir, "cstrike", "addons")
	os.MkdirAll(add, 0o755)
	os.WriteFile(filepath.Join(add, "metamod.vdf"), []byte("a"), 0o644)
	os.WriteFile(filepath.Join(add, "metamod_x64.vdf"), []byte("b"), 0o644)
	s := config.Server{ID: "css", InstallDir: dir, Vanilla: true}
	g := &games.Game{Engine: games.Source, Game: "cstrike", GameDir: "cstrike"}
	exists := func(n string) bool { _, err := os.Stat(filepath.Join(add, n)); return err == nil }

	m.applyVanilla(s, g) // not waiting: nothing changes
	if !exists("metamod.vdf") || exists("metamod.vdf"+vanillaOff) {
		t.Fatal("changed while not waiting")
	}
	m.state.update("css", func(v *savedServer) { v.Vanilla = "sourcemod" })
	m.applyVanilla(s, g)
	if exists("metamod.vdf") || exists("metamod_x64.vdf") || !exists("metamod.vdf"+vanillaOff) || !exists("metamod_x64.vdf"+vanillaOff) {
		t.Fatal("not set aside")
	}
	if m.vanillaFor(s) != "sourcemod" {
		t.Fatal("vanillaFor")
	}
	// a push brought one back meanwhile, then the fix: back on, the set-aside copy dropped
	os.WriteFile(filepath.Join(add, "metamod.vdf"), []byte("new"), 0o644)
	m.state.update("css", func(v *savedServer) { v.Vanilla = "" })
	m.applyVanilla(s, g)
	if b, _ := os.ReadFile(filepath.Join(add, "metamod.vdf")); string(b) != "new" || !exists("metamod_x64.vdf") || exists("metamod.vdf"+vanillaOff) || exists("metamod_x64.vdf"+vanillaOff) {
		t.Fatal("not back")
	}
	// The setting turned off while waiting: back on at the next start
	m.state.update("css", func(v *savedServer) { v.Vanilla = "sourcemod" })
	s.Vanilla = false
	m.applyVanilla(s, g)
	if !exists("metamod.vdf") || m.state.get("css").Vanilla != "" {
		t.Fatal("setting off")
	}

	// CS2: the gameinfo.gi line out, and patchMetamod does not put it back meanwhile
	cs := filepath.Join(dir, "cs2")
	gi := filepath.Join(cs, "game", "csgo", "gameinfo.gi")
	os.MkdirAll(filepath.Join(cs, "game", "csgo", "addons", "metamod"), 0o755)
	os.WriteFile(gi, []byte("\t\t\tGame_LowViolence\tcsgo_lv\n\t\t\tGame\tcsgo/addons/metamod\n\t\t\tGame\tcsgo\n"), 0o644)
	s2 := config.Server{ID: "cs2", InstallDir: cs, Vanilla: true}
	g2 := &games.Game{Engine: games.Source2, Game: "csgo", GameDir: "game/csgo", Metamod: true}
	m.state.update("cs2", func(v *savedServer) { v.Vanilla = "metamod2" })
	m.applyVanilla(s2, g2)
	if ch, _ := m.patchMetamod(s2, g2); ch {
		t.Fatal("patched while waiting")
	}
	if b, _ := os.ReadFile(gi); strings.Contains(string(b), "metamod") {
		t.Fatalf("%q", b)
	}
	m.state.update("cs2", func(v *savedServer) { v.Vanilla = "" })
	m.applyVanilla(s2, g2)
	if ch, _ := m.patchMetamod(s2, g2); !ch {
		t.Fatal("not back")
	}
}

// A package whose version is not known (filled from a server): its program files compared with the version out.
func TestAddonIdentify(t *testing.T) {
	arch := zipBytes(t, map[string]string{
		"mm-2.0/addons/metamod/bin/win64/metamod.2.cs2.dll": "MM1483",
		"mm-2.0/addons/metamod/metaplugins.ini":             "default",
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/latest") {
			w.Write([]byte("mmsource-2.0.0-git1483-windows.zip"))
			return
		}
		w.Write(arch)
	}))
	defer srv.Close()
	m := testAddonManager(t)
	m.Cfg.AddonUpdates = map[string]*config.AddonSource{"metamod2": {Kind: "link", URL: srv.URL + "/latest"}}
	pkg, _ := m.PackagePath("metamod2")
	dll := filepath.Join(pkg, "addons", "metamod", "bin", "win64", "metamod.2.cs2.dll")
	os.MkdirAll(filepath.Dir(dll), 0o755)
	os.WriteFile(dll, []byte("MM1470"), 0o644) // another build
	os.WriteFile(filepath.Join(pkg, "addons", "metamod", "metaplugins.ini"), []byte("mine"), 0o644)
	wait := func() {
		deadline := time.Now().Add(5 * time.Second)
		for m.addons.busyOf("metamod2") != "" {
			if time.Now().After(deadline) {
				t.Fatal("still busy")
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	m.CheckAddon(context.Background(), "metamod2", "")
	wait()
	if st := m.addons.get("metamod2"); st.Installed != "" || st.Compared != "2.0.0-git1483" {
		t.Fatalf("another build: %+v", st)
	}
	// the same build (its config edited: not a program): it has that version
	os.WriteFile(dll, []byte("MM1483"), 0o644)
	m.addons.update("metamod2", func(v *addonPkgState) { v.Compared = "" })
	m.CheckAddon(context.Background(), "metamod2", "")
	wait()
	if st := m.addons.get("metamod2"); st.Installed != "2.0.0-git1483" {
		t.Fatalf("same build: %+v", st)
	}
	if u := m.AddonUpdate("metamod2"); u.New {
		t.Fatal("up to date")
	}
}

// The source a package's name gives, only for the addon itself and for the games it is for.
func TestDefaultAddonSource(t *testing.T) {
	for n, want := range map[string]string{"sourcemod": "sourcemod", "SourceMod1.12": "sourcemod", "metamod2": "metamod", "metamod-2.0": "metamod",
		"metamod1": "metamod", "counterstrikesharp": "counterstrikesharp", "amxmodx": "amxmodx", "sourcemod-plugins": "", "metamod-r": "", "reapi": "", "css": ""} {
		got := ""
		if src := config.DefaultAddonSource(n); src != nil {
			got = src.Kind
		}
		if got != want {
			t.Errorf("%s: %q, want %q", n, got, want)
		}
	}
	m := testAddonManager(t)
	m.SetCatalog(games.Catalog{"cs16": {Engine: games.GoldSrc}, "css": {Engine: games.Source}})
	m.Cfg.Servers = []*config.Server{{ID: "a", Game: "cs16", Packages: []string{"metamod", "amxmodx"}}, {ID: "b", Game: "css", Packages: []string{"metamod1"}}}
	if src, _ := m.AddonSourceOf("metamod"); src != nil {
		t.Fatal("GoldSrc's metamod is not Metamod:Source")
	}
	if src, _ := m.AddonSourceOf("amxmodx"); src == nil || src.Branch != "1.10" {
		t.Fatal("amxmodx", src)
	}
	if src, _ := m.AddonSourceOf("metamod1"); src == nil || src.Branch != "1.12" {
		t.Fatal("metamod1", src)
	}
	// the page's default: the same as the manager's (a "metamod" of a TF2 or CS:S server is 1.12)
	m.Cfg.Servers = append(m.Cfg.Servers, &config.Server{ID: "c", Game: "css", Packages: []string{"mm"}})
	if d := m.AddonUpdate("mm").DefaultBranch["metamod"]; d != "1.12" {
		t.Fatal("default branch", d)
	}
}

// A branch typed as Metamod 1 or 2 is the newest of that version.
func TestAddonBranch(t *testing.T) {
	for in, want := range map[string]string{"1": "1.12", "2": "2.0", "1.0": "1.12", "1.11": "1.11", "": ""} {
		src := &config.AddonSource{Kind: "metamod", Branch: in}
		if err := checkSource(src); err != nil || src.Branch != want {
			t.Errorf("%q: %q %v", in, src.Branch, err)
		}
	}
	if err := checkSource(&config.AddonSource{Kind: "sourcemod", Branch: "abc"}); err == nil {
		t.Fatal("abc")
	}
}

// An older version put in by hand, Install it set, a newer one out: installed at the next minute's check.
func TestAddonOlderUploaded(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/roflmuffin/CounterStrikeSharp/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"tag_name": "v1.0.378", "assets": []map[string]string{
			{"name": "counterstrikesharp-" + osName() + "-1.0.378.zip", "browser_download_url": "http://" + r.Host + "/dl/css.zip"}}})
	})
	mux.HandleFunc("/dl/css.zip", func(w http.ResponseWriter, r *http.Request) {
		w.Write(zipBytes(t, map[string]string{"addons/counterstrikesharp/bin/x.dll": "378"}))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	githubAPI = srv.URL
	defer func() { githubAPI = "https://api.github.com" }()
	m := testAddonManager(t)
	m.Cfg.AddonUpdates = map[string]*config.AddonSource{"counterstrikesharp": {Kind: "counterstrikesharp", NoRuntime: true, Auto: true}}
	pkg, _ := m.PackagePath("counterstrikesharp")
	os.MkdirAll(pkg, 0o755)
	m.addons.update("counterstrikesharp", func(v *addonPkgState) {
		v.Installed, v.Checked = "v1.0.378", time.Now().Unix()
		v.Latest = &Release{Version: "v1.0.378", File: "counterstrikesharp-" + osName() + "-1.0.378.zip"}
	})
	m.noteArchive("counterstrikesharp", "counterstrikesharp-windows-1.0.377.zip", PkgReplace)
	if st := m.addons.get("counterstrikesharp"); st.Installed != "1.0.377" || st.Checked != 0 {
		t.Fatalf("%+v", st)
	}
	m.addonTick(context.Background())
	deadline := time.Now().Add(5 * time.Second)
	for m.addons.busyOf("counterstrikesharp") != "" || m.addons.get("counterstrikesharp").Installed != "v1.0.378" {
		if time.Now().After(deadline) {
			t.Fatalf("not installed: %+v %s", m.addons.get("counterstrikesharp"), m.addons.busyOf("counterstrikesharp"))
		}
		time.Sleep(10 * time.Millisecond)
	}
}
