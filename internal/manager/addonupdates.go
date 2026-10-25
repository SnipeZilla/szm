package manager

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SnipeZilla/szm/internal/config"
	"github.com/SnipeZilla/szm/internal/crashinfo"
	"github.com/SnipeZilla/szm/internal/events"
	"github.com/SnipeZilla/szm/internal/games"
)

// Addon updates: a package can have a source (config.AddonSource) where the new versions of its addon come out:
// AlliedModders' builds (SourceMod, Metamod:Source, AMX Mod X), GitHub releases (CounterStrikeSharp, any project),
// or a link. Each machine checks its own packages, as often as each says (daily by default), and tells of a new
// version, or installs it: the archive goes into the package (its files written over the package's, the files a
// push never overwrites kept), then into the servers that get it, one first (watched 10 minutes), then the others.
// After crashes an addon caused, its package is checked more often, and the fix installed when it comes out; the
// servers the manager gave up on are started again.

// Where the sources are (the tests point them elsewhere)
var (
	smDrop    = "https://sm.alliedmods.net/smdrop"
	mmsDrop   = "https://mms.alliedmods.net/mmsdrop"
	amxxDrop  = "https://www.amxmodx.org/amxxdrop"
	githubAPI = "https://api.github.com"
)

// How long a new version is watched on the first server, and when the first check is made after the start
var (
	addonTestFor    = 10 * time.Minute
	addonFirstCheck = 2 * time.Minute
	addonWaitStep   = 10 * time.Second
)

var addonHTTP = &http.Client{Timeout: 60 * time.Second}

// ErrAddonBusy: the package is being checked or installed already.
var ErrAddonBusy = errors.New("the package is being checked or installed already")

// Release is a version of an addon, as its source gives it.
type Release struct {
	Version   string `json:"version"`
	File      string `json:"file"`           // the archive's name
	URL       string `json:"url"`            // where it is downloaded
	Page      string `json:"page,omitempty"` // its release notes, or the downloads page
	Published int64  `json:"published,omitempty"`
}

// CrashWatch: a package checked more often after crashes its addon caused, until the fix is in.
type CrashWatch struct {
	Since   int64    `json:"since"`
	Last    int64    `json:"last"`              // the last of those crashes
	Why     string   `json:"why"`               // "CS2 - Server 1 crashed in SourceMod extension dhooks"
	Base    string   `json:"base,omitempty"`    // the version out then: a fix is a newer one
	Servers []string `json:"servers"`           // the servers that crashed (IDs)
	Vanilla []string `json:"vanilla,omitempty"` // started without their addons meanwhile
}

// Rollout: a new version on its way into the servers.
type Rollout struct {
	Version string   `json:"version"`
	Started int64    `json:"started"`
	First   string   `json:"first,omitempty"` // the server it went to first (ID)
	Until   int64    `json:"until,omitempty"` // ... watched until then
	Rest    []string `json:"rest,omitempty"`  // still to get it
	Done    []string `json:"done,omitempty"`
	Failed  []string `json:"failed,omitempty"` // could not get it (not installed, files in use…)
	Held    string   `json:"held,omitempty"`   // why the others did not get it
	Revive  []string `json:"revive,omitempty"`
	Fix     bool     `json:"fix,omitempty"` // after crashes: the servers given up on meanwhile are started again too
	By      string   `json:"by,omitempty"`
}

type addonPkgState struct {
	Installed     string      `json:"installed,omitempty"` // the version in the package ("" not known)
	InstalledFile string      `json:"installed_file,omitempty"`
	InstalledAt   int64       `json:"installed_at,omitempty"`
	Seen          string      `json:"seen,omitempty"`     // with Installed not known: the version out at the first check
	Compared      string      `json:"compared,omitempty"` // with Installed not known: the version its files were compared with
	Checked       int64       `json:"checked,omitempty"`
	Error         string      `json:"error,omitempty"` // the last check's
	Latest        *Release    `json:"latest,omitempty"`
	Told          string      `json:"told,omitempty"`      // the version already told of
	NoSource      int64       `json:"no_source,omitempty"` // when it was told it has no source (crashes)
	Watch         *CrashWatch `json:"watch,omitempty"`
	Rollout       *Rollout    `json:"rollout,omitempty"`
}

type addonStates struct {
	path string
	mu   sync.Mutex
	Pkgs map[string]*addonPkgState `json:"packages"`
	busy map[string]string         // what is being done with a package, in words
}

func loadAddonStates(p string) *addonStates {
	a := &addonStates{path: p, Pkgs: map[string]*addonPkgState{}, busy: map[string]string{}}
	if b, err := os.ReadFile(p); err == nil {
		_ = json.Unmarshal(b, a)
		if a.Pkgs == nil {
			a.Pkgs = map[string]*addonPkgState{}
		}
	}
	return a
}

// get gives a copy of a package's state.
func (a *addonStates) get(name string) addonPkgState {
	if a == nil { // a manager made bare (tests)
		return addonPkgState{}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if v := a.Pkgs[strings.ToLower(name)]; v != nil {
		c := *v
		if v.Latest != nil {
			l := *v.Latest
			c.Latest = &l
		}
		if v.Watch != nil {
			w := *v.Watch
			w.Servers, w.Vanilla = append([]string(nil), w.Servers...), append([]string(nil), w.Vanilla...)
			c.Watch = &w
		}
		if v.Rollout != nil {
			r := *v.Rollout
			r.Rest, r.Done, r.Revive, r.Failed = append([]string(nil), r.Rest...), append([]string(nil), r.Done...), append([]string(nil), r.Revive...), append([]string(nil), r.Failed...)
			c.Rollout = &r
		}
		return c
	}
	return addonPkgState{}
}

// update changes a package's state and saves.
func (a *addonStates) update(name string, fn func(*addonPkgState)) {
	if a == nil { // a manager made bare (tests)
		return
	}
	a.mu.Lock()
	k := strings.ToLower(name)
	v := a.Pkgs[k]
	if v == nil {
		v = &addonPkgState{}
		a.Pkgs[k] = v
	}
	fn(v)
	b, err := json.MarshalIndent(a, "", "  ")
	a.mu.Unlock()
	if err == nil {
		_ = config.WriteFileAtomic(a.path, b, 0o644)
	}
}

func (a *addonStates) remove(name string) {
	if a == nil { // a manager made bare (tests)
		return
	}
	a.mu.Lock()
	delete(a.Pkgs, strings.ToLower(name))
	b, err := json.MarshalIndent(a, "", "  ")
	a.mu.Unlock()
	if err == nil {
		_ = config.WriteFileAtomic(a.path, b, 0o644)
	}
}

// setBusy says what is being done with a package ("" done); claim: only when nothing is.
func (a *addonStates) setBusy(name, what string, claim bool) bool {
	if a == nil {
		return true
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	k := strings.ToLower(name)
	if claim && a.busy[k] != "" {
		return false
	}
	if what == "" {
		delete(a.busy, k)
	} else {
		a.busy[k] = what
	}
	return true
}

func (a *addonStates) busyOf(name string) string {
	if a == nil { // a manager made bare (tests)
		return ""
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.busy[strings.ToLower(name)]
}

/* ---------------------------------------------------------------------------------------------------------------
 * Sources
 * ------------------------------------------------------------------------------------------------------------- */

// AddonSourceOf is where a package's new versions are found: what was set for it, else the default its name gives
// (set false). nil: not checked.
func (m *Manager) AddonSourceOf(name string) (src *config.AddonSource, set bool) {
	m.Cfg.RLock()
	if v := m.Cfg.AddonUpdates[strings.ToLower(name)]; v != nil {
		c := *v
		src, set = &c, true
	}
	m.Cfg.RUnlock()
	if src == nil {
		src = config.DefaultAddonSource(name)
		if src != nil && !m.defaultFits(name, src.Kind) {
			src = nil
		}
	}
	if src != nil && (src.Kind == "" || src.Kind == "none") {
		return nil, set
	}
	if src != nil && src.Branch == "" {
		src.Branch = m.defaultBranch(name, src.Kind)
	}
	return src, set
}

// defaultFits: the addon a package's name says is for the games of the servers that get it (Metamod:Source,
// SourceMod and CounterStrikeSharp not for GoldSrc, whose metamod is another; AMX Mod X only for GoldSrc).
func (m *Manager) defaultFits(name, kind string) bool {
	cat := m.Catalog()
	for _, s := range m.serversOf(name) {
		g := cat[s.Game]
		if g == nil {
			continue
		}
		if (kind == "amxmodx") != (g.Engine == games.GoldSrc) {
			return false
		}
	}
	return true
}

// defaultBranch: SourceMod 1.12, AMX Mod X 1.10; Metamod 2.0 for CS2 (a name with 2, or a CS2 server getting it), 1.12
// for the Source games.
func (m *Manager) defaultBranch(name, kind string) string {
	switch kind {
	case "sourcemod":
		return "1.12"
	case "amxmodx":
		return "1.10"
	case "metamod":
		n := strings.ToLower(name)
		switch {
		case strings.Contains(n, "2"):
			return "2.0"
		case strings.Contains(n, "1"):
			return "1.12"
		}
		cat := m.Catalog()
		for _, s := range m.serversOf(name) {
			if g := cat[s.Game]; g != nil && !g.Metamod {
				return "1.12"
			}
		}
		return "2.0"
	}
	return ""
}

// SetAddonSource sets where a package's new versions are found (nil: back to the default its name gives).
func (m *Manager) SetAddonSource(name string, src *config.AddonSource, by string) error {
	if _, err := m.PackagePath(name); err != nil {
		return err
	}
	if src != nil {
		if err := checkSource(src); err != nil {
			return err
		}
	}
	before, _ := m.AddonSourceOf(name)
	m.Cfg.Lock()
	if src == nil {
		delete(m.Cfg.AddonUpdates, strings.ToLower(name))
	} else {
		if m.Cfg.AddonUpdates == nil {
			m.Cfg.AddonUpdates = map[string]*config.AddonSource{}
		}
		c := *src
		m.Cfg.AddonUpdates[strings.ToLower(name)] = &c
	}
	m.Cfg.Unlock()
	if err := m.Cfg.Save(); err != nil {
		return err
	}
	m.Log.Add(events.Manager, "", "", "Updates of the addon package %s set by %s: %s", name, by, m.describeSource(name))
	after, _ := m.AddonSourceOf(name)
	moved := before == nil || after == nil || before.Kind != after.Kind || before.Branch != after.Branch || before.Repo != after.Repo ||
		before.Asset != after.Asset || before.Pre != after.Pre || before.NoRuntime != after.NoRuntime || before.URL != after.URL
	m.addons.update(name, func(v *addonPkgState) {
		v.Checked, v.Error = 0, "" // checked again soon
		if moved {
			v.Latest, v.Told, v.Seen, v.Compared = nil, "", "", "" // what another source said
		}
	})
	return nil
}

var (
	branchRe = regexp.MustCompile(`^\d+\.\d+$`)
	repoRe   = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
)

func checkSource(src *config.AddonSource) error {
	src.Branch, src.Repo, src.Asset, src.URL = strings.TrimSpace(src.Branch), strings.TrimSpace(src.Repo), strings.TrimSpace(src.Asset), strings.TrimSpace(src.URL)
	switch src.Kind {
	case "none":
	case "sourcemod", "metamod", "amxmodx":
		// "1", "2": the newest of that major version (Metamod 1 is 1.12, 2 is 2.0)
		switch src.Branch {
		case "1", "1.0", "1.x":
			src.Branch = map[string]string{"sourcemod": "1.12", "metamod": "1.12", "amxmodx": "1.10"}[src.Kind]
		case "2", "2.x":
			if src.Kind == "metamod" {
				src.Branch = "2.0"
			}
		}
		if src.Branch != "" && !branchRe.MatchString(src.Branch) {
			return errors.New("the branch is a version like 1.12")
		}
	case "counterstrikesharp":
	case "github":
		src.Repo = strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(src.Repo, "https://github.com/"), "github.com/"), "/")
		if !repoRe.MatchString(src.Repo) {
			return errors.New("the GitHub project is owner/name (roflmuffin/CounterStrikeSharp)")
		}
		if _, err := path.Match(strings.ToLower(src.Asset), ""); err != nil {
			return errors.New("the file pattern is not valid")
		}
	case "link":
		u, err := url.Parse(src.URL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return errors.New("the link starts with https://")
		}
	default:
		return errors.New("unknown kind of source")
	}
	for _, v := range []string{src.Every, src.Watch} {
		if _, ok := everyOf(v, 0); !ok {
			return errors.New("unknown interval: " + v)
		}
	}
	return nil
}

// everyOf reads an interval: "" gives def, "off" 0.
func everyOf(v string, def time.Duration) (time.Duration, bool) {
	switch v {
	case "":
		return def, true
	case "off":
		return 0, true
	case "daily":
		return 24 * time.Hour, true
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < 15*time.Minute {
		return 0, false
	}
	return d, true
}

func (m *Manager) describeSource(name string) string {
	src, _ := m.AddonSourceOf(name)
	if src == nil {
		return "not checked"
	}
	what := sourceName(src)
	every, _ := everyOf(src.Every, 24*time.Hour)
	when := "checked " + everyWords(every)
	if every == 0 {
		when = "not checked by itself"
	}
	act := "a new version is told"
	if src.Auto {
		act = "a new version is installed by itself"
	}
	return what + ", " + when + ", " + act
}

func everyWords(d time.Duration) string {
	switch {
	case d == 24*time.Hour:
		return "daily"
	case d == time.Hour:
		return "every hour"
	case d%time.Hour == 0:
		return fmt.Sprintf("every %d hours", d/time.Hour)
	}
	return fmt.Sprintf("every %d minutes", d/time.Minute)
}

func sourceName(src *config.AddonSource) string {
	switch src.Kind {
	case "sourcemod":
		return "SourceMod " + src.Branch
	case "metamod":
		return "Metamod:Source " + src.Branch
	case "amxmodx":
		return "AMX Mod X " + src.Branch
	case "counterstrikesharp":
		if src.NoRuntime {
			return "CounterStrikeSharp (without the .NET runtime)"
		}
		return "CounterStrikeSharp"
	case "github":
		return "GitHub " + src.Repo
	case "link":
		return src.URL
	}
	return src.Kind
}

func osName() string {
	if runtime.GOOS == "windows" {
		return "windows"
	}
	return "linux"
}

// latestRelease asks a source for its newest version.
func latestRelease(ctx context.Context, src *config.AddonSource) (*Release, error) {
	sys := osName()
	switch src.Kind {
	case "sourcemod":
		return dropRelease(ctx, smDrop+"/"+src.Branch+"/sourcemod-latest-"+sys, "https://www.sourcemod.net/downloads.php")
	case "metamod":
		return dropRelease(ctx, mmsDrop+"/"+src.Branch+"/mmsource-latest-"+sys, "https://www.metamodsource.net/downloads.php")
	case "amxmodx":
		return dropRelease(ctx, amxxDrop+"/"+src.Branch+"/amxmodx-latest-base-"+sys, "https://www.amxmodx.org/downloads-new.php")
	case "counterstrikesharp":
		asset := "counterstrikesharp-with-runtime-{os}-*.zip"
		if src.NoRuntime {
			asset = "counterstrikesharp-{os}-*.zip"
		}
		return githubRelease(ctx, "roflmuffin/CounterStrikeSharp", asset, false)
	case "github":
		return githubRelease(ctx, src.Repo, src.Asset, src.Pre)
	case "link":
		return linkRelease(ctx, src.URL)
	}
	return nil, errors.New("unknown kind of source")
}

func newRequest(ctx context.Context, u string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "SnipeZilla-Server-Manager/"+Version)
	return req, nil
}

var archiveNameRe = regexp.MustCompile(`(?i)^[A-Za-z0-9][A-Za-z0-9._+-]*\.(zip|tar\.gz|tgz)$`)

func isArchiveName(n string) bool { return archiveNameRe.MatchString(n) }

// dropRelease reads a "latest" file of AlliedModders' builds: it names the newest archive, in the same folder.
func dropRelease(ctx context.Context, latest, page string) (*Release, error) {
	req, err := newRequest(ctx, latest)
	if err != nil {
		return nil, err
	}
	resp, err := addonHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("%s: not found (is the branch right?)", latest)
		}
		return nil, fmt.Errorf("%s: %s", latest, resp.Status)
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	name := strings.TrimSpace(string(b))
	if !isArchiveName(name) {
		return nil, fmt.Errorf("%s does not name an archive", latest)
	}
	dir := latest[:strings.LastIndex(latest, "/")+1]
	return &Release{Version: versionInName(name), File: name, URL: dir + name, Page: page}, nil
}

type ghRelease struct {
	Tag        string    `json:"tag_name"`
	Page       string    `json:"html_url"`
	Draft      bool      `json:"draft"`
	Prerelease bool      `json:"prerelease"`
	Published  time.Time `json:"published_at"`
	Assets     []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

// githubRelease reads a GitHub project's newest release, and picks its archive for this system.
func githubRelease(ctx context.Context, repo, asset string, pre bool) (*Release, error) {
	u := githubAPI + "/repos/" + repo + "/releases/latest"
	if pre {
		u = githubAPI + "/repos/" + repo + "/releases?per_page=10"
	}
	req, err := newRequest(ctx, u)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := addonHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, fmt.Errorf("GitHub: %s has no release (or no such project)", repo)
	case (resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests) && resp.Header.Get("X-RateLimit-Remaining") == "0":
		return nil, errors.New("GitHub: too many checks from this address for now (60 an hour); checked again later")
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("GitHub: %s", resp.Status)
	}
	var rel ghRelease
	if pre {
		var list []ghRelease
		if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&list); err != nil {
			return nil, fmt.Errorf("GitHub: %v", err)
		}
		found := false
		for _, r := range list {
			if !r.Draft {
				rel, found = r, true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("GitHub: %s has no release", repo)
		}
	} else if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&rel); err != nil {
		return nil, fmt.Errorf("GitHub: %v", err)
	}
	pattern := strings.ToLower(strings.ReplaceAll(asset, "{os}", osName()))
	var names []string
	pick := -1
	for k, a := range rel.Assets {
		n := strings.ToLower(a.Name)
		if !isArchiveName(a.Name) {
			continue
		}
		names = append(names, a.Name)
		if pick >= 0 {
			continue
		}
		if pattern != "" {
			if ok, _ := path.Match(pattern, n); ok {
				pick = k
			}
		} else if strings.Contains(n, osName()) {
			pick = k
		}
	}
	if pick < 0 && pattern == "" {
		// no file says its system: the only archive, or the first that names no other system
		for k, a := range rel.Assets {
			n := strings.ToLower(a.Name)
			if isArchiveName(a.Name) && !strings.Contains(n, "windows") && !strings.Contains(n, "linux") && !strings.Contains(n, "mac") {
				pick = k
				break
			}
		}
	}
	if pick < 0 {
		has := "none"
		if len(names) > 0 {
			has = strings.Join(names, ", ")
		}
		want := asset
		if want == "" {
			want = "one for " + osName()
		}
		return nil, fmt.Errorf("the release %s has no archive matching %s (it has: %s)", rel.Tag, want, has)
	}
	a := rel.Assets[pick]
	r := &Release{Version: rel.Tag, File: a.Name, URL: a.URL, Page: rel.Page}
	if !rel.Published.IsZero() {
		r.Published = rel.Published.Unix()
	}
	return r, nil
}

var ghTagRe = regexp.MustCompile(`/releases/download/([^/]+)/`)

// linkRelease reads a link: an archive (its version from where it leads, its name, else its date), or a small
// file naming the newest archive, in the same folder (as AlliedModders' "latest" files do).
func linkRelease(ctx context.Context, link string) (*Release, error) {
	var chain []string
	client := *addonHTTP
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("too many redirects")
		}
		chain = append(chain, req.URL.String())
		return nil
	}
	req, err := newRequest(ctx, link)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the link: %s", resp.Status)
	}
	name := ""
	if _, p, err := mime.ParseMediaType(resp.Header.Get("Content-Disposition")); err == nil {
		name = filepath.Base(strings.ReplaceAll(p["filename"], `\`, "/"))
	}
	if !isArchiveName(name) {
		name = path.Base(resp.Request.URL.Path)
	}
	if isArchiveName(name) {
		r := &Release{File: name, URL: link}
		for _, u := range chain {
			if m := ghTagRe.FindStringSubmatch(u); m != nil {
				r.Version = m[1]
				if !strings.Contains(u, "/latest/") {
					r.URL = u // the version's own address
				}
			}
		}
		if r.Version == "" {
			r.Version = versionInName(name)
		}
		if t, err := http.ParseTime(resp.Header.Get("Last-Modified")); err == nil {
			r.Published = t.Unix()
			if r.Version == "" {
				r.Version = t.UTC().Format("2006-01-02 15:04 UTC")
			}
		}
		if r.Version == "" {
			if e := strings.Trim(strings.TrimPrefix(resp.Header.Get("ETag"), "W/"), `"`); e != "" {
				r.Version = "etag " + e
			} else if resp.ContentLength > 0 {
				r.Version = fmt.Sprintf("%d bytes", resp.ContentLength)
			} else {
				return nil, errors.New("the link gives an archive, but nothing tells its versions apart (no version in its name, no date)")
			}
		}
		return r, nil
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	first := strings.TrimSpace(strings.SplitN(string(b), "\n", 2)[0])
	if !isArchiveName(first) {
		return nil, errors.New("the link gives neither an archive (.zip, .tar.gz) nor the name of one")
	}
	base := resp.Request.URL
	ref, _ := url.Parse(first)
	return &Release{Version: versionInName(first), File: first, URL: base.ResolveReference(ref).String()}, nil
}

/* ---------------------------------------------------------------------------------------------------------------
 * Versions
 * ------------------------------------------------------------------------------------------------------------- */

var nameVersionRe = regexp.MustCompile(`\d+(?:\.\d+)+(?:-git\d+)?`)

// versionInName: the version an archive's name has: sourcemod-1.12.0-git7255-windows.zip gives 1.12.0-git7255.
func versionInName(n string) string { return nameVersionRe.FindString(n) }

var verDigitsRe = regexp.MustCompile(`\d+`)

// newerVersion tells whether a is newer than b: their numbers compared in order (v1.0.378, 1.12.0-git7255), else
// whether they differ (dates, tags without numbers).
func newerVersion(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	na, nb := verDigitsRe.FindAllString(a, -1), verDigitsRe.FindAllString(b, -1)
	if len(na) == 0 || len(nb) == 0 || strings.HasPrefix(a, "etag ") || strings.HasSuffix(a, " UTC") || strings.HasSuffix(a, " bytes") {
		return !strings.EqualFold(strings.TrimPrefix(a, "v"), strings.TrimPrefix(b, "v"))
	}
	for k := 0; k < len(na) && k < len(nb); k++ {
		x, _ := strconv.ParseInt(na[k], 10, 64)
		y, _ := strconv.ParseInt(nb[k], 10, 64)
		if x != y {
			return x > y
		}
	}
	return len(na) > len(nb)
}

// hasNew tells whether the version out is newer than the package's (or, that one not known, than the first seen).
func hasNew(st addonPkgState) bool {
	if st.Latest == nil {
		return false
	}
	if st.Installed != "" {
		return newerVersion(st.Latest.Version, st.Installed)
	}
	return st.Seen != "" && newerVersion(st.Latest.Version, st.Seen)
}

// noteArchive: an archive was put into a package by hand: the version it has, when its name says it.
func (m *Manager) noteArchive(name, filename, mode string) {
	if mode == PkgAdd {
		return
	}
	src, _ := m.AddonSourceOf(name)
	m.addons.update(name, func(v *addonPkgState) {
		ver, own := "", false
		switch {
		case v.Latest != nil && strings.EqualFold(filename, v.Latest.File):
			ver, own = v.Latest.Version, true
		case src != nil && isArchiveOf(src, filename):
			ver, own = versionInName(filename), true
		}
		if !own && mode == PkgUpdate {
			return // another archive (a plugin) added into it: the addon's version is the same
		}
		v.Installed, v.InstalledFile, v.InstalledAt, v.Seen, v.Compared = ver, filename, time.Now().Unix(), "", ""
		v.Checked, v.Told = 0, "" // checked again within a minute: a newer version is told, or installed (Install it, a crash watch)
		if ver == "" && v.Latest != nil {
			v.Seen = v.Latest.Version
		}
	})
}

// isArchiveOf tells whether an archive's name is one of the addon's own (sourcemod-1.12.0-git7255-windows.zip for
// SourceMod), not a plugin's put into its package.
func isArchiveOf(src *config.AddonSource, filename string) bool {
	n := strings.ToLower(filename)
	switch src.Kind {
	case "sourcemod":
		return strings.HasPrefix(n, "sourcemod-")
	case "metamod":
		return strings.HasPrefix(n, "mmsource-")
	case "amxmodx":
		return strings.HasPrefix(n, "amxmodx-")
	case "counterstrikesharp":
		return strings.HasPrefix(n, "counterstrikesharp-")
	case "github":
		if src.Asset != "" {
			ok, _ := path.Match(strings.ToLower(strings.ReplaceAll(src.Asset, "{os}", "*")), n)
			return ok
		}
		_, repo, _ := strings.Cut(strings.ToLower(src.Repo), "/")
		return repo != "" && strings.Contains(n, repo)
	}
	return false
}

/* ---------------------------------------------------------------------------------------------------------------
 * Checking
 * ------------------------------------------------------------------------------------------------------------- */

// addonChecker checks the packages when each is due.
func (m *Manager) addonChecker(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(addonFirstCheck):
	}
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		m.addonTick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// packageNames lists the packages (their folders).
func (m *Manager) packageNames() []string {
	entries, _ := os.ReadDir(m.AddonsRoot())
	var out []string
	for _, e := range entries {
		if e.IsDir() && packageName.MatchString(e.Name()) {
			out = append(out, e.Name())
		}
	}
	return out
}

func (m *Manager) addonTick(ctx context.Context) {
	m.resumeRollouts()
	now := time.Now()
	for _, name := range m.packageNames() {
		src, _ := m.AddonSourceOf(name)
		if src == nil {
			continue
		}
		st := m.addons.get(name)
		every, _ := everyOf(src.Every, 24*time.Hour)
		if st.Watch != nil {
			if w, _ := everyOf(src.Watch, time.Hour); w > 0 && (every == 0 || w < every) {
				every = w
			}
			m.endQuietWatch(name, st)
		}
		if every == 0 {
			continue
		}
		if st.Error != "" && every > time.Hour {
			every = time.Hour // a failed check is tried again sooner
		}
		if now.Sub(time.Unix(st.Checked, 0)) >= every && m.addons.busyOf(name) == "" {
			_, _ = m.CheckAddon(ctx, name, "")
		}
	}
}

// CheckAddon asks the source of a package for its newest version, then does what is set: tell of it, or install it
// (a fix after crashes is always installed, when the crash watch is on).
func (m *Manager) CheckAddon(ctx context.Context, name, by string) (AddonUpdateInfo, error) {
	src, _ := m.AddonSourceOf(name)
	if src == nil {
		return m.AddonUpdate(name), errors.New("this package has no source of updates")
	}
	if !m.addons.setBusy(name, "Checking…", true) {
		return m.AddonUpdate(name), ErrAddonBusy
	}
	cctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	rel, err := latestRelease(cctx, src)
	cancel()
	m.addons.setBusy(name, "", false)
	m.addons.update(name, func(v *addonPkgState) {
		v.Checked = time.Now().Unix()
		if err != nil {
			v.Error = err.Error()
			return
		}
		v.Error, v.Latest = "", rel
		if v.Installed == "" && v.Seen == "" {
			v.Seen = rel.Version
		}
		if v.Watch != nil && v.Watch.Base == "" {
			v.Watch.Base = rel.Version
		}
	})
	if err != nil {
		if by != "" {
			m.Log.Add(events.Error, "", name, "Addon package %s: checking %s: %v", name, sourceName(src), err)
		}
		return m.AddonUpdate(name), err
	}
	st := m.addons.get(name)
	// A fix: newer than the version the package has (not known: than the one out when the crashes began)
	fix := st.Watch != nil && ((st.Installed != "" && newerVersion(rel.Version, st.Installed)) || (st.Installed == "" && newerVersion(rel.Version, st.Watch.Base)))
	watchOn := func() bool { w, _ := everyOf(src.Watch, time.Hour); return w > 0 }()
	switch {
	case st.Rollout != nil && st.Rollout.Held == "" && len(st.Rollout.Rest) > 0:
		// on its way already
	case fix && watchOn:
		m.Log.Add(events.Update, "", name, "Addon package %s: %s is out (%s), after the crashes (%s): installing it", name, rel.Version, sourceName(src), st.Watch.Why)
		m.installOrSay(name, "the crash watch", true)
	case hasNew(st) && src.Auto:
		m.Log.Add(events.Update, "", name, "Addon package %s: %s is out (%s; it has %s): installing it", name, rel.Version, sourceName(src), orUnknown(st.Installed))
		m.installOrSay(name, "", false)
	case hasNew(st) && st.Told != rel.Version:
		m.Log.Add(events.Update, "", name, "Addon package %s: %s is out (%s; it has %s). The Addons page installs it.", name, rel.Version, sourceName(src), orUnknown(st.Installed))
		m.addons.update(name, func(v *addonPkgState) { v.Told = rel.Version })
	case st.Installed == "" && st.Compared != rel.Version:
		// The version it has is not known (filled from a server): is it the one out? Its files tell
		if m.addons.setBusy(name, "Comparing its files with "+rel.Version+"…", true) {
			go func() {
				m.identifyAddon(name, *rel)
				m.addons.setBusy(name, "", false)
			}()
		}
	}
	return m.AddonUpdate(name), nil
}

// identifyAddon downloads a version and compares its programs (.dll, .so, .smx, .amxx…) with the package's: all the
// same (and at least one), the package has that version.
func (m *Manager) identifyAddon(name string, rel Release) {
	defer m.addons.update(name, func(v *addonPkgState) { v.Compared = rel.Version })
	dir, err := m.PackagePath(name)
	if err != nil {
		return
	}
	tmp, err := os.MkdirTemp("", "szm-addon-")
	if err != nil {
		return
	}
	defer os.RemoveAll(tmp)
	file := filepath.Join(tmp, rel.File)
	if err := m.download(name, rel.URL, file); err != nil {
		return
	}
	m.addons.setBusy(name, "Comparing its files with "+rel.Version+"…", false)
	same, differ, err := comparePrograms(dir, file, rel.File)
	if err != nil || same == 0 || differ > 0 {
		return
	}
	m.addons.update(name, func(v *addonPkgState) {
		if v.Installed == "" {
			v.Installed, v.InstalledFile, v.Seen = rel.Version, rel.File, ""
		}
	})
	m.Log.Add(events.Manager, "", "", "Addon package %s: has %s (its %d program files are those of %s)", name, rel.Version, same, rel.File)
}

var programExt = map[string]bool{".dll": true, ".so": true, ".smx": true, ".amxx": true, ".exe": true}

// comparePrograms compares the program files of an archive with a package's: how many are the same, how many differ
// (those the package does not have are left out).
func comparePrograms(dir, archive, filename string) (same, differ int, err error) {
	names, open, closeAll, err := openArchive(archive, filename)
	if err != nil {
		return 0, 0, err
	}
	defer closeAll()
	prefix := archivePrefix(names)
	for _, entry := range names {
		rel := strings.TrimPrefix(entry, prefix)
		if !programExt[strings.ToLower(path.Ext(rel))] {
			continue
		}
		target, err := insideDir(dir, rel)
		if err != nil {
			continue
		}
		f, err := os.Open(target)
		if err != nil {
			continue // the package does not have it
		}
		rc, _, err := open(entry)
		if err != nil {
			f.Close()
			return same, differ, err
		}
		eq, err := sameContent(f, rc)
		f.Close()
		rc.Close()
		if err != nil {
			return same, differ, err
		}
		if eq {
			same++
		} else {
			differ++
		}
	}
	return same, differ, nil
}

func sameContent(a, b io.Reader) (bool, error) {
	ba, bb := make([]byte, 64<<10), make([]byte, 64<<10)
	for {
		na, ea := io.ReadFull(a, ba)
		nb, eb := io.ReadFull(b, bb)
		if na != nb || !bytes.Equal(ba[:na], bb[:nb]) {
			return false, nil
		}
		aEnd := ea == io.EOF || ea == io.ErrUnexpectedEOF
		bEnd := eb == io.EOF || eb == io.ErrUnexpectedEOF
		if ea != nil && !aEnd {
			return false, ea
		}
		if eb != nil && !bEnd {
			return false, eb
		}
		if aEnd || bEnd {
			return aEnd == bEnd, nil
		}
	}
}

// installOrSay installs the version out, saying why when it can not.
func (m *Manager) installOrSay(name, by string, fix bool) {
	err := m.InstallAddon(name, by, fix)
	switch {
	case errors.Is(err, ErrAddonBusy):
		m.addons.update(name, func(v *addonPkgState) { v.Checked = 0 }) // something else first: checked again in a minute
	case err != nil:
		m.Log.Add(events.Error, "", name, "Addon package %s: could not install the new version: %v", name, err)
	}
}

func orUnknown(v string) string {
	if v == "" {
		return "a version not known"
	}
	return v
}

// MarkAddonInstalled: the package has the version out (said by hand, for a package filled another way).
func (m *Manager) MarkAddonInstalled(name, by string) error {
	st := m.addons.get(name)
	if st.Latest == nil {
		return errors.New("check it first")
	}
	m.addons.update(name, func(v *addonPkgState) {
		v.Installed, v.InstalledFile, v.Seen = st.Latest.Version, st.Latest.File, ""
	})
	m.Log.Add(events.Manager, "", "", "Addon package %s: has %s, said %s", name, st.Latest.Version, by)
	return nil
}

// AddonUpdateInfo is what the Addons page shows of a package's updates.
type AddonUpdateInfo struct {
	Source        *config.AddonSource `json:"source,omitempty"` // nil: not checked
	Set           bool                `json:"set"`              // set by hand (else the default its name gives)
	Installed     string              `json:"installed,omitempty"`
	InstalledFile string              `json:"installed_file,omitempty"`
	InstalledAt   int64               `json:"installed_at,omitempty"`
	Checked       int64               `json:"checked,omitempty"`
	Next          int64               `json:"next,omitempty"` // the next check
	Error         string              `json:"error,omitempty"`
	Latest        *Release            `json:"latest,omitempty"`
	New           bool                `json:"new"`
	Current       bool                `json:"current"` // the package has the version out (v1.0.378 and 1.0.378 are the same)
	Busy          string              `json:"busy,omitempty"`
	Watch         *CrashWatch         `json:"watch,omitempty"`
	Rollout       *Rollout            `json:"rollout,omitempty"`
	// DefaultBranch: the branch each kind of source has for this package when none is set (Metamod: 2.0 for CS2,
	// 1.12 for the Source games its servers play)
	DefaultBranch map[string]string `json:"default_branch"`
}

// AddonUpdate gives a package's updates.
func (m *Manager) AddonUpdate(name string) AddonUpdateInfo {
	src, set := m.AddonSourceOf(name)
	st := m.addons.get(name)
	u := AddonUpdateInfo{Source: src, Set: set, Installed: st.Installed, InstalledFile: st.InstalledFile, InstalledAt: st.InstalledAt,
		Checked: st.Checked, Error: st.Error, Latest: st.Latest, New: hasNew(st), Busy: m.addons.busyOf(name), Watch: st.Watch, Rollout: st.Rollout}
	u.DefaultBranch = map[string]string{}
	for _, k := range []string{"sourcemod", "metamod", "amxmodx"} {
		u.DefaultBranch[k] = m.defaultBranch(name, k)
	}
	if l := st.Latest; l != nil && st.Installed != "" {
		u.Current = !newerVersion(l.Version, st.Installed) && !newerVersion(st.Installed, l.Version)
	}
	if src != nil {
		every, _ := everyOf(src.Every, 24*time.Hour)
		if st.Watch != nil {
			if w, _ := everyOf(src.Watch, time.Hour); w > 0 && (every == 0 || w < every) {
				every = w
			}
		}
		if every > 0 {
			next := time.Unix(st.Checked, 0).Add(every)
			if st.Checked == 0 || next.Before(m.Started.Add(addonFirstCheck)) {
				next = m.Started.Add(addonFirstCheck)
			}
			u.Next = next.Unix()
		}
	}
	return u
}

/* ---------------------------------------------------------------------------------------------------------------
 * Installing: the archive into the package, then the package into its servers
 * ------------------------------------------------------------------------------------------------------------- */

// InstallAddon downloads the version out into the package (its files over the package's, the files a push never
// overwrites kept), then pushes it to the servers that get it, in the background. fix: after crashes (the servers
// the manager gave up on are started again).
func (m *Manager) InstallAddon(name, by string, fix bool) error {
	return m.installNow(name, by, fix, "", nil)
}

// installNow is InstallAddon; first: the server that gets it first (it crashed), done: told how it ended, after
// the push into the servers (an error: not installed).
func (m *Manager) installNow(name, by string, fix bool, first string, done func(error)) error {
	dir, err := m.PackagePath(name)
	if err != nil {
		return err
	}
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("the addon package %s does not exist", name)
	}
	st := m.addons.get(name)
	if st.Latest == nil {
		return errors.New("check it first")
	}
	if !m.addons.setBusy(name, "Downloading "+st.Latest.File+"…", true) {
		return ErrAddonBusy
	}
	rel := *st.Latest
	go func() {
		err := m.installAddon(name, rel, by, fix, first)
		m.addons.setBusy(name, "", false)
		if err != nil {
			m.Log.Add(events.Error, "", name, "Addon package %s: installing %s: %v", name, rel.Version, err)
		}
		if done != nil {
			done(err)
		}
	}()
	return nil
}

func (m *Manager) installAddon(name string, rel Release, by string, fix bool, first string) error {
	tmp, err := os.MkdirTemp("", "szm-addon-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	file := filepath.Join(tmp, rel.File)
	if err := m.download(name, rel.URL, file); err != nil {
		return err
	}
	m.addons.setBusy(name, "Putting "+rel.File+" into the package…", false)
	m.Cfg.RLock()
	keep := ExcludeMatcher(m.Cfg.AddonsKeep)
	m.Cfg.RUnlock()
	n, kept, err := m.extractPackage(name, file, rel.File, PkgUpdate, keep)
	if err != nil {
		return err
	}
	m.addons.update(name, func(v *addonPkgState) {
		v.Installed, v.InstalledFile, v.InstalledAt, v.Seen, v.Told = rel.Version, rel.File, time.Now().Unix(), "", rel.Version
	})
	who := ""
	if by != "" {
		who = ", by " + by
	}
	m.Log.Add(events.Manager, "", "", "Addon package %s: %s (%s) put in: %d files written, %d kept as the package had them (the files a push never overwrites)%s", name, rel.Version, rel.File, n, kept, who)
	m.startRollout(name, rel.Version, by, fix, first)
	return nil
}

// download fetches a file, saying how far it is.
func (m *Manager) download(name, u, file string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	req, err := newRequest(ctx, u)
	if err != nil {
		return err
	}
	client := *addonHTTP
	client.Timeout = 0 // the context's
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download: %s", resp.Status)
	}
	f, err := os.Create(file)
	if err != nil {
		return err
	}
	var got atomic.Int64
	cw := countWriter{w: f, tell: func(n int64) { got.Add(n) }}
	stop := make(chan struct{})
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				what := "Downloading " + filepath.Base(file) + ": " + fmtBytes(got.Load())
				if resp.ContentLength > 0 {
					what += fmt.Sprintf(" (%d%%)", got.Load()*100/resp.ContentLength)
				}
				m.addons.setBusy(name, what, false)
			}
		}
	}()
	_, err = io.Copy(cw, io.LimitReader(resp.Body, 2<<30))
	close(stop)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && resp.ContentLength > 0 && got.Load() != resp.ContentLength {
		err = fmt.Errorf("download: %s of %s arrived", fmtBytes(got.Load()), fmtBytes(resp.ContentLength))
	}
	return err
}

// serversOf: the servers of this machine that get a package.
func (m *Manager) serversOf(name string) []config.Server {
	var out []config.Server
	for _, s := range m.servers() {
		for _, p := range s.Packages {
			if strings.EqualFold(p, name) {
				out = append(out, s)
				break
			}
		}
	}
	return out
}

// startRollout pushes a new version of a package into its servers: one first, watched, then the others one at a
// time. Without a server, or all at once, they all get it in turn. It returns when it is done (the caller holds the
// package busy).
func (m *Manager) startRollout(name, version, by string, fix bool, first string) {
	list := m.serversOf(name)
	if len(list) == 0 {
		m.addons.update(name, func(v *addonPkgState) { v.Rollout = nil })
		m.Log.Add(events.Updated, "", name, "Addon package %s: %s is in the package (no server gets it)", name, version)
		m.watchDone(name, version)
		return
	}
	src, _ := m.AddonSourceOf(name)
	test := len(list) > 1 && (src == nil || !src.AllAtOnce)
	// The servers the manager gave up on are started again with it
	var revive []string
	for _, s := range list {
		if i := m.instance(s.ID); i != nil {
			i.mu.Lock()
			if i.failed && i.startMapGone == "" && i.cantStart == "" {
				revive = append(revive, s.ID)
			}
			i.mu.Unlock()
		}
	}
	// First: a server the fix is for (one that crashed), else a running one with the fewest players
	var crashed []string
	if w := m.addons.get(name).Watch; fix && w != nil {
		crashed = w.Servers
	}
	order := make([]config.Server, len(list))
	copy(order, list)
	rank := func(s config.Server) int {
		if s.ID == first {
			return -2
		}
		if contains(revive, s.ID) || contains(crashed, s.ID) {
			return -1
		}
		v, ok := m.View(s.ID)
		if !ok || v.PID == 0 {
			return 1000
		}
		return humansOf(v)
	}
	sort.SliceStable(order, func(a, b int) bool { return rank(order[a]) < rank(order[b]) })
	r := &Rollout{Version: version, Started: time.Now().Unix(), Revive: revive, Fix: fix, By: by}
	for _, s := range order {
		r.Rest = append(r.Rest, s.ID)
	}
	if test {
		v, ok := m.View(order[0].ID)
		if order[0].ID == first || contains(revive, order[0].ID) || contains(crashed, order[0].ID) || (ok && (v.PID > 0 || v.Desired)) {
			r.First = order[0].ID
		} else {
			test = false // none runs: nothing to watch
		}
	}
	m.addons.update(name, func(v *addonPkgState) { v.Rollout = r })
	m.runRollout(name, test)
}

func contains(list []string, x string) bool {
	for _, v := range list {
		if v == x {
			return true
		}
	}
	return false
}

func humansOf(v View) int {
	n := 0
	for _, p := range v.Players {
		if !p.Bot {
			n++
		}
	}
	return n
}

// resumeRollouts: a rollout that stopped (the manager restarted; or it waited for the package to be free) goes on,
// without the watch of the first server.
func (m *Manager) resumeRollouts() {
	for _, name := range m.packageNames() {
		st := m.addons.get(name)
		if r := st.Rollout; r != nil && r.Held == "" && len(r.Rest) > 0 {
			if !m.addons.setBusy(name, "Pushing…", true) {
				continue
			}
			m.Log.Add(events.Info, "", name, "Addon package %s: %s goes on into %d servers", name, r.Version, len(r.Rest))
			go func(name string) {
				m.runRollout(name, false)
				m.addons.setBusy(name, "", false)
			}(name)
		}
	}
}

// runRollout pushes the rollout's version into the servers still to get it (the package held busy by the caller).
func (m *Manager) runRollout(name string, test bool) {
	st := m.addons.get(name)
	r := st.Rollout
	if r == nil {
		return
	}
	nameOf := func(id string) string {
		if v, ok := m.View(id); ok {
			return v.Name
		}
		return id
	}
	total := len(r.Rest) + len(r.Done) + len(r.Failed)
	for len(r.Rest) > 0 {
		id := r.Rest[0]
		first := test && id == r.First
		var crashes chan events.Event
		if first {
			crashes = m.Log.Subscribe() // from before its start: a crash at once is seen
		}
		m.addons.setBusy(name, fmt.Sprintf("Pushing %s into %s (%d of %d)…", r.Version, nameOf(id), len(r.Done)+len(r.Failed)+1, total), false)
		err := m.pushWait(name, id, contains(r.Revive, id), r.Fix)
		if err == nil && first {
			until := time.Now().Add(addonTestFor)
			m.addons.update(name, func(v *addonPkgState) {
				if v.Rollout != nil {
					v.Rollout.Until = until.Unix()
				}
			})
			m.addons.setBusy(name, fmt.Sprintf("Trying %s on %s for %d minutes, then the others…", r.Version, nameOf(id), int(addonTestFor/time.Minute)), false)
			if why := m.watchServer(crashes, id, addonTestFor); why != "" {
				err = errors.New(why)
			}
		}
		if crashes != nil {
			m.Log.Unsubscribe(crashes)
		}
		if err != nil && !first {
			// another server: it keeps its version, the others get the new one
			m.Log.Add(events.Error, id, nameOf(id), "Addon package %s: %s could not go into it: %v", name, r.Version, err)
			r.Failed, r.Rest = append(r.Failed, id), r.Rest[1:]
			m.addons.update(name, func(v *addonPkgState) {
				if v.Rollout != nil {
					v.Rollout.Failed, v.Rollout.Rest = append([]string(nil), r.Failed...), append([]string(nil), r.Rest...)
				}
			})
			continue
		}
		if err != nil {
			held := fmt.Sprintf("%s: %v", nameOf(id), err)
			m.addons.update(name, func(v *addonPkgState) {
				if v.Rollout != nil {
					v.Rollout.Held = held
					v.Rollout.Rest = v.Rollout.Rest[1:]
				}
				if v.Watch != nil {
					v.Watch.Base = r.Version // not the fix: the next version is waited for
				}
			})
			rest := len(r.Rest) - 1
			switch {
			case rest == 1:
				m.Log.Add(events.Addons, "", name, "Addon package %s: %s held back: %s. %s keeps the version it has; Push on the Addons page gives it the new one.", name, r.Version, held, nameOf(r.Rest[1]))
			case rest > 1:
				m.Log.Add(events.Addons, "", name, "Addon package %s: %s held back: %s. The other %d servers keep the version they have; Push on the Addons page gives them the new one.", name, r.Version, held, rest)
			default:
				m.Log.Add(events.Addons, "", name, "Addon package %s: %s: %s", name, r.Version, held)
			}
			return
		}
		r.Done, r.Rest = append(r.Done, id), r.Rest[1:]
		m.addons.update(name, func(v *addonPkgState) {
			if v.Rollout != nil {
				v.Rollout.Done, v.Rollout.Rest = append([]string(nil), r.Done...), append([]string(nil), r.Rest...)
			}
		})
	}
	names := make([]string, len(r.Done))
	for k, id := range r.Done {
		names[k] = nameOf(id)
	}
	m.addons.update(name, func(v *addonPkgState) { v.Rollout = nil })
	if len(r.Failed) > 0 {
		failed := make([]string, len(r.Failed))
		for k, id := range r.Failed {
			failed[k] = nameOf(id)
		}
		if len(names) > 0 {
			m.Log.Add(events.Updated, "", name, "Addon package %s: %s is on %s; not on %s (see Activity)", name, r.Version, strings.Join(names, ", "), strings.Join(failed, ", "))
			m.watchDone(name, r.Version)
		}
		return
	}
	m.Log.Add(events.Updated, "", name, "Addon package %s: %s is on %s", name, r.Version, strings.Join(names, ", "))
	m.watchDone(name, r.Version)
}

// pushWait pushes a package into a server and waits for it to be done (the server started again). revive: the
// manager had given up on it: it is started again too (with fix, also when it gave up since the rollout started). A
// server busy with something else is waited for.
func (m *Manager) pushWait(name, id string, revive, fix bool) error {
	s, _, i, err := m.lookup(id)
	if err != nil {
		return err
	}
	if fix {
		i.mu.Lock()
		revive = revive || (i.failed && i.startMapGone == "" && i.cantStart == "")
		i.mu.Unlock()
	}
	// The fix of an addon it waits for: with its addons again
	if waits := m.vanillaFor(s); waits != "" && contains(strings.Split(strings.ToLower(waits), ", "), strings.ToLower(name)) {
		m.state.update(id, func(v *savedServer) { v.Vanilla = "" })
		for _, pkg := range strings.Split(waits, ", ") {
			m.addons.update(pkg, func(v *addonPkgState) {
				if v.Watch != nil {
					v.Watch.Vanilla = without(v.Watch.Vanilla, id)
				}
			})
		}
		revive = true
	}
	if revive {
		i.mu.Lock()
		i.failed, i.crashes, i.nextStart, i.repairWanted = false, 0, time.Time{}, false
		i.mu.Unlock()
		m.setDesired(i, s, true)
	}
	deadline := time.Now().Add(time.Hour)
	for {
		done := make(chan error, 1)
		err := m.pushAddons(id, name, "", true, func(e error) { done <- e })
		if err == nil {
			return <-done
		}
		if !errors.Is(err, errBusy) || time.Now().After(deadline) {
			return err
		}
		time.Sleep(addonWaitStep)
	}
}

// watchServer watches a server that got a new version: "" when it ran that long without crashing, else what went
// wrong.
func (m *Manager) watchServer(c chan events.Event, id string, d time.Duration) string {
	end := time.After(d)
	check := time.NewTicker(addonWaitStep)
	defer check.Stop()
	for {
		select {
		case e := <-c:
			if e.Server == id && e.Machine == "" && (e.Kind == events.Crash || e.Kind == events.Failed) {
				return "it crashed after it got it"
			}
		case <-check.C:
			if v, ok := m.View(id); ok && v.PID == 0 && !v.Desired {
				return "it was stopped while it was being tried"
			}
		case <-end:
			return ""
		}
	}
}

// addonCrashWait: a server that crashed while a new version of one of its addons is out waits that long for it (the
// push starts it as soon as it is in; not installed, it starts at once)
var addonCrashWait = 10 * time.Minute

// newVersionsFor: the packages of a server with a new version out that is to be installed (Install it, or a crash
// watch on), and not being installed already.
func (m *Manager) newVersionsFor(s config.Server) []string {
	var out []string
	for _, p := range s.Packages {
		src, _ := m.AddonSourceOf(p)
		if src == nil || m.addons.busyOf(p) != "" {
			continue
		}
		st := m.addons.get(p)
		if st.Rollout != nil && st.Rollout.Held == "" && len(st.Rollout.Rest) > 0 {
			continue
		}
		watch, _ := everyOf(src.Watch, time.Hour)
		if hasNew(st) && (src.Auto || (st.Watch != nil && watch > 0)) {
			out = append(out, p)
		}
	}
	return out
}

// installBeforeRestart installs the new versions out of a crashed server's addons, it first; the push starts it.
// Not installed: it starts now.
func (m *Manager) installBeforeRestart(s config.Server, pkgs []string) {
	i := m.instance(s.ID)
	startNow := func(error) {
		i.mu.Lock()
		if i.nextStart.After(time.Now()) {
			i.nextStart = time.Now()
		}
		i.mu.Unlock()
	}
	for _, p := range pkgs {
		st := m.addons.get(p)
		if st.Latest == nil {
			continue
		}
		m.Log.Add(events.Update, "", p, "Addon package %s: %s is out and %s crashed: installing it before it starts again (it has %s)", p, st.Latest.Version, s.Name, orUnknown(st.Installed))
		if err := m.installNow(p, "", true, s.ID, func(err error) {
			if err != nil {
				startNow(err)
			}
		}); err != nil {
			startNow(err)
		}
	}
}

/* ---------------------------------------------------------------------------------------------------------------
 * Crash watch: after crashes an addon caused, its package is checked more often, and the fix installed
 * ------------------------------------------------------------------------------------------------------------- */

// addonCrashed looks at a crash report: an addon of a package to blame (the crash dump sure or likely), or crashes
// in a row after a game update, start watching the packages for a fix. kind: Crash, or Failed (given up).
func (m *Manager) addonCrashed(r *CrashReport, kind string) {
	s, _, i, err := m.lookup(r.Server)
	if err != nil || len(s.Packages) == 0 {
		return
	}
	i.mu.Lock()
	mapGone := i.startMapGone != "" || i.cantStart != ""
	i.mu.Unlock()
	if mapGone {
		return // its start map is missing, or a DLL Windows lacks: not an addon
	}
	var pkgs []string
	why := ""
	if p := blamedPath(r); p != "" {
		if pkg := m.packageHolding(s.Packages, p); pkg != "" {
			pkgs = []string{pkg}
			why = s.Name + " crashed in " + r.Suspect
		}
	}
	if len(pkgs) == 0 && (kind == events.Failed || r.InARow >= 2) && gameUpdatedBefore(r, 48*time.Hour) {
		pkgs = append(pkgs, s.Packages...)
		why = s.Name + " crashed " + strconv.Itoa(r.InARow) + " times in a row since its game was updated"
	}
	var waiting []string // the packages watched for a fix
	for _, pkg := range pkgs {
		if at := m.addons.get(pkg).InstalledAt; at > 0 && r.Time.Before(time.Unix(at, 0)) {
			continue // a crash of the version it had before (the report is made a moment after)
		}
		src, _ := m.AddonSourceOf(pkg)
		if src == nil {
			st := m.addons.get(pkg)
			if r.Suspect != "" && time.Since(time.Unix(st.NoSource, 0)) > 24*time.Hour {
				m.addons.update(pkg, func(v *addonPkgState) { v.NoSource = time.Now().Unix() })
				m.Log.Add(events.Addons, "", pkg, "Addon package %s: %s, but the package has no source of updates: set one on the Addons page, and a fix is installed when it comes out.", pkg, why)
			}
			continue
		}
		every, _ := everyOf(src.Watch, time.Hour)
		if every == 0 {
			continue
		}
		st := m.addons.get(pkg)
		now := time.Now().Unix()
		fresh := st.Watch == nil
		m.addons.update(pkg, func(v *addonPkgState) {
			if v.Watch == nil {
				v.Watch = &CrashWatch{Since: now, Why: why}
				if v.Latest != nil && v.Checked > 0 && time.Since(time.Unix(v.Checked, 0)) < 24*time.Hour {
					v.Watch.Base = v.Latest.Version
				}
			}
			v.Watch.Last = now
			if !contains(v.Watch.Servers, s.ID) {
				v.Watch.Servers = append(v.Watch.Servers, s.ID)
			}
		})
		if !fresh && time.Since(time.Unix(st.Checked, 0)) > 10*time.Minute && m.addons.busyOf(pkg) == "" {
			go func(pkg string) { _, _ = m.CheckAddon(context.Background(), pkg, "") }(pkg) // a fix may be out since
		}
		if fresh {
			m.Log.Add(events.Addons, "", pkg, "Addon package %s: %s. Checking %s %s for a fix; it is installed when it comes out, and the servers the manager gave up on are started again.", pkg, why, sourceName(src), everyWords(every))
			go func(pkg string) { _, _ = m.CheckAddon(context.Background(), pkg, "") }(pkg)
		}
		waiting = append(waiting, pkg)
	}
	if kind == events.Failed && s.Vanilla {
		m.startVanilla(s.ID, waiting)
	}
}

// blamedPath: the file of the addon the crash dump (or Windows' record) blames, when it is sure or likely.
func blamedPath(r *CrashReport) string {
	if d := r.Dump; d != nil && d.Code != "" {
		if d.Suspect == "" || (d.Sure != "sure" && d.Sure != "likely") {
			return ""
		}
		if d.Unloaded && d.Module == d.Suspect {
			return d.Path
		}
		for _, list := range [][]crashinfo.Frame{d.Chain, d.Frames, d.Addons} {
			for _, f := range list {
				if f.Label == d.Suspect && f.Path != "" {
					return f.Path
				}
			}
		}
		for _, l := range d.Loaded {
			if l.Label == d.Suspect {
				return l.Path
			}
		}
		return ""
	}
	for _, s := range r.System {
		if s.ModPath != "" && s.Module == r.Suspect {
			return s.ModPath
		}
	}
	return ""
}

// packageHolding: which of the packages has the file (by its path from addons/), else the one with its addon's
// folder (addons/sourcemod).
func (m *Manager) packageHolding(pkgs []string, file string) string {
	p := strings.ReplaceAll(file, `\`, "/")
	k := strings.Index(strings.ToLower(p), "/addons/")
	if k < 0 {
		return ""
	}
	rel := p[k+1:] // addons/...
	top := strings.SplitN(rel, "/", 3)
	for _, pkg := range pkgs {
		dir, err := m.PackagePath(pkg)
		if err != nil {
			continue
		}
		if fileExistsFold(dir, rel) {
			return pkg
		}
	}
	if len(top) >= 2 {
		for _, pkg := range pkgs {
			if dir, err := m.PackagePath(pkg); err == nil && fileExistsFold(dir, top[0]+"/"+top[1]) {
				return pkg
			}
		}
	}
	return ""
}

// fileExistsFold: dir/rel exists, the names compared without case (a Windows path in a dump, a Linux package).
func fileExistsFold(dir, rel string) bool {
	cur := dir
	for _, part := range strings.Split(rel, "/") {
		if part == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(cur, part)); err == nil {
			cur = filepath.Join(cur, part)
			continue
		}
		entries, err := os.ReadDir(cur)
		if err != nil {
			return false
		}
		found := false
		for _, e := range entries {
			if strings.EqualFold(e.Name(), part) {
				cur, found = filepath.Join(cur, e.Name()), true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

var gameUpdateRe = regexp.MustCompile(`^(Game updated: |Up to date: |Updated to |Installed )`)

// gameUpdatedBefore: the game was updated in the d before the run that crashed.
func gameUpdatedBefore(r *CrashReport, d time.Duration) bool {
	for _, c := range r.Changes {
		if c.Kind == "event" && gameUpdateRe.MatchString(c.Text) && r.Time.Sub(c.Time) < d {
			return true
		}
	}
	return false
}

// watchDone: a version got into the servers. The crash watch ends with it.
func (m *Manager) watchDone(name, version string) {
	st := m.addons.get(name)
	if st.Watch == nil {
		return
	}
	m.addons.update(name, func(v *addonPkgState) { v.Watch = nil })
	m.Log.Add(events.Addons, "", name, "Addon package %s: %s is in, the crash watch ends (back to the usual checks)", name, version)
}

// endQuietWatch ends a crash watch after a day without those crashes, the servers running (none given up or
// without its addons).
func (m *Manager) endQuietWatch(name string, st addonPkgState) {
	w := st.Watch
	if w == nil || time.Since(time.Unix(w.Last, 0)) < 24*time.Hour || (st.Rollout != nil && st.Rollout.Held == "" && len(st.Rollout.Rest) > 0) {
		return
	}
	for _, s := range m.serversOf(name) {
		if m.vanillaFor(s) != "" {
			return
		}
		if i := m.instance(s.ID); i != nil {
			i.mu.Lock()
			failed := i.failed
			i.mu.Unlock()
			if failed {
				return
			}
		}
	}
	m.addons.update(name, func(v *addonPkgState) { v.Watch = nil })
	m.Log.Add(events.Addons, "", name, "Addon package %s: no crash of it for a day: the crash watch ends", name)
}

// StopWatch ends a package's crash watch by hand.
func (m *Manager) StopWatch(name, by string) {
	m.addons.update(name, func(v *addonPkgState) { v.Watch = nil })
	m.Log.Add(events.Addons, "", name, "Addon package %s: crash watch ended by %s", name, by)
}

/* ---------------------------------------------------------------------------------------------------------------
 * Without its addons while it waits for a fix
 * ------------------------------------------------------------------------------------------------------------- */

// vanillaFor: the package a server waits for a fix of, without its addons ("" none).
func (m *Manager) vanillaFor(s config.Server) string {
	if !s.Vanilla {
		return ""
	}
	return m.state.get(s.ID).Vanilla
}

// CanVanilla tells whether a game's servers can start without their addons: Metamod loaded by gameinfo.gi (CS2),
// or by addons/metamod.vdf (the Source games).
func CanVanilla(g *games.Game) bool { return g != nil && (g.Metamod || g.Engine == games.Source) }

// startVanilla starts a server the manager gave up on without its addons, while it waits for a fix of pkgs. Once:
// given up on again while without them, the addons are not to blame, and it stays stopped.
func (m *Manager) startVanilla(id string, pkgs []string) {
	s, g, i, err := m.lookup(id)
	if err != nil || len(pkgs) == 0 || !CanVanilla(g) || !hasLoader(s, g) || m.state.get(id).Vanilla != "" {
		return
	}
	// still given up on (the report is made a moment after: a fix may have started it again since)
	i.mu.Lock()
	gaveUp := i.failed && i.pid == 0 && i.busy == ""
	i.mu.Unlock()
	if !gaveUp {
		return
	}
	waits := strings.Join(pkgs, ", ")
	m.state.update(id, func(v *savedServer) { v.Vanilla = waits })
	for _, pkg := range pkgs {
		m.addons.update(pkg, func(v *addonPkgState) {
			if v.Watch != nil && !contains(v.Watch.Vanilla, id) {
				v.Watch.Vanilla = append(v.Watch.Vanilla, id)
			}
		})
	}
	m.Log.Add(events.Addons, s.ID, s.Name, "Started without its addons (Metamod off) while it waits for a fix of %s: players can play meanwhile. Its addons come back with the fix.", waits)
	if err := m.Start(id, ""); err != nil {
		m.Log.Add(events.Error, s.ID, s.Name, "Could not start without its addons: %v", err)
	}
}

// hasLoader: the server loads Metamod (what starting without addons turns off).
func hasLoader(s config.Server, g *games.Game) bool {
	gameDir := g.Path(s.InstallDir, g.GameDir)
	if g.Metamod {
		b, _ := os.ReadFile(filepath.Join(gameDir, "gameinfo.gi"))
		st, err := os.Stat(filepath.Join(gameDir, "addons", "metamod"))
		return bytes.Contains(bytes.ToLower(b), []byte(strings.ToLower(g.Game+"/addons/metamod"))) || (err == nil && st.IsDir())
	}
	vdfs, _ := filepath.Glob(filepath.Join(gameDir, "addons", "metamod*.vdf"))
	return len(vdfs) > 0
}

const vanillaOff = ".szm-off" // metamod.vdf renamed metamod.vdf.szm-off: not loaded

// applyVanilla, before a start: Metamod off while the server waits for a fix, back on otherwise.
func (m *Manager) applyVanilla(s config.Server, g *games.Game) {
	if g == nil {
		return
	}
	gameDir := g.Path(s.InstallDir, g.GameDir)
	off := m.vanillaFor(s) != ""
	if !off && m.state.get(s.ID).Vanilla != "" {
		m.state.update(s.ID, func(v *savedServer) { v.Vanilla = "" }) // the setting was turned off
	}
	if g.Metamod {
		if off {
			if changed, err := UnpatchGameinfo(filepath.Join(gameDir, "gameinfo.gi"), g.Game+"/addons/metamod"); err != nil {
				m.Log.Add(events.Error, s.ID, s.Name, "Metamod off: %v", err)
			} else if changed {
				m.Log.Add(events.Info, s.ID, s.Name, "Metamod taken out of gameinfo.gi: it starts without its addons")
			}
		}
		return // on: patchMetamod puts it back
	}
	vdfs, _ := filepath.Glob(filepath.Join(gameDir, "addons", "metamod*.vdf"))
	offs, _ := filepath.Glob(filepath.Join(gameDir, "addons", "metamod*.vdf"+vanillaOff))
	if off {
		for _, v := range vdfs {
			_ = os.Remove(v + vanillaOff)
			if err := os.Rename(v, v+vanillaOff); err != nil {
				m.Log.Add(events.Error, s.ID, s.Name, "Metamod off: %v", err)
			}
		}
		return
	}
	for _, o := range offs {
		v := strings.TrimSuffix(o, vanillaOff)
		if _, err := os.Stat(v); err == nil {
			_ = os.Remove(o) // a push brought it back
		} else if err := os.Rename(o, v); err != nil {
			m.Log.Add(events.Error, s.ID, s.Name, "Metamod back on: %v", err)
		}
	}
}

// UnpatchGameinfo takes a search path ("csgo/addons/metamod") out of a gameinfo.gi.
func UnpatchGameinfo(p, entry string) (bool, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	nl := "\n"
	if strings.Contains(string(b), "\r\n") {
		nl = "\r\n"
	}
	lines := strings.Split(string(b), nl)
	out := lines[:0:0]
	changed := false
	for _, l := range lines {
		f := strings.Fields(l)
		if len(f) == 2 && strings.EqualFold(f[0], "Game") && strings.EqualFold(f[1], entry) {
			changed = true
			continue
		}
		out = append(out, l)
	}
	if !changed {
		return false, nil
	}
	return true, config.WriteFileAtomic(p, []byte(strings.Join(out, nl)), 0o644)
}
