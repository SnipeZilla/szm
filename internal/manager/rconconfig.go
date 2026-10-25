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

// setRconConfig sets, before a server starts, the lines of the files its RCON is set in (BattlEye's
// BEServer_x64.cfg: its password and port) from its settings; their other lines are kept. Nothing without an RCON
// password: its RCON stays as it is.
func (m *Manager) setRconConfig(i *Instance, s config.Server, g *games.Game) {
	if len(g.RconConfig) == 0 || s.RconPassword == "" {
		return
	}
	if g.RconMode() == games.RconBattlEye && !alnumRe.MatchString(s.RconPassword) {
		i.say("[rcon] BattlEye takes an RCON password of letters and numbers only (24 at most): change it in Settings")
	}
	files := make([]string, 0, len(g.RconConfig))
	for f := range g.RconConfig {
		files = append(files, f)
	}
	sort.Strings(files)
	var set, same []string
	for _, f := range files {
		var lines []string
		for _, l := range g.RconConfig[f] {
			if v, ok := s.FillLine(g, l); ok {
				lines = append(lines, v)
			}
		}
		changed, err := setLines(filepath.Join(s.InstallDir, filepath.FromSlash(f)), lines)
		if err != nil {
			i.say("[rcon] " + f + ": " + err.Error())
		} else if changed {
			set = append(set, f)
		} else {
			same = append(same, f)
		}
	}
	switch {
	case len(set) > 0:
		i.say("[rcon] its RCON password and port set in " + strings.Join(set, ", "))
	case len(same) > 0:
		i.say("[rcon] its RCON password and port already in " + strings.Join(same, ", "))
	}
}

// newConfig writes a game's config file when it is not there yet, from its entry (Arma's server.cfg: its name,
// password, players): changed after by hand, it is kept.
func (m *Manager) newConfig(i *Instance, s config.Server, g *games.Game) {
	if g.Config == "" || len(g.ConfigNew) == 0 {
		return
	}
	path := g.Path(s.InstallDir, g.Config)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		return
	}
	var lines []string
	for _, l := range g.ConfigNew {
		if v, ok := s.FillLine(g, l); ok {
			lines = append(lines, v)
		}
	}
	nl := "\n"
	if runtime.GOOS == "windows" {
		nl = "\r\n"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err == nil {
		err = os.WriteFile(path, []byte(strings.Join(lines, nl)+nl), 0o644)
		if err == nil {
			i.say("[setup] " + g.Config + " made (its name, password, players): change it on its Set up page")
			return
		}
	}
	i.say("[setup] " + g.Config + ": could not be made")
}

var alnumRe = regexp.MustCompile(`^[A-Za-z0-9]{1,24}$`)

// setLines sets "Key value" lines in a file: a line of the same key (case ignored) is replaced, the others kept,
// a key it does not have added. A file that is not there is made from its newest "_active_" copy when there is
// one (BattlEye renames BEServer_x64.cfg to BEServer_x64_active_<id>.cfg as it starts): its other lines are kept.
// Tells whether it wrote.
func setLines(path string, lines []string) (bool, error) {
	old, err := os.ReadFile(path)
	exists := err == nil
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	if !exists {
		ext := filepath.Ext(path)
		active, _ := filepath.Glob(strings.TrimSuffix(path, ext) + "_active_*" + ext)
		var newest os.FileInfo
		for _, a := range active {
			if st, err := os.Stat(a); err == nil && (newest == nil || st.ModTime().After(newest.ModTime())) {
				if b, err := os.ReadFile(a); err == nil {
					newest, old = st, b
				}
			}
		}
	}
	nl := "\n"
	if strings.Contains(string(old), "\r\n") || (len(old) == 0 && runtime.GOOS == "windows") {
		nl = "\r\n"
	}
	key := func(l string) string {
		f := strings.Fields(l)
		if len(f) == 0 {
			return ""
		}
		return strings.ToLower(f[0])
	}
	want := map[string]string{}
	var order []string
	for _, l := range lines {
		if k := key(l); k != "" {
			if _, dup := want[k]; !dup {
				order = append(order, k)
			}
			want[k] = l
		}
	}
	var out []string
	done := map[string]bool{}
	text := strings.TrimRight(strings.ReplaceAll(string(old), "\r\n", "\n"), "\n")
	if text != "" {
		for _, l := range strings.Split(text, "\n") {
			k := key(l)
			if w, ok := want[k]; ok {
				if !done[k] {
					out = append(out, w)
					done[k] = true
				}
				continue // a key given twice: once
			}
			out = append(out, l)
		}
	}
	for _, k := range order {
		if !done[k] {
			out = append(out, want[k])
		}
	}
	data := strings.Join(out, nl) + nl
	if exists && data == string(old) {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	return true, os.WriteFile(path, []byte(data), 0o644)
}
