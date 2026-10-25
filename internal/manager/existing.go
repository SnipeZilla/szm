package manager

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/SnipeZilla/szm/internal/config"
	"github.com/SnipeZilla/szm/internal/games"
)

// Folder is what a folder holds, for a new server of a game: nothing (it is installed there), the game's server
// already (it is added as it is), another game's, or something else.
type Folder struct {
	Path      string      `json:"path"`
	Exists    bool        `json:"exists"`
	Empty     bool        `json:"empty"`
	Server    string      `json:"server,omitempty"`  // the server of the manager that has this folder, or one around it
	Exe       string      `json:"exe,omitempty"`     // the game's program, found in it
	Version   string      `json:"version,omitempty"` // steam.inf
	Partial   bool        `json:"partial"`           // steamcmd's files without a program: a download that did not finish
	Detected  []GameBrief `json:"detected"`          // the games whose server is in it
	Programs  []string    `json:"programs"`          // the programs at its top (and the usual places), to pick one
	Hostname  string      `json:"hostname,omitempty"`
	HasRcon   bool        `json:"has_rcon"` // its server.cfg has an RCON password: the manager uses that one
	Installed bool        `json:"installed"`
	Expected  []string    `json:"expected"` // the programs the game's server starts with
}

// GameBrief names a game.
type GameBrief struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

var hostnameRe = regexp.MustCompile(`(?im)^\s*hostname\s+"?([^"\r\n]*)"?`)

// InspectFolder tells what a folder holds for a new server of a game (game may be "").
func (m *Manager) InspectFolder(dir, game string) Folder {
	f := Folder{Path: dir, Detected: []GameBrief{}, Programs: []string{}, Expected: []string{}}
	if o, ok := m.Cfg.ServerAt(dir); ok {
		f.Server = o.Name
	} else {
		m.Cfg.RLock()
		for _, o := range m.Cfg.Servers {
			if config.Within(o.InstallDir, dir) {
				f.Server = o.Name
				break
			}
		}
		m.Cfg.RUnlock()
	}
	list, err := os.ReadDir(dir)
	if err != nil {
		return f
	}
	f.Exists = true
	f.Empty = len(list) == 0
	f.Installed = LooksInstalled(dir)
	cat := m.Catalog()
	for _, g := range cat.Sorted() {
		if exe, _ := g.FindExe(dir); exe != "" && isDir(filepath.Join(dir, filepath.FromSlash(g.GameDir))) {
			f.Detected = append(f.Detected, GameBrief{g.Key, g.Name})
		}
	}
	// Every GoldSrc server has the valve folder (Half-Life's): HLDM only when no other GoldSrc game is there
	if len(f.Detected) > 1 {
		out := f.Detected[:0]
		for _, d := range f.Detected {
			if g := cat[d.Key]; g.Engine == games.GoldSrc && strings.EqualFold(g.GameDir, "valve") {
				continue
			}
			out = append(out, d)
		}
		if len(out) > 0 {
			f.Detected = out
		}
	}
	if g := cat[game]; g != nil {
		for _, c := range g.ExeCandidates() {
			f.Expected = append(f.Expected, filepath.ToSlash(c))
		}
		// Its program and its game folder: srcds.exe alone could be any Source game's
		if exe, _ := g.FindExe(dir); exe != "" && isDir(filepath.Join(dir, filepath.FromSlash(g.GameDir))) {
			f.Exe = exe
			f.Version, _ = installedVersion(g, dir)
			if b, err := os.ReadFile(g.Path(dir, g.Config)); err == nil {
				if mm := hostnameRe.FindSubmatch(b); mm != nil {
					f.Hostname = strings.TrimSpace(string(mm[1]))
				}
				if mm := rconCfgRe.FindSubmatch(b); mm != nil && strings.TrimSpace(string(mm[1])) != "" {
					f.HasRcon = true
				}
			}
		} else if f.Installed && len(f.Detected) == 0 {
			f.Partial = true
		}
	}
	f.Programs = Programs(dir)
	return f
}

// Programs lists the programs a server folder can start with: the ones at its top, and in game/bin/win64 (CS2).
func Programs(dir string) []string {
	out := []string{}
	seen := map[string]bool{}
	add := func(sub string) {
		list, err := os.ReadDir(filepath.Join(dir, filepath.FromSlash(sub)))
		if err != nil {
			return
		}
		for _, e := range list {
			if e.IsDir() {
				continue
			}
			full := filepath.Join(dir, filepath.FromSlash(sub), e.Name())
			if !IsProgram(full) || seen[strings.ToLower(full)] {
				continue
			}
			seen[strings.ToLower(full)] = true
			out = append(out, full)
		}
	}
	add("")
	add("game/bin/win64")
	add("game/bin/linuxsteamrt64")
	add("game")
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i]) < strings.ToLower(out[j]) })
	return out
}

// IsProgram tells whether a file can start a server: an .exe on Windows; on Linux a file that may be run.
func IsProgram(path string) bool {
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return false
	}
	low := strings.ToLower(filepath.Base(path))
	if runtime.GOOS == "windows" {
		return strings.HasSuffix(low, ".exe")
	}
	if strings.HasSuffix(low, ".so") || strings.HasSuffix(low, ".txt") || strings.HasSuffix(low, ".dll") {
		return false
	}
	return st.Mode()&0o111 != 0 || strings.HasSuffix(low, ".sh")
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// ExeSetting is how a server keeps the program picked for it: relative to its folder when it is inside, so the
// setting still holds when the folder moves.
func ExeSetting(installDir, exe string) string {
	exe = strings.TrimSpace(exe)
	if exe == "" || !filepath.IsAbs(exe) {
		return exe
	}
	if config.Within(exe, installDir) {
		if rel, err := filepath.Rel(installDir, exe); err == nil && !strings.HasPrefix(rel, "..") {
			return rel
		}
	}
	return filepath.Clean(exe)
}

// ExeOf is the program a server starts with: found, or the one expected when it is not there.
func (m *Manager) ExeOf(s config.Server) (found, expected string) {
	g := m.Catalog()[s.Game]
	if g == nil {
		return "", ""
	}
	return m.exePath(s, g)
}

// existingRcon is the RCON password of the server.cfg of a server already installed, "" when none.
func existingRcon(s config.Server, g *games.Game) string {
	b, err := os.ReadFile(g.Path(s.InstallDir, g.Config))
	if err != nil {
		return ""
	}
	if mm := rconCfgRe.FindSubmatch(b); mm != nil {
		return strings.TrimSpace(string(mm[1]))
	}
	return ""
}
