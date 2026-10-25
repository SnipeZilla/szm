package web

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/SnipeZilla/szm/internal/config"
)

// rcloneInfo: rclone's program, version and remotes (gdrive:, onedrive:…), and the command that adds a remote.
func (s *Server) rcloneInfo(w http.ResponseWriter, r *http.Request, u *User) error {
	return writeJSON(w, s.M.RCloneInfo())
}

// rcloneDownload gets rclone from rclone.org into the data folder.
func (s *Server) rcloneDownload(w http.ResponseWriter, r *http.Request, u *User) error {
	if _, err := s.M.DownloadRClone(); err != nil {
		return fail(http.StatusBadGateway, "rclone could not be downloaded: %v", err)
	}
	return writeJSON(w, s.M.RCloneInfo())
}

// offsiteTest checks that a destination can be reached and written to.
func (s *Server) offsiteTest(w http.ResponseWriter, r *http.Request, u *User) error {
	var in config.OffsiteCopy
	if err := readJSON(r, &in); err != nil {
		return err
	}
	in.Target = strings.TrimSpace(in.Target)
	switch {
	case in.Target == "":
		return fail(http.StatusBadRequest, "Say where.")
	case in.Kind == "folder" && !filepath.IsAbs(in.Target):
		return fail(http.StatusBadRequest, "The folder is a full path.")
	case in.Kind == "rclone" && !strings.Contains(in.Target, ":"):
		return fail(http.StatusBadRequest, "An rclone remote and a folder, like gdrive:SZM backups.")
	}
	msg, err := s.M.TestOffsite(in)
	if err != nil {
		return fail(http.StatusBadRequest, "%s", err.Error())
	}
	return writeJSON(w, map[string]any{"ok": true, "message": msg})
}
