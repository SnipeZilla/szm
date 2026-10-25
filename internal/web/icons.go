package web

import (
	"bytes"
	"encoding/base64"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/SnipeZilla/szm/internal/events"
)

// Game icons: the ones built into szm.exe (static/games), and the ones the admin adds in <data>/icons, which win
// when both have the same name. Icons added there work at once, without building szm.exe again.

var iconNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

var pngMagic = []byte("\x89PNG\r\n\x1a\n")

const maxIcon = 1 << 20

func (s *Server) iconsDir() string { return filepath.Join(s.M.DataDir, "icons") }

// iconPNG gives the PNG of an icon (nil when there is none).
func (s *Server) iconPNG(name string) []byte {
	name = strings.ToLower(strings.TrimSuffix(name, ".png"))
	if !iconNameRe.MatchString(name) {
		return nil
	}
	if b, err := os.ReadFile(filepath.Join(s.iconsDir(), name+".png")); err == nil && bytes.HasPrefix(b, pngMagic) {
		return b
	}
	b, _ := fs.ReadFile(static, "static/games/"+name+".png")
	return b
}

// iconNames lists every icon, built-in and added.
func (s *Server) iconNames() []string {
	seen := map[string]bool{}
	add := func(n string) {
		n = strings.ToLower(n)
		if strings.HasSuffix(n, ".png") && iconNameRe.MatchString(strings.TrimSuffix(n, ".png")) {
			seen[strings.TrimSuffix(n, ".png")] = true
		}
	}
	if entries, err := fs.ReadDir(static, "static/games"); err == nil {
		for _, e := range entries {
			add(e.Name())
		}
	}
	if entries, err := os.ReadDir(s.iconsDir()); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				add(e.Name())
			}
		}
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// serveIcon answers GET /games/<name>.png.
func (s *Server) serveIcon(w http.ResponseWriter, r *http.Request) {
	b := s.iconPNG(r.PathValue("file"))
	if b == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(b)
}

// uploadIcon saves an icon in <data>/icons: {"name": "csgo", "png": "<base64>"}.
func (s *Server) uploadIcon(w http.ResponseWriter, r *http.Request, u *User) error {
	var in struct {
		Name string `json:"name"`
		PNG  string `json:"png"`
	}
	if err := readJSON(r, &in); err != nil {
		return err
	}
	name := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(in.Name), ".png"))
	if !iconNameRe.MatchString(name) {
		return fail(http.StatusBadRequest, "Name the icon with letters, digits, - or _ (32 at most).")
	}
	b, err := base64.StdEncoding.DecodeString(in.PNG)
	if err != nil || !bytes.HasPrefix(b, pngMagic) {
		return fail(http.StatusBadRequest, "This is not a PNG picture.")
	}
	if len(b) > maxIcon {
		return fail(http.StatusBadRequest, "This picture is too big (1 MB at most; 64×64 is plenty).")
	}
	if err := os.MkdirAll(s.iconsDir(), 0o755); err != nil {
		return fail(http.StatusInternalServerError, "%s", capitalize(err.Error()))
	}
	if err := os.WriteFile(filepath.Join(s.iconsDir(), name+".png"), b, 0o644); err != nil {
		return fail(http.StatusInternalServerError, "%s", capitalize(err.Error()))
	}
	s.M.Log.Add(events.Manager, "", "", "Icon %s added by %s", name, u.Name)
	return writeJSON(w, map[string]any{"ok": true, "name": name, "icons": s.iconNames()})
}
