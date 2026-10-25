// Package config is the manager's configuration (szm.json): its settings and its servers. It is saved by the web
// interface; it can also be edited by hand while the manager is stopped.
package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/SnipeZilla/szm/internal/games"
)

// Settings of the manager.
type Settings struct {
	Listen      string `json:"listen"`       // address of the web interface, ":8888" (all) or "127.0.0.1:8888"
	SteamCmd    string `json:"steamcmd"`     // steamcmd.exe; empty: <data>/steamcmd/steamcmd.exe, downloaded when missing
	ServersRoot string `json:"servers_root"` // where new servers are installed by default
	CacheDir    string `json:"cache_dir"`    // the shared copies of the games (update mode "cache"); empty: <data>/cache
	Priority    string `json:"priority"`     // default priority of the servers
	// Debug: set by hand in szm.json ("debug": true), read at start: logs/memory.txt gets what the manager uses,
	// every half hour, and the manager's tooltip shows where its memory goes
	Debug bool `json:"debug,omitempty"`
	// ShowManager: set by hand in szm.json ("show_manager": true): the manager's own line (its memory, CPU, network)
	// is shown, and measured; on a hub, its nodes' lines too. Without it, hidden (ManagerHidden).
	ShowManager bool `json:"show_manager,omitempty"`

	Monitor Monitor `json:"monitor"`

	Updates struct {
		Auto          bool `json:"auto"`           // check Steam and update by itself
		CheckInterval int  `json:"check_interval"` // seconds between two checks of a game's version
		WarnSeconds   int  `json:"warn_seconds"`   // players are warned this long before an update (not when empty)
		SayEvery      int  `json:"say_every"`      // seconds between two warnings
	} `json:"updates"`

	Restart struct {
		Mode        string `json:"mode"`         // "daily" at Time, "auto" after 24 h when empty, "never"
		Time        string `json:"time"`         // HH:MM, local time of the machine
		OnlyEmpty   bool   `json:"only_empty"`   // daily: wait until the server is empty (at most 24 h past Time)
		WarnSeconds int    `json:"warn_seconds"` // players are warned this long before a restart
	} `json:"restart"`

	// Backups of each server's settings and plugins (its game folder: cfg, addons…), as zip files
	Backups struct {
		Schedule string   `json:"schedule"` // "manual" (only by hand, and before a change), "daily", "weekly", "monthly"
		Time     string   `json:"time"`     // HH:MM, local time of the machine
		Weekday  int      `json:"weekday"`  // weekly: 0 Sunday … 6 Saturday
		Day      int      `json:"day"`      // monthly: 1…31 (the last day of a shorter month)
		Keep     int      `json:"keep"`     // the newest kept per server
		Before   bool     `json:"before"`   // also before an update, an addons push and a restore
		Dir      string   `json:"dir"`      // where they go; empty: <data>/backups
		Include  []string `json:"include"`  // what is backed up, in the game folder (cfg, addons, *.txt)
		Exclude  []string `json:"exclude"`  // what is left out (logs, *.dem)
		// Off-site copies: each backup is copied there too (the same number kept). rclone reaches Google Drive,
		// OneDrive, Dropbox, SFTP, S3…; a folder can be a synced one (Google Drive, OneDrive) or on another disk.
		Offsite []OffsiteCopy `json:"offsite"`
		RClone  string        `json:"rclone"` // rclone's program; empty: the one in <data>/rclone, else in the PATH
	} `json:"backups"`

	// Discord channels (webhooks) and XenForo forums the events go to: as many as wanted, each with what it gets
	// (a public channel the updates, an admins' channel everything)
	Discords []DiscordHook `json:"discords"`
	Forums   []ForumPost   `json:"forums"`

	// The single Discord webhook and XenForo forum of earlier versions: moved into Discords and Forums at load
	OldDiscord *DiscordHook `json:"discord,omitempty"`
	OldXenForo *ForumPost   `json:"xenforo,omitempty"`

	Logs struct {
		KeepDays int `json:"keep_days"` // the manager's activity log
		// ServerDays: the files of every "logs" folder of the servers (csgo/logs, addons/sourcemod/logs…) older than
		// this are deleted by themselves; 0 keeps them all
		ServerDays int `json:"server_days"`
	} `json:"logs"`

	// Files an update never overwrites in a server ("cache" mode): names, or paths inside the server's folder
	CopyExclude []string `json:"copy_exclude"`

	// Addons: packages (AddonsDir/<package>: metamod2, counterstrikesharp, sourcemod...), each mirroring a server's
	// game folder, copied into the servers that get them. AddonsKeep are the files a push never overwrites (plugin
	// configs): patterns, inside the game folder.
	AddonsDir  string   `json:"addons_dir"`
	AddonsKeep []string `json:"addons_keep"`
}

// OffsiteCopy is where the backups are copied to.
type OffsiteCopy struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`   // "rclone" or "folder"
	Target string `json:"target"` // rclone: remote:path (gdrive:SZM backups); folder: a full path
}

// Monitor is how the servers are watched.
type Monitor struct {
	QueryInterval int `json:"query_interval"` // seconds between two queries (A2S_INFO) of a server
	QueryFailures int `json:"query_failures"` // unanswered queries in a row before a server is taken as frozen
	StartGrace    int `json:"start_grace"`    // seconds a server has to start before it is queried
	StableAfter   int `json:"stable_after"`   // seconds a restarted server must run to be stable again
	GiveUpAfter   int `json:"give_up_after"`  // restarts after crashes in a row; the last one checks the files first. One more crash: it gives up
}

// Server is a game server the manager runs.
type Server struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Game         string   `json:"game"`        // key of the catalog (games.json)
	InstallDir   string   `json:"install_dir"` // the server's folder
	Exe          string   `json:"exe,omitempty"`
	IP           string   `json:"ip"`
	Port         int      `json:"port"`
	Map          string   `json:"map"`
	MaxPlayers   int      `json:"maxplayers"`
	Args         string   `json:"args"`               // more options; they replace a default option of the same name
	GSLT         string   `json:"gslt"`               // Steam game server login token (+sv_setsteamaccount)
	Password     string   `json:"password,omitempty"` // to join it (the games of other engines: {password} of their command line)
	RconPassword string   `json:"rcon_password"`
	SayCommand   string   `json:"say_command"` // say, sm_csay, hlx_sm_csay...
	Priority     string   `json:"priority,omitempty"`
	AutoStart    bool     `json:"autostart"`
	AutoUpdate   *bool    `json:"autoupdate,omitempty"` // nil: the setting of the manager
	UpdateMode   string   `json:"update_mode"`          // "direct": steamcmd into the server; "cache": shared copy, then files copied
	SteamLogin   string   `json:"steam_login,omitempty"`
	Metamod      *bool    `json:"metamod,omitempty"`        // CS2: keep Metamod in gameinfo.gi after updates
	ConsoleLog   bool     `json:"console_log"`              // -condebug: the console is shown in the web interface
	Packages     []string `json:"addon_packages,omitempty"` // the addon packages it gets
	// Vanilla: when the manager gives up on it after crashes an addon caused, it starts it without its addons
	// (Metamod off) while it waits for a fix of that addon, and puts them back with the fix
	Vanilla     bool   `json:"vanilla_while_waiting,omitempty"`
	Restart     string `json:"restart,omitempty"` // "" the manager's, or daily / auto / never
	RestartTime string `json:"restart_time,omitempty"`
	Notes       string `json:"notes,omitempty"`

	// CS2 Workshop: the maps added to it, the one it starts on (+host_workshop_map) and its collection
	// (+host_workshop_collection: the server downloads it and plays its maps)
	Workshop        []WorkshopMap `json:"workshop,omitempty"`
	WorkshopStart   string        `json:"workshop_map,omitempty"`
	WorkshopColl    string        `json:"workshop_collection,omitempty"`
	WorkshopCollFor string        `json:"workshop_collection_title,omitempty"`
}

// AddonSource is where the new versions of an addon package are found, and what is done with them.
type AddonSource struct {
	// Kind: sourcemod, metamod, amxmodx (AlliedModders' builds), counterstrikesharp (its GitHub releases), github
	// (a GitHub project's releases), link (an archive at a fixed address, or a file naming the newest one); "none":
	// not checked
	Kind      string `json:"kind"`
	Branch    string `json:"branch,omitempty"`      // sourcemod 1.12, metamod 2.0 (CS2) or 1.12, amxmodx 1.10
	Repo      string `json:"repo,omitempty"`        // github: owner/name
	Asset     string `json:"asset,omitempty"`       // github: the release's file, * wildcards, {os} is windows or linux
	Pre       bool   `json:"prereleases,omitempty"` // github: pre-releases count too
	NoRuntime bool   `json:"no_runtime,omitempty"`  // counterstrikesharp: the build without the .NET runtime
	URL       string `json:"url,omitempty"`         // link
	// Every: how often it is checked: daily (""), 6h, 1h, off
	Every string `json:"every,omitempty"`
	// Auto: a new version is installed by itself (the package updated, then pushed); else it is only told
	Auto bool `json:"auto,omitempty"`
	// Watch: after crashes this addon caused (or crashes in a row after a game update), checked this often, and
	// the fix installed when it comes out, the servers the manager gave up on started again: 1h (""), 15m, 30m,
	// 3h, off
	Watch string `json:"crash_watch,omitempty"`
	// AllAtOnce: a new version goes to all the servers; else to one first, watched 10 minutes, then the others
	AllAtOnce bool `json:"all_at_once,omitempty"`
}

// DefaultAddonSource is the source a package without one has: the known addon its name is (sourcemod, metamod,
// metamod1, metamod2, counterstrikesharp, amxmodx; a version after it is fine), checked daily, told; nil for another
// name (sourcemod-plugins, metamod-r: not those addons).
func DefaultAddonSource(pkg string) *AddonSource {
	switch strings.TrimRight(strings.ToLower(pkg), "0123456789.-_") {
	case "sourcemod":
		return &AddonSource{Kind: "sourcemod"}
	case "metamod", "mmsource", "metamodsource", "metamod-source", "metamod_source":
		return &AddonSource{Kind: "metamod"}
	case "counterstrikesharp", "cssharp":
		return &AddonSource{Kind: "counterstrikesharp"}
	case "amxmodx", "amxx":
		return &AddonSource{Kind: "amxmodx"}
	}
	return nil
}

// WorkshopMap is a Workshop map added to a server.
type WorkshopMap struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Preview string `json:"preview,omitempty"`
	Size    int64  `json:"size,omitempty"`
	Updated int64  `json:"updated,omitempty"`
}

// DiscordHook is a Discord channel the events go to.
type DiscordHook struct {
	Name     string   `json:"name"`              // for the admin: "Public", "Admins"
	Webhook  string   `json:"webhook"`           // https://discord.com/api/webhooks/...
	Username string   `json:"username"`          // the poster's name
	Events   []string `json:"events"`            // kinds of events posted: crash, failed, update, updated, restart, start, stop, error
	Servers  []string `json:"servers,omitempty"` // only these servers (IDs); none: all, and the manager's own events
}

// ForumPost is a XenForo forum the events go to (its REST API), as replies of a thread, or a new thread each time.
type ForumPost struct {
	Name    string   `json:"name"`
	URL     string   `json:"url"`    // address of the forum (https://snipezilla.com)
	Key     string   `json:"key"`    // API key (Admin CP > Setup > API keys)
	User    int      `json:"user"`   // user ID posting, for a super user key (XF-Api-User)
	Thread  int      `json:"thread"` // posts go as replies of this thread...
	Node    int      `json:"node"`   // ...or, without a thread, each one a new thread of this forum
	Prefix  int      `json:"prefix"` // thread prefix ID of those new threads
	Icons   string   `json:"icons"`  // where the game icons are on the web; empty: attached to each post
	Events  []string `json:"events"` // kinds of events posted
	Servers []string `json:"servers,omitempty"`
}

// Wants tells whether an event of this kind, about this server ("" for the manager), goes there.
func Wants(kinds, servers []string, kind, server string) bool {
	found := false
	for _, k := range kinds {
		if k == kind {
			found = true
			break
		}
	}
	if !found {
		return false
	}
	if len(servers) == 0 {
		return true
	}
	for _, s := range servers {
		if s == server {
			return true
		}
	}
	return false
}

// Config is szm.json.
type Config struct {
	Settings
	Servers []*Server `json:"servers"`
	// AddonUpdates: how each addon package of this machine is kept up to date, by package name (lower case). A
	// package named after a known addon (sourcemod, metamod2, counterstrikesharp, amxmodx) without one checks
	// that addon's releases once a day and tells (DefaultAddonSource). Set on the Addons page.
	AddonUpdates map[string]*AddonSource `json:"addon_updates,omitempty"`

	// Upgraded lists the settings Load brought to new defaults (they were still an old default): saved at start
	Upgraded []string `json:"-"`

	mu   sync.RWMutex
	path string
}

// Defaults of a new configuration.
func Defaults() *Config {
	c := &Config{}
	c.Listen = ":8888"
	c.Priority = "above normal"
	c.Monitor.QueryInterval = 30
	c.Monitor.QueryFailures = 3
	c.Monitor.StartGrace = 120
	c.Monitor.StableAfter = 900
	c.Monitor.GiveUpAfter = 3
	c.Updates.Auto = true
	c.Updates.CheckInterval = 300
	c.Updates.WarnSeconds = 120
	c.Updates.SayEvery = 30
	c.Restart.Mode = "daily"
	c.Restart.Time = "06:00"
	c.Restart.OnlyEmpty = true
	c.Restart.WarnSeconds = 60
	c.Backups.Schedule = "manual"
	c.Backups.Time = "05:30"
	c.Backups.Weekday = 1
	c.Backups.Day = 1
	c.Backups.Keep = 14
	c.Backups.Offsite = []OffsiteCopy{}
	c.Backups.Before = true
	c.Backups.Include = []string{"cfg", "addons", "gameinfo.gi", "*.txt", "*.ini", "*.cfg"}
	c.Backups.Exclude = []string{"logs", "*.log", "*.dem", "*.bsp", "*.vpk", "*.nav", "addons/counterstrikesharp/dotnet"}
	c.Discords = []DiscordHook{}
	c.Forums = []ForumPost{}
	c.Logs.KeepDays = 14
	c.AddonsKeep = []string{"addons/counterstrikesharp/configs", "addons/sourcemod/configs", "addons/sourcemod/data",
		"cfg/sourcemod", "addons/sourcemod/plugins", "addons/amxmodx/configs", "addons/amxmodx/data", "addons/amxmodx/plugins",
		"addons/metamod/metaplugins.ini", "*.db", "*.sq3"}
	c.CopyExclude = []string{"server.cfg", "server_*.cfg", "autoexec.cfg", "gamemode_*.cfg", "gamemodes_server.txt", "motd.txt",
		"mapcycle.txt", "mapcycle_*", "banned_user.cfg", "banned_ip.cfg"}
	return c
}

// Load reads szm.json; a missing file gives the defaults.
func Load(path string) (*Config, error) {
	c := Defaults()
	c.path = path
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	c.Backups.Schedule, c.Backups.Time = "", "" // what the file says (earlier versions: no schedule)
	if err := json.Unmarshal(b, c); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	c.normalize()
	return c, nil
}

func (c *Config) normalize() {
	d := Defaults()
	if c.Listen == "" {
		c.Listen = d.Listen
	}
	if c.Monitor.QueryInterval < 5 {
		c.Monitor.QueryInterval = d.Monitor.QueryInterval
	}
	if c.Monitor.QueryFailures < 1 {
		c.Monitor.QueryFailures = d.Monitor.QueryFailures
	}
	if c.Monitor.StartGrace < 10 {
		c.Monitor.StartGrace = d.Monitor.StartGrace
	}
	if c.Monitor.StableAfter < 10 {
		c.Monitor.StableAfter = d.Monitor.StableAfter
	}
	if c.Monitor.GiveUpAfter < 1 {
		c.Monitor.GiveUpAfter = d.Monitor.GiveUpAfter
	}
	if c.Monitor.GiveUpAfter == 8 { // the old default
		c.Monitor.GiveUpAfter = d.Monitor.GiveUpAfter
		c.Upgraded = append(c.Upgraded, "the restarts after crashes (now 3, the last one after checking the files)")
	}
	if c.Updates.CheckInterval < 60 {
		c.Updates.CheckInterval = d.Updates.CheckInterval
	}
	if c.Updates.SayEvery < 5 {
		c.Updates.SayEvery = d.Updates.SayEvery
	}
	switch c.Restart.Mode {
	case "daily", "auto", "never":
	default:
		c.Restart.Mode = d.Restart.Mode
	}
	if _, _, ok := ParseClock(c.Restart.Time); !ok {
		c.Restart.Time = d.Restart.Time
	}
	if c.Backups.Include == nil && c.Backups.Time == "" {
		// a configuration of before the backups: their defaults
		c.Backups.Before = d.Backups.Before
	}
	switch c.Backups.Schedule {
	case "manual", "daily", "weekly", "monthly":
	case "":
		// Earlier versions: a backup each day at Time, or "off". Their default (05:30) was never chosen: by hand now
		switch c.Backups.Time {
		case "", "off":
			c.Backups.Schedule = "manual"
		case "05:30":
			c.Backups.Schedule = "manual"
			c.Upgraded = append(c.Upgraded, "the backups (now by hand, and before a change; Settings, Backups for daily, weekly or monthly ones)")
		default:
			c.Backups.Schedule = "daily"
		}
	default:
		c.Backups.Schedule = d.Backups.Schedule
	}
	if _, _, ok := ParseClock(c.Backups.Time); !ok {
		c.Backups.Time = d.Backups.Time
	}
	if c.Backups.Weekday < 0 || c.Backups.Weekday > 6 {
		c.Backups.Weekday = d.Backups.Weekday
	}
	if c.Backups.Day < 1 || c.Backups.Day > 31 {
		c.Backups.Day = d.Backups.Day
	}
	if c.Backups.Keep < 1 {
		c.Backups.Keep = d.Backups.Keep
	}
	if c.Backups.Include == nil {
		c.Backups.Include = d.Backups.Include
	}
	if c.Backups.Exclude == nil {
		c.Backups.Exclude = d.Backups.Exclude
	}
	if c.Backups.Offsite == nil {
		c.Backups.Offsite = []OffsiteCopy{}
	}
	// Earlier versions: one Discord webhook, one forum
	if o := c.OldDiscord; o != nil {
		if strings.TrimSpace(o.Webhook) != "" {
			o.Name = "Discord"
			c.Discords = append([]DiscordHook{*o}, c.Discords...)
			c.Upgraded = append(c.Upgraded, "the Discord webhook (now one of a list: Settings, Discord)")
		}
		c.OldDiscord = nil
	}
	if o := c.OldXenForo; o != nil {
		if strings.TrimSpace(o.URL) != "" && strings.TrimSpace(o.Key) != "" {
			o.Name = "Forum"
			if o.Events == nil {
				o.Events = []string{"crash", "failed", "updated"}
			}
			c.Forums = append([]ForumPost{*o}, c.Forums...)
			c.Upgraded = append(c.Upgraded, "the XenForo forum (now one of a list: Settings, XenForo)")
		}
		c.OldXenForo = nil
	}
	if c.Discords == nil {
		c.Discords = []DiscordHook{}
	}
	if c.Forums == nil {
		c.Forums = []ForumPost{}
	}
	for k := range c.Discords {
		if c.Discords[k].Events == nil {
			c.Discords[k].Events = []string{}
		}
	}
	for k := range c.Forums {
		if c.Forums[k].Events == nil {
			c.Forums[k].Events = []string{}
		}
	}
	if c.Logs.KeepDays < 1 {
		c.Logs.KeepDays = d.Logs.KeepDays
	}
	if c.Logs.ServerDays < 0 {
		c.Logs.ServerDays = 0
	}
	if c.CopyExclude == nil || sameAsOne(c.CopyExclude, oldDefaults) {
		if c.CopyExclude != nil {
			c.Upgraded = append(c.Upgraded, "the files an update never overwrites (now with gamemodes_server.txt, the map groups of CS2, and mapcycle_*)")
		}
		c.CopyExclude = d.CopyExclude
	}
	if c.AddonsKeep == nil || sameAsOne(c.AddonsKeep, oldAddonsKeep) {
		if c.AddonsKeep != nil {
			c.Upgraded = append(c.Upgraded, "the files an addons push never overwrites (now with addons/sourcemod/plugins and addons/amxmodx/plugins)")
		}
		c.AddonsKeep = d.AddonsKeep
	}
	for _, s := range c.Servers {
		s.normalize()
	}
}

func (s *Server) normalize() {
	if s.UpdateMode != "cache" {
		s.UpdateMode = "direct"
	}
	if s.SayCommand == "" {
		s.SayCommand = "say"
	}
	if s.SteamLogin == "" {
		s.SteamLogin = "anonymous"
	}
	switch s.Restart {
	case "", "daily", "auto", "never":
	default:
		s.Restart = ""
	}
	s.InstallDir = strings.TrimRight(filepath.Clean(s.InstallDir), `\/`)
}

// oldDefaults are the lists of files never overwritten that earlier versions wrote as defaults. A list still equal
// to one of them was never changed by hand: it gets the current default.
var oldDefaults = [][]string{
	{"server.cfg", "motd.txt", "mapcycle.txt", "autoexec.cfg", "steam.inf"},
	{"server.cfg", "autoexec.cfg", "motd.txt", "mapcycle.txt", "gamemodes_server.txt", "banned_user.cfg", "banned_ip.cfg"},
	{"server.cfg", "server_*.cfg", "autoexec.cfg", "gamemode_*.cfg", "gamemodes_server.txt", "motd.txt", "mapcycle.txt",
		"banned_user.cfg", "banned_ip.cfg"},
	{"server.cfg", "server_*.cfg", "autoexec.cfg", "gamemode_*.cfg", "motd.txt", "mapcycle.txt", "mapcycle_*", "banned_user.cfg",
		"banned_ip.cfg"},
}

// oldAddonsKeep: the same for the files an addons push never overwrites.
var oldAddonsKeep = [][]string{
	{"addons/counterstrikesharp/configs", "addons/sourcemod/configs", "addons/sourcemod/data", "cfg/sourcemod",
		"addons/amxmodx/configs", "addons/amxmodx/data", "addons/metamod/metaplugins.ini", "*.db", "*.sq3"},
}

// sameAsOne says whether a list holds the same names as one of the lists (in any order).
func sameAsOne(list []string, olds [][]string) bool {
	for _, old := range olds {
		if len(old) != len(list) {
			continue
		}
		same := true
		have := map[string]bool{}
		for _, x := range list {
			have[strings.ToLower(strings.TrimSpace(x))] = true
		}
		for _, x := range old {
			if !have[x] {
				same = false
			}
		}
		if same {
			return true
		}
	}
	return false
}

// Save writes szm.json (to a temporary file first, so a crash never leaves half a file).
func (c *Config) Save() error {
	c.mu.RLock()
	b, err := json.MarshalIndent(c, "", "  ")
	c.mu.RUnlock()
	if err != nil {
		return err
	}
	return WriteFileAtomic(c.path, b, 0o600)
}

// Path of szm.json.
func (c *Config) Path() string { return c.path }

// SetPath sets where Save writes.
func (c *Config) SetPath(p string) { c.path = p }

// Lock and Unlock guard the configuration while it is changed.
func (c *Config) Lock()    { c.mu.Lock() }
func (c *Config) Unlock()  { c.mu.Unlock() }
func (c *Config) RLock()   { c.mu.RLock() }
func (c *Config) RUnlock() { c.mu.RUnlock() }

// Within tells whether path is dir or inside it (without case, as Windows compares paths).
func Within(path, dir string) bool {
	if path == "" || dir == "" {
		return false
	}
	p, d := filepath.Clean(path), filepath.Clean(dir)
	if strings.EqualFold(p, d) {
		return true
	}
	if !strings.HasSuffix(d, string(filepath.Separator)) {
		d += string(filepath.Separator)
	}
	return len(p) > len(d) && strings.EqualFold(p[:len(d)], d)
}

// ServerAt gives the server whose folder holds path (or is it), if any.
func (c *Config) ServerAt(path string) (*Server, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, o := range c.Servers {
		if Within(path, o.InstallDir) {
			return o, true
		}
	}
	return nil, false
}

// Server finds a server by ID.
func (c *Config) Server(id string) *Server {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, s := range c.Servers {
		if s.ID == id {
			return s
		}
	}
	return nil
}

// Validate checks a server against the others: its game, folder, address. The errors to show, none when it is right.
func (c *Config) Validate(s *Server, cat games.Catalog) []string {
	var errs []string
	if cat[s.Game] == nil {
		errs = append(errs, "Pick a game.")
	}
	if strings.TrimSpace(s.Name) == "" {
		errs = append(errs, "Give the server a name.")
	}
	if s.InstallDir == "" || s.InstallDir == "." || !filepath.IsAbs(s.InstallDir) {
		errs = append(errs, "Give the server a full folder path (E:\\Counter-Strike 2 - Server 1).")
	}
	if s.Port < 1 || s.Port > 65535 {
		errs = append(errs, "The port is a number from 1 to 65535.")
	}
	if s.RestartTime != "" {
		if _, _, ok := ParseClock(s.RestartTime); !ok {
			errs = append(errs, "The restart time is HH:MM (24 h).")
		}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	var wild []string // the servers on this port at an address of their own, when this one takes every address
	wildPort := s.Port
	for _, o := range c.Servers {
		if o.ID == s.ID {
			continue
		}
		if strings.EqualFold(filepath.Clean(o.InstallDir), filepath.Clean(s.InstallDir)) {
			errs = append(errs, fmt.Sprintf("%s already uses this folder.", o.Name))
		} else if Within(s.InstallDir, o.InstallDir) {
			errs = append(errs, fmt.Sprintf("This folder is inside the one of %s (%s): give the server a folder of its own, next to it.", o.Name, o.InstallDir))
		} else if Within(o.InstallDir, s.InstallDir) {
			errs = append(errs, fmt.Sprintf("This folder holds the one of %s (%s): give the server a folder of its own.", o.Name, o.InstallDir))
		}
		shared := sharedPort(s, cat[s.Game], o, cat[o.Game])
		if shared > 0 && (o.IP == s.IP || o.IP == "" || s.IP == "" || o.IP == "0.0.0.0" || s.IP == "0.0.0.0") {
			if shared != s.Port {
				wildPort = shared
			}
			switch {
			case anyIP(s.IP) && !anyIP(o.IP):
				wild = append(wild, o.Name+" ("+o.IP+")")
			case anyIP(o.IP) && !anyIP(s.IP):
				errs = append(errs, fmt.Sprintf("%s uses port %d on every address of this machine (0.0.0.0), this one included: give it its own IP, or this server another port.", o.Name, shared))
			default:
				errs = append(errs, fmt.Sprintf("%s already uses port %d on this address.", o.Name, shared))
			}
		}
	}
	if len(wild) > 0 {
		errs = append(errs, fmt.Sprintf("0.0.0.0 takes port %d on every address of this machine, and %s use%s it on theirs: give this server its own IP, or a port no other server uses.",
			wildPort, strings.Join(wild, ", "), map[bool]string{true: "s", false: ""}[len(wild) == 1]))
	}
	return errs
}

// UsedPorts: the ports a server takes: its port; for a game of another engine, those after it it needs (Valheim:
// port+1), its query port and its RCON port.
func UsedPorts(s *Server, g *games.Game) []int {
	out := []int{s.Port}
	if g == nil || !g.IsOther() {
		return out
	}
	for k := 1; k < g.PortSpan(); k++ {
		out = append(out, s.Port+k)
	}
	add := func(p int) {
		for _, x := range out {
			if x == p {
				return
			}
		}
		out = append(out, p)
	}
	add(s.Port + g.QueryOffset)
	if g.RconMode() != games.RconNone {
		add(s.Port + g.RconOffset)
	}
	return out
}

// sharedPort: a port two servers would both take (0 none).
func sharedPort(s *Server, sg *games.Game, o *Server, og *games.Game) int {
	theirs := map[int]bool{}
	for _, p := range UsedPorts(o, og) {
		theirs[p] = true
	}
	for _, p := range UsedPorts(s, sg) {
		if theirs[p] {
			return p
		}
	}
	return 0
}

// NewID makes an ID for a server: the game's key and a number.
func (c *Config) NewID(game string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	used := map[string]bool{}
	for _, s := range c.Servers {
		used[s.ID] = true
	}
	for i := 1; ; i++ {
		id := fmt.Sprintf("%s-%d", game, i)
		if !used[id] {
			return id
		}
	}
}

// SortServers orders the servers by game, then name.
func (c *Config) SortServers() {
	c.mu.Lock()
	defer c.mu.Unlock()
	sort.SliceStable(c.Servers, func(i, j int) bool {
		if c.Servers[i].Game != c.Servers[j].Game {
			return c.Servers[i].Game < c.Servers[j].Game
		}
		return strings.ToLower(c.Servers[i].Name) < strings.ToLower(c.Servers[j].Name)
	})
}

// ParseClock reads HH:MM.
func ParseClock(s string) (h, m int, ok bool) {
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) != 2 {
		return 0, 0, false
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	return h, m, err1 == nil && err2 == nil && h >= 0 && h < 24 && m >= 0 && m < 60
}

// RandomPassword makes an RCON password.
func RandomPassword() string {
	b := make([]byte, 9)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// WriteFileAtomic writes a file through a temporary one.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// whose it is: written by root (sudo szm password…), it stays the manager's user's (Linux)
	owner, err := os.Stat(path)
	if err != nil {
		owner, _ = os.Stat(filepath.Dir(path))
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	keepOwner(path, owner)
	return nil
}

/* ---------------------------------------------------------------------------------------------------------------
 * The command line of a server
 * ------------------------------------------------------------------------------------------------------------- */

// An option of a command line and its value ("" for a flag).
type option struct{ key, value string }

var tokenRe = regexp.MustCompile(`"[^"]*"|\S+`)

// SplitArgs splits options as a command line does, keeping "quoted values" together.
func SplitArgs(s string) []string {
	var out []string
	for _, t := range tokenRe.FindAllString(s, -1) {
		out = append(out, strings.Trim(t, `"`))
	}
	return out
}

func isKey(t string) bool { return len(t) > 1 && (t[0] == '-' || t[0] == '+') && !isNumber(t) }

func isNumber(t string) bool { _, err := strconv.ParseFloat(t, 64); return err == nil }

func pairs(tokens []string) []option {
	var out []option
	for i := 0; i < len(tokens); i++ {
		t := tokens[i]
		if isKey(t) && i+1 < len(tokens) && !isKey(tokens[i+1]) {
			out = append(out, option{t, tokens[i+1]})
			i++
			continue
		}
		out = append(out, option{t, ""})
	}
	return out
}

// family groups options that mean the same thing, so that one given replaces the default.
func family(key string) string {
	k := strings.ToLower(key)
	switch k {
	case "-maxplayers", "+maxplayers", "-maxplayers_override":
		return "maxplayers"
	case "-port", "+port", "+hostport":
		return "port"
	case "-ip", "+ip":
		return "ip"
	case "+map", "+changelevel", "+host_workshop_map":
		return "map"
	}
	return k
}

// Args gives the command line of a server: the game's defaults filled in, with the server's own options (which
// replace a default of the same name), its token and -condebug. The map comes last.
func (s *Server) BuildArgs(g *games.Game) []string {
	if g.IsOther() {
		return s.otherArgs(g)
	}
	vals := map[string]string{
		"{game}":       g.Game,
		"{ip}":         s.IP,
		"{port}":       strconv.Itoa(s.Port),
		"{maxplayers}": "",
		"{map}":        s.Map,
	}
	if s.MaxPlayers > 0 {
		vals["{maxplayers}"] = strconv.Itoa(s.MaxPlayers)
	}
	if s.IP == "" || s.IP == "0.0.0.0" {
		vals["{ip}"] = ""
	}

	extra := pairs(SplitArgs(s.Args))
	given := map[string]bool{}
	for _, o := range extra {
		given[family(o.key)] = true
	}

	var defaults, mapOpt []option
	for _, o := range pairs(g.Args) {
		v := o.value
		for ph, val := range vals {
			v = strings.ReplaceAll(v, ph, val)
		}
		if strings.Contains(o.value, "{") && v == "" {
			continue // a value the server does not have: the option is left out
		}
		if given[family(o.key)] {
			continue
		}
		if family(o.key) == "map" {
			mapOpt = append(mapOpt, option{o.key, v})
			continue
		}
		defaults = append(defaults, option{o.key, v})
	}

	// CS2 Workshop: its collection, and the Workshop map it starts on (in place of +map)
	if g.Engine == games.Source2 {
		if s.WorkshopColl != "" && !given["+host_workshop_collection"] {
			extra = append(extra, option{"+host_workshop_collection", s.WorkshopColl})
		}
		if s.WorkshopStart != "" && !given["map"] {
			mapOpt = []option{{"+host_workshop_map", s.WorkshopStart}}
		}
	}

	all := append(defaults, extra...)
	if s.GSLT != "" && g.Engine != games.GoldSrc && !given["+sv_setsteamaccount"] {
		all = append(all, option{"+sv_setsteamaccount", s.GSLT})
	}
	if s.ConsoleLog && !given["-condebug"] {
		all = append(all, option{"-condebug", ""})
	}
	all = append(all, mapOpt...)

	var out []string
	for _, o := range all {
		out = append(out, o.key)
		if o.value != "" {
			out = append(out, o.value)
		}
	}
	return out
}

// otherArgs: the command line of a game of another engine. Its entry gives it whole, with {placeholders}: an option
// left empty is left out (-password {password}, -ServerPassword={password}, ?ServerPassword={password} in a URL);
// the server's own options come after (most of these games take the last of an option given twice).
func (s *Server) otherArgs(g *games.Game) []string {
	vals := s.otherValues(g)
	fill := func(t string) (string, bool) {
		if !strings.Contains(t, "{") {
			return t, true
		}
		// a URL of options (TheIsland?listen?SessionName={name}): a part left empty goes
		if strings.Contains(t, "?") {
			var parts []string
			for _, p := range strings.Split(t, "?") {
				v, ok := fillOne(p, vals)
				if ok {
					parts = append(parts, v)
				}
			}
			return strings.Join(parts, "?"), len(parts) > 0
		}
		return fillOne(t, vals)
	}
	var out []string
	toks := g.CommandLine()
	for k := 0; k < len(toks); k++ {
		t := toks[k]
		// "-name {name}": the option goes with its value when that is left empty
		if isKey(t) && !strings.Contains(t, "{") && k+1 < len(toks) && strings.Contains(toks[k+1], "{") && !strings.Contains(toks[k+1], "?") {
			if v, ok := fillOne(toks[k+1], vals); ok {
				out = append(out, t, v)
			}
			k++
			continue
		}
		if v, ok := fill(t); ok {
			out = append(out, v)
		}
	}
	return append(out, SplitArgs(s.Args)...)
}

// FillLine fills the placeholders of a line for a server of a game of another engine (a line of its RCON's
// config): false when one of them is empty.
func (s *Server) FillLine(g *games.Game, line string) (string, bool) {
	return fillOne(line, s.otherValues(g))
}

// otherValues: the values of the placeholders of a game of another engine, for a server.
func (s *Server) otherValues(g *games.Game) map[string]string {
	ip := s.IP
	if ip == "0.0.0.0" {
		ip = ""
	}
	logfile := ""
	if s.ConsoleLog && g.ConsoleLog != "" {
		logfile = filepath.Join(s.InstallDir, filepath.FromSlash(g.ConsoleLog))
	}
	vals := map[string]string{
		"{ip}":            ip,
		"{port}":          strconv.Itoa(s.Port),
		"{queryport}":     strconv.Itoa(s.Port + g.QueryOffset),
		"{rconport}":      strconv.Itoa(s.Port + g.RconOffset),
		"{maxplayers}":    "",
		"{map}":           s.Map,
		"{name}":          s.Name,
		"{password}":      s.Password,
		"{rcon_password}": s.RconPassword,
		"{gslt}":          s.GSLT,
		"{logfile}":       logfile,
		"{dir}":           s.InstallDir,
		"{game}":          g.Game,
	}
	if s.MaxPlayers > 0 {
		vals["{maxplayers}"] = strconv.Itoa(s.MaxPlayers)
	}
	return vals
}

// fillOne fills the placeholders of a token: left out (false) when one of them is empty, or it is only an
// option's name (-ServerPassword=, +server.ip "").
func fillOne(t string, vals map[string]string) (string, bool) {
	v := t
	for ph, val := range vals {
		if strings.Contains(v, ph) {
			if val == "" {
				return "", false
			}
			v = strings.ReplaceAll(v, ph, val)
		}
	}
	return v, true
}

// tokenLike is what a Steam game server login token looks like.
var tokenLike = regexp.MustCompile(`(?i)(\+sv_setsteamaccount\s+)("?)[0-9A-F]{16,}("?)`)

// HideSecrets hides a server's passwords and token in a text (its command line, for the games of other engines
// that take them there).
func (s *Server) HideSecrets(text string) string {
	for _, v := range []string{s.Password, s.RconPassword, s.GSLT} {
		if len(v) >= 3 {
			text = strings.ReplaceAll(text, v, "••••••")
		}
	}
	return text
}

// Redact hides the login tokens of a text (a command line, a log line).
func Redact(text string) string {
	return tokenLike.ReplaceAllString(text, `${1}${2}••••••••${3}`)
}

// QuoteArgs shows a command line, with quotes around what has spaces.
func QuoteArgs(args []string) string {
	out := make([]string, len(args))
	for i, a := range args {
		if strings.ContainsAny(a, " \t") {
			a = `"` + a + `"`
		}
		out[i] = a
	}
	return strings.Join(out, " ")
}

// ArgValue gives the value of an option of a command line ("+mapgroup mg_bomb": "mg_bomb"), "" when it has none.
func ArgValue(args, key string) string {
	for _, o := range pairs(SplitArgs(args)) {
		if strings.EqualFold(o.key, key) {
			return o.value
		}
	}
	return ""
}

// SetArg gives the command line with an option set to a value: changed where it is, added at the end otherwise.
// The rest of the line stays as it was.
func SetArg(args, key, value string) string {
	locs := tokenRe.FindAllStringIndex(args, -1)
	for i, l := range locs {
		if strings.EqualFold(strings.Trim(args[l[0]:l[1]], `"`), key) {
			if i+1 < len(locs) && !isKey(strings.Trim(args[locs[i+1][0]:locs[i+1][1]], `"`)) {
				v := locs[i+1]
				return args[:v[0]] + value + args[v[1]:]
			}
			return args[:l[1]] + " " + value + args[l[1]:]
		}
	}
	args = strings.TrimRight(args, " ")
	if args != "" {
		args += " "
	}
	return args + key + " " + value
}

// ManagerHidden: the manager's own line is hidden (the default), unless "show_manager": true.
func (c *Config) ManagerHidden() bool { return !c.ShowManager }

// anyIP: every address of the machine ("" or 0.0.0.0)
func anyIP(ip string) bool { return ip == "" || ip == "0.0.0.0" }
