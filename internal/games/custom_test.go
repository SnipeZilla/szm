package games

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestCustomGames(t *testing.T) {
	dir := t.TempDir()
	g := &Game{Key: "fof", Name: "Fistful of Frags", Engine: Source, ServerAppID: 295230, Game: "fof", DefaultMap: "fof_fistful"}
	if err := g.Check(); err != nil {
		t.Fatal(err)
	}
	if g.Exe["windows"][0] != "srcds_win64.exe" || g.Short != "Fistful of Frags" {
		t.Fatalf("%+v", g)
	}
	if err := SaveCustom(dir, map[string]*Game{"fof": g}); err != nil {
		t.Fatal(err)
	}
	cat, err := Load(dir)
	if err != nil || cat["fof"] == nil || cat["css"] == nil {
		t.Fatalf("%v %v", err, cat["fof"])
	}
	back, _ := LoadCustom(dir)
	if back["fof"] == nil || back["fof"].ServerAppID != 295230 {
		t.Fatal(back)
	}
	mod := &Game{Key: "ts", Name: "The Specialists", Engine: GoldSrc, ServerAppID: 90, Game: "ts"}
	if err := mod.Check(); err != nil || mod.AppSetConfig != "90 mod ts" {
		t.Fatal(err, mod.AppSetConfig)
	}
	if (&Game{Key: "Bad Key", Name: "x", Engine: Source, ServerAppID: 1, Game: "x"}).Check() == nil {
		t.Fatal("bad key accepted")
	}
	if Builtin()["cs2"] == nil {
		t.Fatal("Builtin")
	}
}

// The games built into szm.exe must read: a comma missing there would stop the manager.
func TestBuiltinReads(t *testing.T) {
	cat, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	known := regexp.MustCompile(`\{(ip|port|queryport|rconport|maxplayers|map|name|password|rcon_password|gslt|logfile|dir|game)\}`)
	ph := regexp.MustCompile(`\{[a-z_]+\}`)
	for k, g := range cat {
		if g.IsOther() {
			c := *g
			if len(c.Args) == 0 && len(c.ArgsOS) == 0 {
				c.Args = []string{"-x"} // Enshrouded takes none: its config file says it all
			}
			if err := c.Check(); err != nil || g.Name == "" || len(g.Exe["windows"]) == 0 {
				t.Errorf("%s: %v", k, err)
			}
			for _, list := range append([][]string{g.Args}, g.ArgsOS["linux"], g.ArgsOS["windows"]) {
				for _, a := range list {
					if left := ph.FindAllString(known.ReplaceAllString(a, ""), -1); len(left) > 0 {
						t.Errorf("%s: unknown placeholder %v in %q", k, left, a)
					}
				}
			}
			continue
		}
		if g.Name == "" || g.Game == "" || len(g.Exe["windows"]) == 0 {
			t.Errorf("%s: name, game folder and Windows program are needed", k)
		}
	}
}

func TestErrorWhere(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(CustomPath(dir), []byte("{\n  \"games\": {\n    \"x\": { \"name\": \"X\"\n      \"engine\": \"source\" }\n  }\n}"), 0o644)
	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "line 4, column 7") {
		t.Fatal(err)
	}
	cat, err := LoadOrBuiltin(dir)
	if err == nil || cat == nil || cat["css"] == nil {
		t.Fatal("the built-in games are not given with a broken games.json")
	}
}

func TestCheckURL(t *testing.T) {
	g := &Game{Key: "fof", Name: "Fistful of Frags", Engine: Source, ServerAppID: 295230, Game: "fof"}
	for _, bad := range []string{"store.steampowered.com/app/1", "javascript:alert(1)", "https://"} {
		g.URL = bad
		if g.Check() == nil {
			t.Fatal("accepted", bad)
		}
	}
	g.URL = " https://fistful-of-frags.com/ "
	if err := g.Check(); err != nil || g.URL != "https://fistful-of-frags.com/" {
		t.Fatal(err, g.URL)
	}
}
