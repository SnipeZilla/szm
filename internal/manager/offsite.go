package manager

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/SnipeZilla/szm/internal/config"
	"github.com/SnipeZilla/szm/internal/events"
	"github.com/SnipeZilla/szm/internal/proc"
)

// Off-site copies of the backups: each new backup is copied to the destinations of the settings (with rclone:
// Google Drive, OneDrive, Dropbox, SFTP, S3…; or into a folder: a synced one, another disk, a network share), and
// the same number is kept there as here.

// OffsiteStatus is how the copies to a destination go.
type OffsiteStatus struct {
	Name   string     `json:"name"`
	Kind   string     `json:"kind"`
	Target string     `json:"target"`
	OK     *time.Time `json:"ok,omitempty"`    // the last copy that worked
	At     *time.Time `json:"at,omitempty"`    // the last try
	Error  string     `json:"error,omitempty"` // of the last try
	Busy   bool       `json:"busy,omitempty"`
}

type offsiteState struct {
	mu      sync.Mutex
	status  map[string]*OffsiteStatus // by kind + target
	pending map[string]bool           // the servers waiting for a copy
	running bool
}

func offsiteKey(c config.OffsiteCopy) string { return c.Kind + "|" + c.Target }

// RClonePath is rclone's program: the settings', else the one the manager downloaded, else rclone in the PATH ("" when
// there is none).
func (m *Manager) RClonePath() string {
	m.Cfg.RLock()
	p := strings.TrimSpace(m.Cfg.Backups.RClone)
	m.Cfg.RUnlock()
	if p != "" {
		return p
	}
	if p := m.rcloneLocal(); fileExists(p) {
		return p
	}
	if p, err := exec.LookPath("rclone"); err == nil {
		return p
	}
	return ""
}

func (m *Manager) rcloneLocal() string {
	name := "rclone"
	if runtime.GOOS == "windows" {
		name = "rclone.exe"
	}
	return filepath.Join(m.DataDir, "rclone", name)
}

// RCloneConfig is the configuration file rclone uses here (its remotes: gdrive, onedrive…): next to the manager's data,
// so that the service (another Windows user) finds the same.
func (m *Manager) RCloneConfig() string { return filepath.Join(m.DataDir, "rclone", "rclone.conf") }

// RClone runs rclone with the manager's configuration; it gives what it wrote.
func (m *Manager) RClone(ctx context.Context, args ...string) (string, error) {
	exe := m.RClonePath()
	if exe == "" {
		return "", errors.New("rclone is not installed (Settings, Backups: Download rclone)")
	}
	_ = os.MkdirAll(filepath.Dir(m.RCloneConfig()), 0o755)
	cmd := exec.CommandContext(ctx, exe, append([]string{"--config", m.RCloneConfig()}, args...)...)
	proc.RunHiddenCmd(cmd)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	text := strings.TrimSpace(out.String())
	if err != nil {
		if text == "" {
			text = err.Error()
		}
		return text, errors.New(lastLines(text, 3))
	}
	return text, nil
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	for k := range lines {
		lines[k] = strings.TrimSpace(lines[k])
	}
	return strings.Join(lines, " ")
}

// RCloneInfo says whether rclone is there, its version, and its remotes (gdrive:, onedrive:…).
func (m *Manager) RCloneInfo() map[string]any {
	info := map[string]any{"path": m.RClonePath(), "config": m.RCloneConfig(), "local": m.rcloneLocal(), "remotes": []string{}}
	if info["path"] == "" {
		return info
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if v, err := m.RClone(ctx, "version"); err == nil {
		info["version"] = strings.TrimSpace(strings.SplitN(v, "\n", 2)[0])
	} else {
		info["error"] = err.Error()
		return info
	}
	if r, err := m.RClone(ctx, "listremotes"); err == nil {
		var list []string
		for _, l := range strings.Split(r, "\n") {
			if l = strings.TrimSpace(l); l != "" {
				list = append(list, l)
			}
		}
		if list != nil {
			info["remotes"] = list
		}
	}
	return info
}

// DownloadRClone gets rclone from rclone.org into <data>/rclone.
func (m *Manager) DownloadRClone() (string, error) {
	osName := runtime.GOOS
	if osName == "darwin" {
		osName = "osx"
	}
	url := fmt.Sprintf("https://downloads.rclone.org/rclone-current-%s-%s.zip", osName, runtime.GOARCH)
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("download: %s", resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 200<<20))
	if err != nil {
		return "", err
	}
	z, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return "", err
	}
	want := filepath.Base(m.rcloneLocal())
	for _, f := range z.File {
		if f.FileInfo().IsDir() || filepath.Base(f.Name) != want {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		dst := m.rcloneLocal()
		_ = os.MkdirAll(filepath.Dir(dst), 0o755)
		tmp := dst + ".part"
		out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err == nil {
			_, err = io.Copy(out, rc)
			if cerr := out.Close(); err == nil {
				err = cerr
			}
		}
		rc.Close()
		if err == nil {
			err = os.Rename(tmp, dst)
		}
		if err != nil {
			os.Remove(tmp)
			return "", err
		}
		m.Log.Add(events.Manager, "", "", "rclone downloaded: %s", dst)
		return dst, nil
	}
	return "", errors.New("rclone was not found in the download")
}

// TestOffsite checks a destination: it can be written to.
func (m *Manager) TestOffsite(c config.OffsiteCopy) (string, error) {
	switch c.Kind {
	case "folder":
		if err := os.MkdirAll(c.Target, 0o755); err != nil {
			return "", err
		}
		p := filepath.Join(c.Target, ".szm-test")
		if err := os.WriteFile(p, []byte("ok"), 0o644); err != nil {
			return "", err
		}
		os.Remove(p)
		return "The folder can be written to.", nil
	case "rclone":
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if _, err := m.RClone(ctx, "mkdir", c.Target); err != nil {
			return "", err
		}
		out, err := m.RClone(ctx, "lsf", c.Target, "--max-depth", "1")
		if err != nil {
			return "", err
		}
		n := 0
		for _, l := range strings.Split(out, "\n") {
			if strings.TrimSpace(l) != "" {
				n++
			}
		}
		if n == 0 {
			return "Reached " + c.Target + ": empty so far.", nil
		}
		return fmt.Sprintf("Reached %s: %d item(s) there.", c.Target, n), nil
	}
	return "", errors.New("unknown kind of destination")
}

// OffsiteStatuses gives how the copies go, one per destination of the settings.
func (m *Manager) OffsiteStatuses() []OffsiteStatus {
	m.Cfg.RLock()
	list := append([]config.OffsiteCopy(nil), m.Cfg.Backups.Offsite...)
	m.Cfg.RUnlock()
	m.offsite.mu.Lock()
	defer m.offsite.mu.Unlock()
	out := []OffsiteStatus{}
	for _, c := range list {
		st := OffsiteStatus{Name: c.Name, Kind: c.Kind, Target: c.Target}
		if s := m.offsite.status[offsiteKey(c)]; s != nil {
			st.OK, st.At, st.Error, st.Busy = s.OK, s.At, s.Error, s.Busy
		}
		out = append(out, st)
	}
	return out
}

// queueOffsite copies a server's backups to the destinations soon (in the background, one server after the other).
// No id: every server's.
func (m *Manager) queueOffsite(ids ...string) {
	m.Cfg.RLock()
	n := len(m.Cfg.Backups.Offsite)
	m.Cfg.RUnlock()
	if n == 0 {
		return
	}
	if len(ids) == 0 {
		for _, s := range m.servers() {
			ids = append(ids, s.ID)
		}
	}
	m.offsite.mu.Lock()
	if m.offsite.pending == nil {
		m.offsite.pending = map[string]bool{}
	}
	for _, id := range ids {
		m.offsite.pending[id] = true
	}
	if m.offsite.running {
		m.offsite.mu.Unlock()
		return
	}
	m.offsite.running = true
	m.offsite.mu.Unlock()
	go func() {
		for {
			m.offsite.mu.Lock()
			var id string
			for k := range m.offsite.pending {
				id = k
				break
			}
			if id == "" {
				m.offsite.running = false
				m.offsite.mu.Unlock()
				return
			}
			delete(m.offsite.pending, id)
			m.offsite.mu.Unlock()
			m.copyOffsite(id)
		}
	}()
}

// offsiteFailing: a destination's last copy failed.
func (m *Manager) offsiteFailing() bool {
	for _, st := range m.OffsiteStatuses() {
		if st.Error != "" {
			return true
		}
	}
	return false
}

// SyncOffsite copies every server's backups to the destinations now (those not there yet).
func (m *Manager) SyncOffsite() { m.queueOffsite() }

// copyOffsite copies a server's backups that a destination does not have yet, and removes there the ones past the
// number kept.
func (m *Manager) copyOffsite(id string) {
	m.Cfg.RLock()
	list := append([]config.OffsiteCopy(nil), m.Cfg.Backups.Offsite...)
	keep := m.Cfg.Backups.Keep
	m.Cfg.RUnlock()
	local := m.Backups(id)
	if len(local) == 0 {
		return
	}
	name := id
	if s := m.Cfg.Server(id); s != nil {
		name = s.Name
	}
	for _, c := range list {
		key := offsiteKey(c)
		m.offsite.mu.Lock()
		if m.offsite.status == nil {
			m.offsite.status = map[string]*OffsiteStatus{}
		}
		st := m.offsite.status[key]
		if st == nil {
			st = &OffsiteStatus{}
			m.offsite.status[key] = st
		}
		st.Busy = true
		m.offsite.mu.Unlock()

		var copied int
		var err error
		switch c.Kind {
		case "folder":
			copied, err = m.copyToFolder(id, c.Target, local, keep)
		case "rclone":
			copied, err = m.copyWithRClone(id, c.Target, local, keep)
		default:
			err = errors.New("unknown kind of destination")
		}

		m.offsite.mu.Lock()
		now := time.Now()
		st.Busy, st.At = false, &now
		prev := st.Error
		if err != nil {
			st.Error = err.Error()
		} else {
			st.Error, st.OK = "", &now
		}
		m.offsite.mu.Unlock()
		label := c.Name
		if label == "" {
			label = c.Target
		}
		switch {
		case err != nil && err.Error() != prev:
			m.Log.Add(events.Error, id, name, "Off-site copy to %s failed: %v", label, err)
		case err == nil && prev != "":
			m.Log.Add(events.Info, id, name, "Off-site copies to %s work again", label)
		}
		if err == nil && copied > 0 {
			m.Log.Add(events.Info, id, name, "Backup copied to %s", label)
		}
	}
}

// copyToFolder copies the backups missing in <target>/<server> and prunes there.
func (m *Manager) copyToFolder(id, target string, local []Backup, keep int) (int, error) {
	dst := filepath.Join(target, safeName(id))
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return 0, err
	}
	src := m.BackupsDir(id)
	copied := 0
	for _, b := range local {
		p := filepath.Join(dst, b.Name)
		if st, err := os.Stat(p); err == nil && st.Size() == b.Size {
			continue
		}
		if err := copyBackup(filepath.Join(src, b.Name), p); err != nil {
			return copied, err
		}
		copied++
	}
	entries, _ := os.ReadDir(dst)
	var there []string
	for _, e := range entries {
		if !e.IsDir() && backupNameRe.MatchString(e.Name()) {
			there = append(there, e.Name())
		}
	}
	for _, n := range pastKeep(there, keep) {
		os.Remove(filepath.Join(dst, n))
	}
	return copied, nil
}

// copyWithRClone does the same with rclone: <remote:path>/<server>.
func (m *Manager) copyWithRClone(id, target string, local []Backup, keep int) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	dst := strings.TrimRight(target, "/") + "/" + safeName(id)
	before := map[string]bool{}
	if out, err := m.RClone(ctx, "lsf", dst, "--files-only"); err == nil {
		for _, l := range strings.Split(out, "\n") {
			before[strings.TrimSpace(l)] = true
		}
	}
	missing := 0
	for _, b := range local {
		if !before[b.Name] {
			missing++
		}
	}
	if missing > 0 {
		if _, err := m.RClone(ctx, "copy", m.BackupsDir(id), dst, "--include", "*.zip"); err != nil {
			return 0, err
		}
	}
	out, err := m.RClone(ctx, "lsf", dst, "--files-only")
	if err != nil {
		return missing, err
	}
	var there []string
	for _, l := range strings.Split(out, "\n") {
		if l = strings.TrimSpace(l); backupNameRe.MatchString(l) {
			there = append(there, l)
		}
	}
	for _, n := range pastKeep(there, keep) {
		if _, err := m.RClone(ctx, "deletefile", dst+"/"+n); err != nil {
			return missing, err
		}
	}
	return missing, nil
}

// pastKeep gives the backups past the number kept: the oldest (their names start with the time).
func pastKeep(names []string, keep int) []string {
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	if keep < 1 || len(names) <= keep {
		return nil
	}
	return names[keep:]
}

func copyBackup(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, dst)
	}
	if err != nil {
		os.Remove(tmp)
	}
	return err
}
