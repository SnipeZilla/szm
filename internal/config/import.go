package config

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/SnipeZilla/szm/internal/games"
)

// The config.xml of SnipeZilla Srcds Manager 1.x.
type oldConfig struct {
	SteamCmd struct {
		Dir      string `xml:"steamcmddir"`
		XCopy    string `xml:"xcopy"`
		Priority string `xml:"priority"`
		CleanLog string `xml:"cleanlog"`
		Restart  string `xml:"restart"`
		Delay    struct {
			Steam, Ping, Start, Failure, Quit, Say string
		} `xml:"delay"`
	} `xml:"steamcmd"`
	Servers []struct {
		AppID        string `xml:"appid,attr"`
		InstallDir   string `xml:"installdir"`
		Srcds        string `xml:"srcds"`
		IP           string `xml:"ip"`
		Port         string `xml:"port"`
		FName        string `xml:"fname"`
		Login        string `xml:"login"`
		Priority     string `xml:"priority"`
		Map          string `xml:"map"`
		MaxPlayers   string `xml:"maxplayers"`
		Cmd          string `xml:"cmd"`
		RconSay      string `xml:"rcon_say"`
		XCopy        string `xml:"xcopy"`
		Metamod      string `xml:"metamod"`
		RconPassword string `xml:"rcon_password"`
	} `xml:"server"`
}

var gsltRe = regexp.MustCompile(`(?i)\+sv_setsteamaccount\s+"?([0-9A-F]{16,})"?`)

// ImportReport says what an import did.
type ImportReport struct {
	Added   []string // names of the servers added
	Skipped []string // why some were left out
	Stopped []string // IDs of the servers that were stopped (sz.stop.txt): they stay stopped
}

// ImportOld reads the config.xml of the old manager (SnipeZilla Srcds Manager 1.x) into this configuration: its
// servers, with their passwords, tokens and options, and the settings that still mean something. Servers already
// here (same folder) are left as they are.
func (c *Config) ImportOld(path string, cat games.Catalog) (*ImportReport, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var old oldConfig
	if err := xml.Unmarshal(b, &old); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	rep := &ImportReport{}
	oldRoot := filepath.Dir(path)
	if abs, err := filepath.Abs(oldRoot); err == nil {
		oldRoot = abs
	}
	cacheMode := strings.EqualFold(strings.TrimSpace(old.SteamCmd.XCopy), "true")

	if p := strings.TrimSpace(old.SteamCmd.Dir); p != "" && c.SteamCmd == "" {
		if !strings.HasSuffix(strings.ToLower(p), ".exe") {
			p = filepath.Join(p, "steamcmd.exe")
		}
		if !filepath.IsAbs(p) {
			p = filepath.Join(oldRoot, p) // relative to the old manager's folder
		}
		c.SteamCmd = p
	}
	if cacheMode && c.CacheDir == "" {
		// The games the old manager already downloaded: apps\<server app id>
		c.CacheDir = filepath.Join(oldRoot, "apps")
	}
	if p := strings.TrimSpace(old.SteamCmd.Priority); p != "" {
		c.Priority = p
	}
	if n, err := strconv.Atoi(strings.TrimSpace(old.SteamCmd.CleanLog)); err == nil && n > 0 {
		c.Logs.KeepDays = n
	}
	switch r := strings.ToLower(strings.TrimSpace(old.SteamCmd.Restart)); {
	case r == "never":
		c.Restart.Mode = "never"
	case r == "auto" || r == "":
		c.Restart.Mode = "auto"
	default:
		if h, err := strconv.Atoi(r); err == nil && h >= 0 && h < 24 {
			c.Restart.Mode = "daily"
			c.Restart.Time = fmt.Sprintf("%02d:00", h)
		}
	}
	setInt := func(dst *int, v string, min int) {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= min {
			*dst = n
		}
	}
	setInt(&c.Monitor.StartGrace, old.SteamCmd.Delay.Start, 30)
	setInt(&c.Monitor.StableAfter, old.SteamCmd.Delay.Start, 30)
	setInt(&c.Updates.WarnSeconds, old.SteamCmd.Delay.Quit, 0)
	setInt(&c.Updates.SayEvery, old.SteamCmd.Delay.Say, 15)

	for i, o := range old.Servers {
		appid, _ := strconv.Atoi(strings.TrimSpace(o.AppID))
		g := cat.ByAppID(appid)
		label := strings.TrimSpace(o.FName)
		if label == "" {
			label = fmt.Sprintf("server %d", i+1)
		}
		if g == nil {
			rep.Skipped = append(rep.Skipped, fmt.Sprintf("%s: game %d is not in the catalog", label, appid))
			continue
		}
		dir := strings.TrimRight(filepath.Clean(strings.TrimSpace(o.InstallDir)), `\/`)
		if dir == "" || dir == "." {
			rep.Skipped = append(rep.Skipped, label+": no installdir")
			continue
		}
		dup := false
		for _, s := range c.Servers {
			if strings.EqualFold(s.InstallDir, dir) {
				dup = true
			}
		}
		if dup {
			rep.Skipped = append(rep.Skipped, label+": already here")
			continue
		}

		s := &Server{
			ID:           c.NewID(g.Key),
			Name:         label,
			Game:         g.Key,
			InstallDir:   dir,
			IP:           strings.TrimSpace(o.IP),
			Map:          strings.TrimSpace(o.Map),
			RconPassword: strings.TrimSpace(o.RconPassword),
			SayCommand:   strings.TrimSpace(o.RconSay),
			Priority:     strings.TrimSpace(o.Priority),
			AutoStart:    true,
			ConsoleLog:   true,
			UpdateMode:   "direct",
		}
		s.Port, _ = strconv.Atoi(strings.TrimSpace(o.Port))
		s.MaxPlayers, _ = strconv.Atoi(strings.TrimSpace(o.MaxPlayers))
		if s.SayCommand == "none" {
			s.SayCommand = "say"
		}
		if l := strings.TrimSpace(o.Login); l != "" {
			s.SteamLogin = l
		}
		mode := cacheMode
		if x := strings.TrimSpace(o.XCopy); x != "" {
			mode = strings.EqualFold(x, "true")
		}
		if mode {
			s.UpdateMode = "cache"
		}
		if m := strings.TrimSpace(o.Metamod); m != "" {
			v := strings.EqualFold(m, "true")
			s.Metamod = &v
		}

		// The program: kept when it is not the one the catalog would pick
		if srcds := strings.TrimSpace(o.Srcds); srcds != "" {
			rel, err := filepath.Rel(dir, srcds)
			known := false
			for _, cand := range g.ExeCandidates() {
				if err == nil && strings.EqualFold(rel, cand) {
					known = true
				}
			}
			if !known || (len(g.ExeCandidates()) > 0 && err == nil && !strings.EqualFold(rel, g.ExeCandidates()[0])) {
				s.Exe = srcds
			}
		}

		// Options: those of sz.cmd.cfg (the web interface's) when there is one, else the config's. The token goes in
		// its own field; what the catalog gives (-dedicated, -console, -usercon) is left out.
		cmd := strings.TrimSpace(o.Cmd)
		if b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(g.GameDir), "cfg", "sz.cmd.cfg")); err == nil && strings.TrimSpace(string(b)) != "" {
			cmd = strings.TrimSpace(string(b))
		}
		if m := gsltRe.FindStringSubmatch(cmd); m != nil {
			s.GSLT = m[1]
			cmd = gsltRe.ReplaceAllString(cmd, "")
		}
		var keep []string
		for _, p := range pairs(SplitArgs(cmd)) {
			k := strings.ToLower(p.key)
			if k == "-dedicated" || k == "-console" || k == "-usercon" || k == "-game" {
				continue
			}
			if family(k) == "ip" || family(k) == "port" {
				continue
			}
			if family(k) == "map" && s.Map == "" {
				s.Map = p.value
				continue
			}
			if family(k) == "maxplayers" && k != "-maxplayers_override" && s.MaxPlayers == 0 {
				s.MaxPlayers, _ = strconv.Atoi(p.value)
				continue
			}
			if p.value != "" {
				keep = append(keep, p.key, quoteIfSpace(p.value))
			} else {
				keep = append(keep, p.key)
			}
		}
		s.Args = strings.Join(keep, " ")

		// Stopped in the old manager (sz.stop.txt): it stays stopped
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(g.GameDir), "sz.stop.txt")); err == nil {
			rep.Stopped = append(rep.Stopped, s.ID)
		}

		s.normalize()
		c.Servers = append(c.Servers, s)
		rep.Added = append(rep.Added, s.Name)
	}
	return rep, nil
}

func quoteIfSpace(v string) string {
	if strings.ContainsAny(v, " \t") {
		return `"` + v + `"`
	}
	return v
}
