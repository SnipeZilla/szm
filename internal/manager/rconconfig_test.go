package manager

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SnipeZilla/szm/internal/games"
)

// BattlEye's BEServer_x64.cfg: its RCON password and port set from the server's settings, its other lines kept,
// made again from its "_active_" copy (BattlEye renames it as it starts), not written again when nothing changed.
func TestSetLines(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "battleye", "BEServer_x64.cfg")
	set := []string{"RConPassword abc123", "RConPort 2305", "RestrictRCon 0"}

	// none yet: made
	if changed, err := setLines(cfg, set); err != nil || !changed {
		t.Fatal(changed, err)
	}
	b, _ := os.ReadFile(cfg)
	if got := strings.ReplaceAll(string(b), "\r\n", "\n"); got != "RConPassword abc123\nRConPort 2305\nRestrictRCon 0\n" {
		t.Fatalf("%q", got)
	}
	// the same: not written
	if changed, _ := setLines(cfg, set); changed {
		t.Fatal("written again")
	}
	// BattlEye renamed it, the admin added lines to it: kept, ours set
	os.Remove(cfg)
	old := filepath.Join(dir, "battleye", "BEServer_x64_active_1a2b.cfg")
	os.WriteFile(old, []byte("RConPassword old\r\nMaxPing 300\r\n"), 0o644)
	newer := filepath.Join(dir, "battleye", "BEServer_x64_active_9f9f.cfg")
	os.WriteFile(newer, []byte("rconpassword old2\r\nRConPort 2305\r\nMaxPing 250\r\nrconpassword twice\r\n"), 0o644)
	os.Chtimes(old, time.Now().Add(-time.Hour), time.Now().Add(-time.Hour))
	if changed, err := setLines(cfg, set); err != nil || !changed {
		t.Fatal(changed, err)
	}
	b, _ = os.ReadFile(cfg)
	if string(b) != "RConPassword abc123\r\nRConPort 2305\r\nMaxPing 250\r\nRestrictRCon 0\r\n" {
		t.Fatalf("%q", b)
	}
}

// The command typed in a console that ends a server: the last of its game's stop commands.
func TestStopsIt(t *testing.T) {
	dayz := &games.Game{Stop: []string{"#shutdown"}}
	ark := &games.Game{Stop: []string{"saveworld", "DoExit"}}
	if !stopsIt(dayz, " #Shutdown ") || stopsIt(dayz, "players") || !stopsIt(ark, "doexit") || stopsIt(ark, "saveworld") || stopsIt(&games.Game{}, "quit") {
		t.Fatal("stopsIt")
	}
}
