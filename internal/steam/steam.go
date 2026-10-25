// Package steam installs and updates game servers with steamcmd, reads their version (steam.inf) and asks Steam
// whether a version is the latest.
package steam

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SnipeZilla/szm/internal/proc"
)

var httpClient = &http.Client{Timeout: 20 * time.Second}

// ReadVersion reads the version of an installed server from its steam.inf (PatchVersion, else ServerVersion):
// the text ("1.40.2.6") and the number Steam compares (1402 6 → 14026). "" when there is none.
func ReadVersion(path string) (text string, number int64) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", 0
	}
	var server string
	for _, line := range strings.Split(string(b), "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "patchversion":
			text = strings.TrimSpace(v)
		case "serverversion":
			server = strings.TrimSpace(v)
		}
	}
	if text == "" {
		text = server
	}
	digits := regexp.MustCompile(`\D`).ReplaceAllString(text, "")
	number, _ = strconv.ParseInt(digits, 10, 64)
	return text, number
}

// ReadBuild reads the build of an app steamcmd installed in a folder (steamapps/appmanifest_<appid>.acf): the games
// without a steam.inf (other engines) are versioned this way. "build 12345678", and the number.
func ReadBuild(dir string, appid int) (text string, number int64) {
	b, err := os.ReadFile(filepath.Join(dir, "steamapps", fmt.Sprintf("appmanifest_%d.acf", appid)))
	if err != nil {
		return "", 0
	}
	if m := buildidRe.FindSubmatch(b); m != nil {
		number, _ = strconv.ParseInt(string(m[1]), 10, 64)
		return "build " + string(m[1]), number
	}
	return "", 0
}

var (
	buildidRe     = regexp.MustCompile(`"buildid"\s+"(\d+)"`)
	publicBuildRe = regexp.MustCompile(`(?s)"public"\s*\{[^{}]*?"buildid"\s+"(\d+)"`)
)

// ErrBusy: steamcmd is downloading; asked again later.
var ErrBusy = errors.New("steamcmd is busy")

// LatestBuild asks steamcmd for the newest public build of an app (app_info_print): how the games of other engines
// are checked for updates. ErrBusy when steamcmd is at work.
func (r *Runner) LatestBuild(ctx context.Context, appid int) (int64, error) {
	if !r.mu.TryLock() {
		return 0, ErrBusy
	}
	defer r.mu.Unlock()
	if r.Path == "" {
		return 0, errors.New("steamcmd is not set up")
	}
	if _, err := os.Stat(r.Path); err != nil {
		return 0, fmt.Errorf("steamcmd not found: %s", r.Path)
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, r.Path, "+login", "anonymous", "+app_info_update", "1", "+app_info_print", strconv.Itoa(appid), "+quit")
	cmd.Dir = filepath.Dir(r.Path)
	proc.RunHiddenCmd(cmd)
	cmd.Stdin = strings.NewReader("")
	out, err := cmd.CombinedOutput()
	return parseLatestBuild(out, err)
}

// parseLatestBuild finds the public branch's build in app_info_print's output.
func parseLatestBuild(out []byte, err error) (int64, error) {
	k := bytes.Index(out, []byte(`"branches"`))
	if k >= 0 {
		if m := publicBuildRe.FindSubmatch(out[k:]); m != nil {
			n, _ := strconv.ParseInt(string(m[1]), 10, 64)
			return n, nil
		}
	}
	if err != nil {
		return 0, fmt.Errorf("steamcmd: %v", err)
	}
	return 0, errors.New("steamcmd gave no build for it")
}

// apiBase is Steam's Web API (SZM_STEAM_API replaces it, for tests).
func apiBase() string {
	if v := os.Getenv("SZM_STEAM_API"); v != "" {
		return v
	}
	return "https://api.steampowered.com"
}

// UpToDate asks Steam whether a version of a game is the latest: up to date, and the version required.
func UpToDate(appid int, version int64) (bool, int64, error) {
	url := fmt.Sprintf("%s/ISteamApps/UpToDateCheck/v1/?appid=%d&version=%d&format=json", apiBase(), appid, version)
	resp, err := httpClient.Get(url)
	if err != nil {
		return true, 0, err
	}
	defer resp.Body.Close()
	var data struct {
		Response struct {
			Success         bool   `json:"success"`
			UpToDate        bool   `json:"up_to_date"`
			RequiredVersion int64  `json:"required_version"`
			Error           string `json:"error"`
		} `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return true, 0, err
	}
	if !data.Response.Success {
		return true, 0, fmt.Errorf("Steam: %s", data.Response.Error)
	}
	return data.Response.UpToDate, data.Response.RequiredVersion, nil
}

/* ---------------------------------------------------------------------------------------------------------------
 * steamcmd
 * ------------------------------------------------------------------------------------------------------------- */

// Job is an install or an update of a server by steamcmd.
type Job struct {
	Dir          string // force_install_dir
	AppID        int    // the dedicated server's app ID
	AppSetConfig string // GoldSrc mods: "90 mod cstrike"
	Login        string // anonymous, or a Steam account ("user", or "user password")
	Validate     bool
	Label        string // shown in the log
	// For a Steam account, given at the time, never saved: its password, and the Steam Guard code Steam asked for
	Password  string
	GuardCode string // the code Steam sent by email
	TwoFactor string // the code of the Steam app on the phone
}

// LoginError says what Steam wants before it lets this account download: Need is "password" (none given),
// "bad_password", "guard" (a code sent by email) or "2fa" (a code of the Steam app on the phone).
type LoginError struct {
	Need    string
	Account string
}

func (e *LoginError) Error() string {
	switch e.Need {
	case "guard":
		return fmt.Sprintf("Steam sent a Steam Guard code to the email address of %s: give it on the server's page to go on", e.Account)
	case "2fa":
		return fmt.Sprintf("Steam wants the code of the Steam app (Steam Guard) of %s: give it on the server's page to go on", e.Account)
	case "approve":
		return fmt.Sprintf("The login of %s was not approved in the Steam app in time: try again from the server's page, and approve it within a minute", e.Account)
	case "bad_password":
		return fmt.Sprintf("Steam says the password of %s is wrong: give it again on the server's page", e.Account)
	}
	return fmt.Sprintf("Steam wants the password of %s: give it on the server's page to go on", e.Account)
}

// Progress is what steamcmd says while it works.
type Progress struct {
	Label   string  `json:"label"`
	Percent float64 `json:"percent"`
	State   string  `json:"state"` // downloading, verifying, committing...
	Line    string  `json:"line"`
	Done    int64   `json:"done,omitempty"`    // bytes of this step done...
	Total   int64   `json:"total,omitempty"`   // ...out of
	Speed   float64 `json:"speed,omitempty"`   // bytes a second (downloading), averaged
	Left    int64   `json:"left,omitempty"`    // seconds left, at that speed
	Started int64   `json:"started,omitempty"` // unix time the job started
	Prompt  string  `json:"prompt,omitempty"`  // "approve": steamcmd waits for the login to be approved in the Steam app
}

// Runner runs steamcmd, one job at a time (two at once corrupt its cache).
type Runner struct {
	Path string
	mu   sync.Mutex
	cur  *Progress
	cmu  sync.Mutex
}

// "Update state (0x61) downloading, progress: 78.12 (27891234567 / 35700000000)"
var progressRe = regexp.MustCompile(`Update state \(0x[0-9a-f]+\) ([a-z ,]+?), progress: ([0-9.]+)(?: \((\d+) / (\d+)\))?`)

// meter averages a download's speed over the last samples.
type meter struct {
	state      string
	lastBytes  int64
	lastAt     time.Time
	speed      float64
	lastLogged int // the last 10 % step written to the console
}

func (m *meter) add(state string, done int64, now time.Time) float64 {
	if state != m.state || done < m.lastBytes {
		m.state, m.lastBytes, m.lastAt, m.speed = state, done, now, 0
		return 0
	}
	dt := now.Sub(m.lastAt).Seconds()
	if dt < 1 {
		return m.speed
	}
	inst := float64(done-m.lastBytes) / dt
	if m.speed == 0 {
		m.speed = inst
	} else {
		m.speed = 0.7*m.speed + 0.3*inst
	}
	m.lastBytes, m.lastAt = done, now
	return m.speed
}

// HumanBytes writes a size as the web page does (1024 based): 33.2 GB.
func HumanBytes(n int64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	v, i := float64(n), 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	if i == 0 || v >= 100 {
		return fmt.Sprintf("%.0f %s", v, units[i])
	}
	return fmt.Sprintf("%.1f %s", v, units[i])
}

// Current gives the job steamcmd is doing, nil when it is idle.
func (r *Runner) Current() *Progress {
	r.cmu.Lock()
	defer r.cmu.Unlock()
	if r.cur == nil {
		return nil
	}
	p := *r.cur
	return &p
}

func (r *Runner) set(p *Progress) {
	r.cmu.Lock()
	r.cur = p
	r.cmu.Unlock()
}

// Run runs a job and waits for it. line gets each line steamcmd writes. GoldSrc (app 90) often needs several runs to
// install: it is run again, up to tries times, until steamcmd says the app is fully installed.
func (r *Runner) Run(ctx context.Context, job Job, tries int, line func(string)) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	defer r.set(nil)
	if r.Path == "" {
		return errors.New("steamcmd is not set up")
	}
	if _, err := os.Stat(r.Path); err != nil {
		return fmt.Errorf("steamcmd not found: %s", r.Path)
	}
	if err := os.MkdirAll(job.Dir, 0o755); err != nil {
		return err
	}
	if tries < 1 {
		tries = 1
	}
	login := strings.Fields(job.Login)
	if len(login) == 0 {
		login = []string{"anonymous"}
	}
	if !strings.EqualFold(login[0], "anonymous") && job.Password != "" {
		login = []string{login[0], job.Password}
	}
	if len(login) == 2 && job.TwoFactor != "" {
		login = append(login, job.TwoFactor) // +login user password code
	}
	var last error
	for try := 1; try <= tries; try++ {
		args := []string{"+force_install_dir", job.Dir}
		if job.GuardCode != "" {
			args = append(args, "+set_steam_guard_code", job.GuardCode)
		}
		args = append(args, "+login")
		args = append(args, login...)
		if job.AppSetConfig != "" {
			args = append(args, "+app_set_config")
			args = append(args, strings.Fields(job.AppSetConfig)...)
		}
		args = append(args, "+app_update", strconv.Itoa(job.AppID))
		if job.Validate || (try > 1 && job.AppID == 90) {
			args = append(args, "validate")
		}
		args = append(args, "+quit")

		p := &Progress{Label: job.Label, State: "starting", Started: time.Now().Unix()}
		r.set(p)
		ok, err := r.once(ctx, args, p, line)
		if ok {
			return nil
		}
		var le *LoginError
		if errors.As(err, &le) {
			le.Account = login[0]
			return le // trying again would only ask again
		}
		last = err
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if try < tries {
			line(fmt.Sprintf("steamcmd: try %d of %d did not finish the install, running it again", try, tries))
			time.Sleep(3 * time.Second)
		}
	}
	if last == nil {
		last = errors.New("steamcmd did not say the app is fully installed")
	}
	return last
}

func (r *Runner) once(ctx context.Context, args []string, p *Progress, line func(string)) (bool, error) {
	cmd := exec.CommandContext(ctx, r.Path, args...)
	cmd.Dir = filepath.Dir(r.Path)
	proc.RunHiddenCmd(cmd)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return false, err
	}
	cmd.Stderr = cmd.Stdout
	cmd.Stdin = strings.NewReader("") // a question (a Steam Guard code) gets an empty answer instead of waiting forever
	if err := cmd.Start(); err != nil {
		return false, err
	}
	success := false
	var failure string
	var need string // what Steam wants to let the account in
	var mt meter
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	sc.Split(scanLinesCR)
	for sc.Scan() {
		t := strings.TrimSpace(sc.Text())
		if t == "" {
			continue
		}
		if m := progressRe.FindStringSubmatch(t); m != nil {
			pct, _ := strconv.ParseFloat(m[2], 64)
			state := strings.TrimSpace(m[1])
			np := &Progress{Label: p.Label, Percent: pct, State: state, Line: t, Started: p.Started}
			if m[3] != "" {
				np.Done, _ = strconv.ParseInt(m[3], 10, 64)
				np.Total, _ = strconv.ParseInt(m[4], 10, 64)
				if strings.Contains(state, "download") {
					np.Speed = mt.add(state, np.Done, time.Now())
					if np.Speed > 0 && np.Total > np.Done {
						np.Left = int64(float64(np.Total-np.Done) / np.Speed)
					}
				} else {
					mt.add(state, np.Done, time.Now())
				}
			}
			r.set(np)
			// Every 10 %, a line in the console too
			if step := int(pct) / 10; step != mt.lastLogged || state != mt.state {
				mt.lastLogged = step
				msg := fmt.Sprintf("%s %.0f%%", state, pct)
				if np.Total > 0 {
					msg += fmt.Sprintf(" (%s of %s", HumanBytes(np.Done), HumanBytes(np.Total))
					if np.Speed > 0 {
						msg += fmt.Sprintf(", %s/s", HumanBytes(int64(np.Speed)))
					}
					msg += ")"
				}
				line(msg)
			}
		} else {
			np := Progress{Label: p.Label, Line: t, Started: p.Started}
			if c := r.Current(); c != nil {
				np.Percent, np.State, np.Done, np.Total = c.Percent, c.State, c.Done, c.Total
			}
			r.set(&np)
			line(t)
		}
		if strings.Contains(t, "Success! App") || strings.Contains(t, "already up to date") {
			success = true
		}
		if strings.HasPrefix(t, "Error!") || strings.Contains(t, "ERROR!") {
			failure = t
		}
		if n := loginNeed(t); needRank[n] > needRank[need] {
			need = n
		}
		if need == "approve" && !success {
			// steamcmd waits (about a minute) for the login to be approved in the Steam app: say it on the page
			if c := r.Current(); c != nil && c.Prompt != "approve" {
				c.Prompt = "approve"
				r.set(c)
			}
		}
		if why := loginTrouble(t); why != "" {
			failure = why
		}
	}
	werr := cmd.Wait()
	if success {
		return true, nil
	}
	if need != "" {
		if need == "password" && passwordGiven(args) {
			need = "bad_password"
		}
		return false, &LoginError{Need: need}
	}
	if failure != "" {
		return false, errors.New(failure)
	}
	if werr != nil {
		return false, fmt.Errorf("steamcmd: %v", werr)
	}
	return false, errors.New("steamcmd ended without installing the app")
}

// needRank: when steamcmd says several things, the most precise one wins (an approval in the Steam app is
// announced with the words "Steam Guard" too).
var needRank = map[string]int{"": 0, "password": 1, "guard": 2, "2fa": 3, "approve": 4}

// loginNeed reads what Steam asks of an account: a password, a Steam Guard code by email, the code of the Steam
// app, or an approval in the Steam app. steamcmd asks with nobody to answer (its input is empty), then says it
// failed; it waits for an approval a minute, then gives up.
func loginNeed(line string) string {
	l := strings.ToLower(line)
	switch {
	case strings.Contains(l, "confirm the login") || strings.Contains(l, "mobile authenticator") ||
		strings.Contains(l, "waiting for confirmation") || strings.Contains(l, "timed out waiting for confirmation"):
		return "approve"
	case strings.Contains(l, "two-factor") || strings.Contains(l, "two factor"):
		return "2fa"
	case strings.Contains(l, "steam guard") || strings.Contains(l, "not been authenticated") || strings.Contains(l, "account logon denied") ||
		strings.Contains(l, "check your email"):
		return "guard"
	case strings.Contains(l, "invalid password") || strings.HasSuffix(strings.TrimSpace(l), "password:") || strings.Contains(l, "password required"):
		return "password"
	}
	return ""
}

func passwordGiven(args []string) bool {
	for i, a := range args {
		if a == "+login" {
			return i+2 < len(args) && !strings.HasPrefix(args[i+2], "+")
		}
	}
	return false
}

// loginTrouble explains the other login refusals ("" when the line says nothing of them).
func loginTrouble(line string) string {
	l := strings.ToLower(line)
	switch {
	case strings.Contains(l, "rate limit"):
		return "Steam refuses logins for a while after too many tries: wait 30 minutes, or use anonymous"
	case strings.Contains(l, "no subscription"):
		return "This account does not own the game this server needs: use anonymous, or an account that owns it"
	}
	return ""
}

// steamcmd rewrites its progress line with \r: split on \r and \n.
func scanLinesCR(data []byte, atEOF bool) (int, []byte, error) {
	for i, b := range data {
		if b == '\n' || b == '\r' {
			return i + 1, data[:i], nil
		}
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// Download gets steamcmd from Valve into a folder: the path of steamcmd.exe (steamcmd.sh on Linux).
func Download(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	url, exe := "https://steamcdn-a.akamaihd.net/client/installer/steamcmd.zip", "steamcmd.exe"
	if runtime.GOOS != "windows" {
		url, exe = "https://steamcdn-a.akamaihd.net/client/installer/steamcmd_linux.tar.gz", "steamcmd.sh"
	}
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("download: %s", resp.Status)
	}
	tmp, err := os.CreateTemp(dir, "steamcmd-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		return "", err
	}
	tmp.Close()
	if strings.HasSuffix(url, ".zip") {
		err = unzip(tmp.Name(), dir)
	} else {
		err = untargz(tmp.Name(), dir)
	}
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, exe), nil
}

func safeJoin(dir, name string) (string, error) {
	p := filepath.Join(dir, name)
	if !strings.HasPrefix(p, filepath.Clean(dir)+string(os.PathSeparator)) {
		return "", fmt.Errorf("bad path in archive: %s", name)
	}
	return p, nil
}

func unzip(src, dir string) error {
	z, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer z.Close()
	for _, f := range z.File {
		p, err := safeJoin(dir, f.Name)
		if err != nil {
			return err
		}
		if f.FileInfo().IsDir() {
			os.MkdirAll(p, 0o755)
			continue
		}
		os.MkdirAll(filepath.Dir(p), 0o755)
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(p, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, f.Mode()|0o200)
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		rc.Close()
		out.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func untargz(src, dir string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		p, err := safeJoin(dir, h.Name)
		if err != nil {
			return err
		}
		switch h.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(p, 0o755)
		case tar.TypeReg:
			os.MkdirAll(filepath.Dir(p), 0o755)
			out, err := os.OpenFile(p, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(h.Mode)|0o200)
			if err != nil {
				return err
			}
			_, err = io.Copy(out, tr)
			out.Close()
			if err != nil {
				return err
			}
		}
	}
}
