package games

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// The games added by the admin live in <data>/games.json: an entry with the key of a built-in game replaces it.

// Builtin gives the games that come with the manager.
func Builtin() Catalog {
	c := Catalog{}
	_ = c.merge(builtin)
	return c
}

// LoadCustom reads the games of <data>/games.json as written (without the defaults Load fills in): to edit them.
func LoadCustom(dataDir string) (map[string]*Game, error) {
	out := map[string]*Game{}
	b, err := os.ReadFile(filepath.Join(dataDir, "games.json"))
	if errors.Is(err, os.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	var doc struct {
		Games map[string]*Game `json:"games"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("games.json: %w", where(b, err))
	}
	for k, g := range doc.Games {
		if g != nil {
			g.Key = k
			out[k] = g
		}
	}
	return out, nil
}

// SaveCustom writes <data>/games.json.
func SaveCustom(dataDir string, list map[string]*Game) error {
	doc := struct {
		Comment string           `json:"_comment"`
		Games   map[string]*Game `json:"games"`
	}{"Games added in the manager (its Games page). An entry with the key of a built-in game replaces it.", list}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(dataDir, "games.json.tmp")
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dataDir, "games.json"))
}

var keyRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

// Check says what is missing or wrong in a game entry (nil when it can be used). It fills what can be guessed: the
// short name, the programs of its engine, the app_set_config of a GoldSrc mod.
func (g *Game) Check() error {
	g.Name = strings.TrimSpace(g.Name)
	g.Short = strings.TrimSpace(g.Short)
	g.Game = strings.Trim(strings.TrimSpace(g.Game), `/\`)
	if !keyRe.MatchString(g.Key) {
		return errors.New("the key is lowercase letters and digits (csgo, fof)")
	}
	if g.Name == "" {
		return errors.New("give the game a name")
	}
	switch g.Engine {
	case GoldSrc, Source, Source2, Other:
	default:
		return errors.New("pick its engine: GoldSrc, Source, Source 2 or another")
	}
	if g.ServerAppID <= 0 {
		return errors.New("give the app ID of its dedicated server (steamcmd)")
	}
	if g.IsOther() {
		return g.checkOther()
	}
	if g.Game == "" || strings.ContainsAny(g.Game, `<>:"|?*`) {
		return errors.New("give its game folder (-game): cstrike, tf, csgo…")
	}
	if g.Short == "" {
		g.Short = g.Name
	}
	g.URL = strings.TrimSpace(g.URL)
	if g.URL != "" {
		if u, err := url.Parse(g.URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return errors.New("its web page is an address starting with https://")
		}
	}
	if g.Exe == nil {
		g.Exe = map[string][]string{}
	}
	if len(g.Exe["windows"]) == 0 {
		switch g.Engine {
		case GoldSrc:
			g.Exe["windows"] = []string{"hlds.exe"}
		case Source:
			g.Exe["windows"] = []string{"srcds_win64.exe", "srcds.exe"}
		default:
			return errors.New("give its program on Windows (game/bin/win64/<name>.exe for Source 2)")
		}
	}
	if len(g.Exe["linux"]) == 0 {
		switch g.Engine {
		case GoldSrc:
			g.Exe["linux"] = []string{"hlds_run"}
		case Source:
			g.Exe["linux"] = []string{"srcds_run_64", "srcds_run"}
		}
	}
	if g.Engine == GoldSrc && g.ServerAppID == 90 && g.AppSetConfig == "" && g.Game != "valve" {
		g.AppSetConfig = "90 mod " + g.Game // HLDS mods are chosen this way
	}
	if g.DefaultMax <= 0 {
		g.DefaultMax = 24
	}
	return nil
}

// checkOther checks the entry of a game of another engine: its program, its command line, what is not Source's.
func (g *Game) checkOther() error {
	if g.Short == "" {
		g.Short = g.Name
	}
	g.Game = strings.Trim(strings.TrimSpace(g.Game), `/\`)
	if len(g.Exe["windows"]) == 0 && len(g.Exe["linux"]) == 0 {
		return errors.New("give its program, on Windows or on Linux (its path in the server's folder)")
	}
	if len(g.Args) == 0 && len(g.ArgsOS) == 0 {
		return errors.New("give its command line: {port}, {queryport}, {rconport}, {name}, {password}, {rcon_password}, {maxplayers}, {map} and {logfile} are filled in")
	}
	switch g.Query {
	case QueryStrict, QueryTry, QueryNone:
	default:
		return errors.New("its query: try, none, or empty (it must answer)")
	}
	switch g.Rcon {
	case "", RconSource, RconWeb, RconTelnet, RconBattlEye, RconNone:
	default:
		return errors.New("its RCON: source, webrcon, telnet, battleye or none")
	}
	for f := range g.RconConfig {
		if f == "" || filepath.IsAbs(f) || strings.Contains(f, ":") || strings.HasPrefix(f, "/") || strings.HasPrefix(f, `\`) || slices.Contains(strings.FieldsFunc(f, func(r rune) bool { return r == '/' || r == '\\' }), "..") {
			return errors.New("its RCON's files: paths in the server's folder")
		}
	}
	if g.StopWait < 0 || g.StopWait > 3600 || g.Ports < 0 || g.Ports > 100 || g.DefaultPort < 0 || g.DefaultPort > 65535 {
		return errors.New("a number of its entry is out of range")
	}
	if g.DefaultMax <= 0 {
		g.DefaultMax = 16
	}
	return nil
}
