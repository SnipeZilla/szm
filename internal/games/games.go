// Package games describes the games the manager can install and run: which dedicated server steamcmd installs, the
// program to start, its default command line and where it keeps its version, console log, config and maps.
package games

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

//go:embed games.json
var builtin []byte

// Engines
const (
	GoldSrc = "goldsrc"
	Source  = "source"
	Source2 = "source2"
	// Other: the games of other engines (Unity, Unreal…: Rust, 7 Days to Die, ARK…). Each says how its server is
	// started, queried and stopped: its query and RCON ports may differ from its game port, many have no RCON
	// (they are stopped with Ctrl+C, which saves), and their version is the build steamcmd installed.
	Other = "other"
)

// Ways a server is queried (Game.Query)
const (
	QueryStrict = ""     // the Source engines: a server that stops answering is frozen, and is restarted
	QueryTry    = "try"  // asked for its players and map; its silence is not taken as frozen
	QueryNone   = "none" // not asked: its process says it runs
)

// RCON protocols (Game.Rcon)
const (
	RconSource = "source"  // Source RCON (TCP): ARK, Conan Exiles, Palworld, Squad…
	RconWeb    = "webrcon" // Rust's WebSocket RCON
	RconTelnet = "telnet"  // 7 Days to Die's telnet console
	// RconBattlEye: BattlEye RCon (UDP), DayZ's and Arma's
	RconBattlEye = "battleye"
	RconNone     = "none"
)

// Game is an entry of the catalog.
type Game struct {
	Key          string              `json:"key"`
	Name         string              `json:"name"`
	Short        string              `json:"short"`
	Icon         string              `json:"icon,omitempty"` // static/games/<icon>.png of the web interface
	Engine       string              `json:"engine"`
	AppID        int                 `json:"appid"`
	ServerAppID  int                 `json:"server_appid"`
	Game         string              `json:"game"`
	GameDir      string              `json:"game_dir,omitempty"`
	AppSetConfig string              `json:"app_set_config,omitempty"`
	Login        bool                `json:"login,omitempty"`
	Exe          map[string][]string `json:"exe"`
	Args         []string            `json:"args,omitempty"`
	DefaultMap   string              `json:"default_map"`
	DefaultMax   int                 `json:"default_maxplayers"`
	Metamod      bool                `json:"metamod,omitempty"`
	VersionFile  string              `json:"version_file,omitempty"`
	ConsoleLog   string              `json:"console_log,omitempty"`
	// ConsoleCapture: the server writes no console log of its own (CS2 ignores -condebug): the manager writes
	// ConsoleLog for it, with what it prints (Linux), or what its console window shows (Windows).
	ConsoleCapture bool     `json:"console_capture,omitempty"`
	Config         string   `json:"config,omitempty"`
	MapsDir        string   `json:"maps_dir,omitempty"`
	MapExt         []string `json:"map_ext,omitempty"`
	URL            string   `json:"url,omitempty"` // its web page; empty: its Steam store page (AppID)

	// The games of other engines: what the manager knows of their server (the Source engines need none of it)
	ArgsOS      map[string][]string `json:"args_os,omitempty"`      // the command line on one system, in place of args
	Query       string              `json:"query,omitempty"`        // QueryStrict, QueryTry (other engines' default), QueryNone
	QueryOffset int                 `json:"query_offset,omitempty"` // its Steam query port is its port + this ({queryport})
	Rcon        string              `json:"rcon,omitempty"`         // RconSource (Source engines' default), RconWeb, RconTelnet, RconNone (other engines' default)
	RconOffset  int                 `json:"rcon_offset,omitempty"`  // its RCON port is its port + this ({rconport})
	Stop        []string            `json:"stop,omitempty"`         // the RCON commands that save and end it; else Ctrl+C (other engines)
	StopWait    int                 `json:"stop_wait,omitempty"`    // seconds it has to end (save) before it is ended by force
	Say         string              `json:"say,omitempty"`          // the RCON command that writes to the players: "say {text}"
	Env         map[string]string   `json:"env,omitempty"`          // environment of its program ({dir}: its folder): LD_LIBRARY_PATH
	Ports       int                 `json:"ports,omitempty"`        // the ports it takes from its port on (Valheim: 2, port and port+1)
	DefaultPort int                 `json:"default_port,omitempty"` // the port of a first server
	Notes       string              `json:"notes,omitempty"`        // shown when a server is added: what to know
	// RconConfig: files its RCON is set in, and their lines the manager sets before it starts ("Key value", with
	// placeholders; its other lines are kept): BattlEye's BEServer_x64.cfg. Only with an RCON password.
	RconConfig map[string][]string `json:"rcon_config,omitempty"`
	// ConfigNew: the lines of its Config file when it is not there yet, written as a server starts (placeholders
	// filled in; a line whose value is empty left out): Arma's server.cfg
	ConfigNew []string `json:"config_new,omitempty"`
}

// IsOther tells whether the game is of another engine than GoldSrc and Source.
func (g *Game) IsOther() bool { return g.Engine == Other }

// QueryMode: how its servers are queried.
func (g *Game) QueryMode() string {
	if g.Query == "" && g.IsOther() {
		return QueryTry
	}
	return g.Query
}

// RconMode: the RCON protocol of its servers.
func (g *Game) RconMode() string {
	switch {
	case g.Rcon != "":
		return g.Rcon
	case g.IsOther():
		return RconNone
	}
	return RconSource
}

// StopTimeout: how long a server has to end by itself.
func (g *Game) StopTimeout() time.Duration {
	switch {
	case g.StopWait > 0:
		return time.Duration(g.StopWait) * time.Second
	case g.IsOther():
		return 2 * time.Minute // it saves its world first
	}
	return 15 * time.Second
}

// PortSpan: the ports its servers take from their port on.
func (g *Game) PortSpan() int {
	if g.Ports > 1 {
		return g.Ports
	}
	return 1
}

// FirstPort: the port a first server of the game gets.
func (g *Game) FirstPort() int {
	if g.DefaultPort > 0 {
		return g.DefaultPort
	}
	return 27015
}

// CommandLine: its default command line on this system.
func (g *Game) CommandLine() []string {
	if a, ok := g.ArgsOS[runtime.GOOS]; ok && len(a) > 0 {
		return a
	}
	return g.Args
}

// OSPath fills {os} of a path of the game: Windows, Linux (ShooterGame/Saved/Config/{os}Server).
func OSPath(p string) string {
	osn := "Linux"
	if runtime.GOOS == "windows" {
		osn = "Windows"
	}
	return strings.ReplaceAll(p, "{os}", osn)
}

// Catalog is the list of games, by key.
type Catalog map[string]*Game

// Load reads the built-in catalog, then dataDir/games.json when there is one (its entries replace or add games).
func Load(dataDir string) (Catalog, error) {
	cat := Catalog{}
	if err := cat.merge(builtin); err != nil {
		return nil, fmt.Errorf("built-in games: %w", err)
	}
	if dataDir != "" {
		if b, err := os.ReadFile(CustomPath(dataDir)); err == nil {
			if err := cat.merge(b); err != nil {
				return nil, fmt.Errorf("games.json: %w", err)
			}
		}
	}
	return cat, nil
}

// LoadOrBuiltin is Load, but a games.json that cannot be read does not stop the manager: the built-in games are
// given, with the error to show.
func LoadOrBuiltin(dataDir string) (Catalog, error) {
	cat, err := Load(dataDir)
	if err == nil {
		return cat, nil
	}
	if b, berr := Load(""); berr == nil {
		return b, err
	}
	return nil, err
}

// CustomPath is the games.json of a data folder (the games added or changed: read again when it changes).
func CustomPath(dataDir string) string { return filepath.Join(dataDir, "games.json") }

func (c Catalog) merge(b []byte) error {
	var doc struct {
		Games map[string]*Game `json:"games"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return where(b, err)
	}
	for key, g := range doc.Games {
		if g == nil || g.ServerAppID == 0 || g.Engine == "" {
			return fmt.Errorf("game %q needs engine and server_appid", key)
		}
		g.Key = key
		g.fill()
		c[key] = g
	}
	return nil
}

// fill gives a game the defaults of its engine for what its entry does not say.
func (g *Game) fill() {
	if g.Short == "" {
		g.Short = g.Name
	}
	if g.GameDir == "" {
		g.GameDir = g.Game
	}
	if g.IsOther() {
		// Its entry says it all: no steam.inf (its version is the build steamcmd installed), no maps folder; its
		// console log and config are where the entry says
		g.ConsoleLog, g.Config = OSPath(g.ConsoleLog), OSPath(g.Config)
		if g.DefaultMax <= 0 {
			g.DefaultMax = 16
		}
		return
	}
	if len(g.Args) == 0 {
		switch g.Engine {
		case GoldSrc:
			g.Args = []string{"-console", "-game", "{game}", "+ip", "{ip}", "-port", "{port}", "+maxplayers", "{maxplayers}", "+map", "{map}"}
		case Source2:
			g.Args = []string{"-dedicated", "-console", "-usercon", "-ip", "{ip}", "-port", "{port}", "-maxplayers", "{maxplayers}", "+map", "{map}"}
		default:
			g.Args = []string{"-console", "-game", "{game}", "-usercon", "-ip", "{ip}", "-port", "{port}", "-maxplayers", "{maxplayers}", "+map", "{map}"}
		}
	}
	if g.VersionFile == "" {
		g.VersionFile = g.GameDir + "/steam.inf"
	}
	if g.ConsoleLog == "" {
		if g.Engine == GoldSrc {
			g.ConsoleLog = g.GameDir + "/qconsole.log"
		} else {
			g.ConsoleLog = g.GameDir + "/console.log"
		}
	}
	if g.Config == "" {
		if g.Engine == GoldSrc {
			g.Config = g.GameDir + "/server.cfg"
		} else {
			g.Config = g.GameDir + "/cfg/server.cfg"
		}
	}
	if g.MapsDir == "" {
		g.MapsDir = g.GameDir + "/maps"
	}
	if len(g.MapExt) == 0 {
		if g.Engine == Source2 {
			g.MapExt = []string{".vpk"}
		} else {
			g.MapExt = []string{".bsp"}
		}
	}
}

// Sorted gives the games by name.
func (c Catalog) Sorted() []*Game {
	list := make([]*Game, 0, len(c))
	for _, g := range c {
		list = append(list, g)
	}
	sort.Slice(list, func(i, j int) bool { return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name) })
	return list
}

// ByAppID finds a game by its Steam app ID (the client's, as the old config.xml names them), or by its server's.
func (c Catalog) ByAppID(appid int) *Game {
	for _, g := range c.Sorted() {
		if g.AppID == appid {
			return g
		}
	}
	for _, g := range c.Sorted() {
		if g.ServerAppID == appid {
			return g
		}
	}
	return nil
}

// ExeCandidates are the programs that can start the server on this system, relative to its folder.
func (g *Game) ExeCandidates() []string {
	list := g.Exe[runtime.GOOS]
	out := make([]string, len(list))
	for i, p := range list {
		out[i] = filepath.FromSlash(p)
	}
	return out
}

// FindExe gives the first program of the game that exists in a server's folder ("" when none is there yet), and
// the one to expect.
func (g *Game) FindExe(installDir string) (found, expected string) {
	list := g.ExeCandidates()
	for _, p := range list {
		full := filepath.Join(installDir, p)
		if st, err := os.Stat(full); err == nil && !st.IsDir() {
			return full, full
		}
	}
	if len(list) > 0 {
		return "", filepath.Join(installDir, list[0])
	}
	return "", ""
}

// Path gives a path of the game (one of its fields, with /) inside a server's folder.
func (g *Game) Path(installDir, rel string) string {
	return filepath.Join(installDir, filepath.FromSlash(rel))
}

// where adds the line and column to a JSON error ("line 97, column 7: invalid character..."), to find it in the file.
func where(b []byte, err error) error {
	var off int64 = -1
	var se *json.SyntaxError
	var te *json.UnmarshalTypeError
	switch {
	case errors.As(err, &se):
		off = se.Offset
	case errors.As(err, &te):
		off = te.Offset
	}
	if off < 0 || off > int64(len(b)) {
		return err
	}
	if off > 0 {
		off-- // the offset is just after the character at fault
	}
	line, col := 1, 1
	for _, c := range b[:off] {
		if c == '\n' {
			line, col = line+1, 1
		} else {
			col++
		}
	}
	return fmt.Errorf("line %d, column %d: %w (a comma or a quote missing just before?)", line, col, err)
}
