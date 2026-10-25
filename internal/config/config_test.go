package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/SnipeZilla/szm/internal/games"
)

func cat(t *testing.T) games.Catalog {
	c, err := games.Load("")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestBuildArgs(t *testing.T) {
	c := cat(t)
	s := &Server{Game: "css", IP: "1.2.3.4", Port: 27016, Map: "de_dust2", MaxPlayers: 18,
		Args: "-tickrate 100 +maxplayers 20", GSLT: "ABCDEF0123456789ABCDEF0123456789", ConsoleLog: true}
	got := strings.Join(s.BuildArgs(c["css"]), " ")
	want := "-console -game cstrike -usercon -ip 1.2.3.4 -port 27016 -tickrate 100 +maxplayers 20 +sv_setsteamaccount ABCDEF0123456789ABCDEF0123456789 -condebug +map de_dust2"
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	// GoldSrc: no token, no -ip when listening on all
	s = &Server{Game: "cs16", Port: 27015, Map: "de_dust2", MaxPlayers: 32, GSLT: "ABCDEF0123456789ABCDEF0123456789"}
	got = strings.Join(s.BuildArgs(c["cs16"]), " ")
	if got != "-console -game cstrike -port 27015 +maxplayers 32 +map de_dust2" {
		t.Fatalf("goldsrc: %s", got)
	}
}

func TestRedact(t *testing.T) {
	in := `cs2 +sv_setsteamaccount ABCDEF0123456789ABCDEF0123456789 +map x`
	if out := Redact(in); strings.Contains(out, "ABCDEF0123") || !strings.Contains(out, "+map x") {
		t.Fatal(out)
	}
}

func TestValidate(t *testing.T) {
	c := Defaults()
	c.Servers = []*Server{{ID: "a", Name: "A", Game: "css", InstallDir: "/srv/a", Port: 27015, IP: "1.1.1.1"}}
	ok := &Server{ID: "b", Name: "B", Game: "css", InstallDir: "/srv/b", Port: 27015, IP: "2.2.2.2"}
	if errs := c.Validate(ok, cat(t)); len(errs) != 0 {
		t.Fatal(errs)
	}
	bad := &Server{ID: "b", Name: "B", Game: "css", InstallDir: "/srv/a", Port: 27015}
	if errs := c.Validate(bad, cat(t)); len(errs) != 2 {
		t.Fatal(errs)
	}
	// every address, while servers use the port on addresses of their own: one message naming them
	c.Servers = append(c.Servers, &Server{ID: "c", Name: "C", Game: "css", InstallDir: "/srv/c", Port: 27015, IP: "3.3.3.3"})
	wild := &Server{ID: "d", Name: "D", Game: "cs2", InstallDir: "/srv/d", Port: 27015, IP: "0.0.0.0"}
	if errs := c.Validate(wild, cat(t)); len(errs) != 1 || !strings.Contains(errs[0], "A (1.1.1.1), C (3.3.3.3) use it") {
		t.Fatal(errs)
	}
	wild.Port = 27016 // a port nobody uses: fine
	if errs := c.Validate(wild, cat(t)); len(errs) != 0 {
		t.Fatal(errs)
	}
}

func TestUpgradeOldDefault(t *testing.T) {
	dir := t.TempDir()
	p := dir + "/szm.json"
	os.WriteFile(p, []byte(`{"copy_exclude":["server.cfg","autoexec.cfg","motd.txt","mapcycle.txt","gamemodes_server.txt","banned_user.cfg","banned_ip.cfg"]}`), 0o644)
	c, err := Load(p)
	if err != nil || len(c.Upgraded) != 1 || !strings.Contains(strings.Join(c.CopyExclude, " "), "gamemode_*.cfg") {
		t.Fatal(err, c.Upgraded, c.CopyExclude)
	}
	// A list changed by hand stays as it is
	os.WriteFile(p, []byte(`{"copy_exclude":["server.cfg","my.cfg"]}`), 0o644)
	c, _ = Load(p)
	if len(c.Upgraded) != 0 || strings.Join(c.CopyExclude, ",") != "server.cfg,my.cfg" {
		t.Fatal(c.Upgraded, c.CopyExclude)
	}
}

func TestWorkshopArgs(t *testing.T) {
	cat, _ := games.Load("")
	s := &Server{Port: 27015, Map: "de_dust2", WorkshopColl: "999999", WorkshopStart: "111111"}
	got := strings.Join(s.BuildArgs(cat["cs2"]), " ")
	if !strings.Contains(got, "+host_workshop_collection 999999") || !strings.HasSuffix(got, "+host_workshop_map 111111") || strings.Contains(got, "+map de_dust2") {
		t.Fatal(got)
	}
	s.WorkshopStart = ""
	got = strings.Join(s.BuildArgs(cat["cs2"]), " ")
	if !strings.HasSuffix(got, "+map de_dust2") {
		t.Fatal(got)
	}
	// not for other games
	got = strings.Join((&Server{Port: 27015, Map: "de_dust2", WorkshopColl: "1", WorkshopStart: "2"}).BuildArgs(cat["css"]), " ")
	if strings.Contains(got, "workshop") {
		t.Fatal(got)
	}
	// typed in its options: theirs wins
	s.Args = "+host_workshop_map 222222"
	s.WorkshopStart = "111111"
	got = strings.Join(s.BuildArgs(cat["cs2"]), " ")
	if strings.Contains(got, "111111") || !strings.Contains(got, "+host_workshop_map 222222") || strings.Contains(got, "+map de_dust2") {
		t.Fatal(got)
	}
}

func TestUpgradeLists(t *testing.T) {
	p := t.TempDir() + "/szm.json"
	os.WriteFile(p, []byte(`{"copy_exclude":["server.cfg","server_*.cfg","autoexec.cfg","gamemode_*.cfg","gamemodes_server.txt","motd.txt","mapcycle.txt","banned_user.cfg","banned_ip.cfg"],
	"addons_keep":["addons/counterstrikesharp/configs","addons/sourcemod/configs","addons/sourcemod/data","cfg/sourcemod","addons/amxmodx/configs","addons/amxmodx/data","addons/metamod/metaplugins.ini","*.db","*.sq3"]}`), 0o644)
	c, err := Load(p)
	if err != nil || len(c.Upgraded) != 2 {
		t.Fatal(err, c.Upgraded)
	}
	ce, ak := strings.Join(c.CopyExclude, " "), strings.Join(c.AddonsKeep, " ")
	if !strings.Contains(ce, "gamemodes_server.txt") || !strings.Contains(ce, "mapcycle_*") {
		t.Fatal(ce)
	}
	if !strings.Contains(ak, "addons/sourcemod/plugins") || !strings.Contains(ak, "addons/amxmodx/plugins") {
		t.Fatal(ak)
	}
}

func TestBackupSchedule(t *testing.T) {
	p := t.TempDir() + "/szm.json"
	for _, tc := range []struct {
		file, schedule, time string
		upgraded             bool
	}{
		{`{}`, "manual", "05:30", false},
		{`{"backups":{"time":"05:30","keep":14}}`, "manual", "05:30", true}, // the old default, never chosen
		{`{"backups":{"time":"off","keep":14}}`, "manual", "05:30", false},
		{`{"backups":{"time":"03:15","keep":14}}`, "daily", "03:15", false}, // chosen: kept
		{`{"backups":{"schedule":"weekly","time":"04:00","weekday":3,"keep":14}}`, "weekly", "04:00", false},
		{`{"backups":{"schedule":"bogus","time":"04:00","keep":14}}`, "manual", "04:00", false},
	} {
		os.WriteFile(p, []byte(tc.file), 0o644)
		c, err := Load(p)
		if err != nil || c.Backups.Schedule != tc.schedule || c.Backups.Time != tc.time || (len(c.Upgraded) > 0) != tc.upgraded {
			t.Fatal(tc.file, err, c.Backups.Schedule, c.Backups.Time, c.Upgraded)
		}
	}
	if d := Defaults(); d.Backups.Schedule != "manual" || !d.Backups.Before {
		t.Fatal(d.Backups)
	}
}

func TestSetArg(t *testing.T) {
	for _, c := range [][4]string{
		{"+game_mode 1 +game_type 0 +mapgroup mg_bomb -condebug", "+mapgroup", "mg_x", "+game_mode 1 +game_type 0 +mapgroup mg_x -condebug"},
		{"+game_mode 1", "+mapgroup", "mg_x", "+game_mode 1 +mapgroup mg_x"},
		{"", "+mapgroup", "mg_x", "+mapgroup mg_x"},
		{"+MapGroup  \"mg_a\" -x", "+mapgroup", "mg_b", "+MapGroup  mg_b -x"},
		{"+mapgroup -condebug", "+mapgroup", "mg_b", "+mapgroup mg_b -condebug"},
	} {
		if got := SetArg(c[0], c[1], c[2]); got != c[3] {
			t.Errorf("%q: %q", c[0], got)
		}
	}
	if v := ArgValue("+game_mode 1 +mapgroup \"mg_bomb\"", "+MAPGROUP"); v != "mg_bomb" {
		t.Fatal(v)
	}
}

func TestUpgradeExcludeWithGameModes(t *testing.T) {
	p := t.TempDir() + "/szm.json"
	os.WriteFile(p, []byte(`{"copy_exclude":["server.cfg","server_*.cfg","autoexec.cfg","gamemode_*.cfg","motd.txt","mapcycle.txt","mapcycle_*","banned_user.cfg","banned_ip.cfg"]}`), 0o644)
	c, err := Load(p)
	if err != nil || !strings.Contains(strings.Join(c.CopyExclude, " "), "gamemodes_server.txt") || len(c.Upgraded) != 1 {
		t.Fatal(err, c.CopyExclude, c.Upgraded)
	}
	// changed by hand: left alone
	os.WriteFile(p, []byte(`{"copy_exclude":["server.cfg","mine.cfg"]}`), 0o644)
	if c, _ := Load(p); strings.Join(c.CopyExclude, " ") != "server.cfg mine.cfg" {
		t.Fatal(c.CopyExclude)
	}
}

func TestManagerHidden(t *testing.T) {
	for in, want := range map[string]bool{`{}`: true, `{"show_manager": false}`: true, `{"show_manager": true}`: false} {
		var c Config
		if err := json.Unmarshal([]byte(in), &c); err != nil || c.ManagerHidden() != want {
			t.Error(in, c.ManagerHidden(), err)
		}
	}
}

// The games of other engines: their command line filled in, an option left empty left out, their secrets hidden.
func TestOtherArgs(t *testing.T) {
	c := cat(t)
	dir := t.TempDir()
	rust := Server{Name: "My Rust", IP: "1.2.3.4", Port: 28015, MaxPlayers: 50, Map: "Procedural Map", RconPassword: "rc0npw", InstallDir: dir, ConsoleLog: true, Args: "+server.seed 42"}
	got := strings.Join(rust.BuildArgs(c["rust"]), "|")
	for _, want := range []string{"+server.ip|1.2.3.4", "+server.port|28015", "+server.queryport|28017", "+rcon.port|28016", "+rcon.password|rc0npw",
		"+server.hostname|My Rust", "+server.level|Procedural Map", "+server.maxplayers|50", "-logfile|" + dir, "+server.seed|42"} {
		if !strings.Contains(got, want) {
			t.Errorf("rust: no %q in %s", want, got)
		}
	}
	if strings.Contains(got, "-condebug") || strings.Contains(got, "sv_setsteamaccount") {
		t.Fatal("Source options:", got)
	}
	if h := rust.HideSecrets(got); strings.Contains(h, "rc0npw") {
		t.Fatal(h)
	}
	// every address, no password, no console: left out
	rust.IP, rust.RconPassword, rust.ConsoleLog = "0.0.0.0", "", false
	got = strings.Join(rust.BuildArgs(c["rust"]), "|")
	if strings.Contains(got, "+server.ip") || strings.Contains(got, "+rcon.password") || strings.Contains(got, "-logfile") {
		t.Fatal("empty options kept:", got)
	}
	// ARK: its options in one URL, an empty one dropped from it
	ark := Server{Name: "Isle", Port: 7779, MaxPlayers: 20, Map: "TheIsland", RconPassword: "adm1n", InstallDir: dir}
	args := ark.BuildArgs(c["ark"])
	if args[0] != "TheIsland?listen?SessionName=Isle?ServerAdminPassword=adm1n?Port=7779?QueryPort=27017?RCONEnabled=True?RCONPort=27022?MaxPlayers=20" {
		t.Fatal("ark:", args[0])
	}
	// 7 Days to Die: -Option=value, left out when empty
	sd := Server{Name: "Horde", Port: 26900, MaxPlayers: 8, InstallDir: dir}
	got = strings.Join(sd.BuildArgs(c["sdtd"]), "|")
	if !strings.Contains(got, "-ServerPort=26900") || !strings.Contains(got, "-TelnetPort=8081") || strings.Contains(got, "-ServerPassword") || strings.Contains(got, "-TelnetPassword") {
		t.Fatal("sdtd:", got)
	}
}

// The ports a server of another engine takes: a second one must not share any.
func TestOtherPorts(t *testing.T) {
	c := cat(t)
	cfg := Defaults()
	a := &Server{ID: "v1", Name: "V1", Game: "valheim", Port: 2456, InstallDir: "/srv/v1"}
	cfg.Servers = []*Server{a}
	if got := UsedPorts(a, c["valheim"]); len(got) != 2 || got[0] != 2456 || got[1] != 2457 {
		t.Fatal(got)
	}
	b := &Server{ID: "v2", Name: "V2", Game: "valheim", Port: 2457, InstallDir: "/srv/v2"}
	if errs := cfg.Validate(b, c); len(errs) == 0 || !strings.Contains(strings.Join(errs, " "), "2457") {
		t.Fatal("overlap:", errs)
	}
	b.Port = 2458
	if errs := cfg.Validate(b, c); len(errs) != 0 {
		t.Fatal(errs)
	}
	// Rust: its RCON (+1) and query (+2) ports
	r := &Server{ID: "r", Name: "R", Game: "rust", Port: 28015, InstallDir: "/srv/r"}
	if got := UsedPorts(r, c["rust"]); strings.Trim(strings.Join(strings.Fields(fmt.Sprint(got)), ","), "[]") != "28015,28016,28017" {
		t.Fatal(got)
	}
}
