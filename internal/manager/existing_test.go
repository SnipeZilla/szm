package manager

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/SnipeZilla/szm/internal/config"
	"github.com/SnipeZilla/szm/internal/games"
)

// fakeServer makes a folder that looks like an installed server of a game: its program and its game folder.
func fakeServer(t *testing.T, dir string, g *games.Game, cfg string) {
	t.Helper()
	exe := filepath.Join(dir, g.ExeCandidates()[len(g.ExeCandidates())-1])
	os.MkdirAll(filepath.Dir(exe), 0o755)
	os.WriteFile(exe, []byte("x"), 0o755)
	os.MkdirAll(filepath.Join(dir, filepath.FromSlash(g.GameDir)), 0o755)
	if cfg != "" {
		p := g.Path(dir, g.Config)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(cfg), 0o644)
	}
}

func TestInspectFolder(t *testing.T) {
	cat, _ := games.Load("")
	root := t.TempDir()
	css, tf := filepath.Join(root, "css"), filepath.Join(root, "tf")
	fakeServer(t, css, cat["css"], "hostname \"Old CSS\"\nrcon_password \"secret1\"\n")
	fakeServer(t, tf, cat["tf2"], "")
	other := filepath.Join(root, "other")
	os.MkdirAll(other, 0o755)
	os.WriteFile(filepath.Join(other, "readme.txt"), []byte("x"), 0o644)
	empty := filepath.Join(root, "empty")
	os.MkdirAll(empty, 0o755)

	cfg := config.Defaults()
	data := t.TempDir()
	cfg.SetPath(filepath.Join(data, "szm.json"))
	m := New(cfg, cat, data)

	f := m.InspectFolder(css, "css")
	if f.Exe == "" || !f.HasRcon || f.Hostname != "Old CSS" || f.Empty || f.Partial {
		t.Fatalf("css folder: %+v", f)
	}
	// The same program, another game's folder: not a CS:S server, but a TF2 one
	f = m.InspectFolder(tf, "css")
	if f.Exe != "" || len(f.Detected) != 1 || f.Detected[0].Key != "tf2" || f.Partial {
		t.Fatalf("tf folder seen as css: %+v", f)
	}
	if f := m.InspectFolder(other, "css"); f.Exe != "" || f.Empty || len(f.Detected) != 0 {
		t.Fatalf("other folder: %+v", f)
	}
	if f := m.InspectFolder(empty, "css"); !f.Empty || !f.Exists {
		t.Fatalf("empty folder: %+v", f)
	}
	if f := m.InspectFolder(filepath.Join(root, "nope"), "css"); f.Exists {
		t.Fatalf("missing folder: %+v", f)
	}

	// Added as it is: its server.cfg's RCON password is taken, nothing to install
	s := &config.Server{Name: "CS:S - Server 1", Game: "css", InstallDir: css, Port: 27015, RconPassword: "random"}
	if !m.AdoptRcon(s) || s.RconPassword != "secret1" {
		t.Fatalf("rcon %q", s.RconPassword)
	}
	if errs := m.AddServer(s, "test"); len(errs) > 0 {
		t.Fatal(errs)
	}
	if v := m.instance(s.ID); v.status == StatusNotInstalled {
		t.Fatal("an installed folder shows as not installed")
	}
	// Its folder is now the manager's
	if f := m.InspectFolder(css, "css"); f.Server == "" {
		t.Fatal("folder of a server of the manager not told")
	}
}

func TestExeSetting(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "srv")
	in := filepath.Join(dir, "game", "bin", "win64", "cs2.exe")
	if got := ExeSetting(dir, in); got != filepath.Join("game", "bin", "win64", "cs2.exe") {
		t.Fatalf("inside: %q", got)
	}
	out := filepath.Join(filepath.Dir(dir), "elsewhere", "srcds.exe")
	if got := ExeSetting(dir, out); got != out {
		t.Fatalf("outside: %q", got)
	}
	if ExeSetting(dir, "") != "" || ExeSetting(dir, "srcds.exe") != "srcds.exe" {
		t.Fatal("empty or relative")
	}
}

func TestCheckExe(t *testing.T) {
	cat, _ := games.Load("")
	dir := t.TempDir()
	m := New(config.Defaults(), cat, t.TempDir())
	s := config.Server{Game: "css", InstallDir: dir, Exe: "missing.exe"}
	if m.CheckExe(s) == "" {
		t.Fatal("a missing program is accepted")
	}
	os.WriteFile(filepath.Join(dir, "srcds_win64.exe"), []byte("x"), 0o755)
	s.Exe = "srcds_win64.exe"
	if e := m.CheckExe(s); e != "" {
		t.Fatal(e)
	}
	s.Exe = ""
	if m.CheckExe(s) != "" {
		t.Fatal("automatic")
	}
	s.InstallDir, s.Exe = filepath.Join(dir, "not-installed-yet"), "x.exe"
	if m.CheckExe(s) != "" {
		t.Fatal("a server not installed yet is checked")
	}
}

func TestPrograms(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "srcds.exe"), []byte("x"), 0o755)
	os.WriteFile(filepath.Join(dir, "srcds_run"), []byte("x"), 0o755)
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("x"), 0o644)
	got := Programs(dir)
	want := 1 // srcds.exe on Windows, srcds_run (runnable) elsewhere
	if runtime.GOOS != "windows" {
		want = 2 // srcds.exe was written runnable too
	}
	if len(got) != want {
		t.Fatalf("programs %v", got)
	}
}
