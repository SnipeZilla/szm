package manager

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
)

// linkSteamClient: on Linux, a Source server loads Steam's library steamclient.so from ~/.steam/sdk32 (a 32-bit
// server) or ~/.steam/sdk64 (a 64-bit one), in the home of the user it runs as: the manager's. Without it the server
// says "Unable to load Steam support library" and plays on the LAN only. steamcmd has both: they are linked there,
// once (what is already there is left; a link to nowhere is made again).
func (m *Manager) linkSteamClient(i *Instance) {
	if runtime.GOOS != "linux" {
		return
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		if u, err := user.Current(); err == nil {
			home = u.HomeDir
		}
	}
	if home == "" {
		return
	}
	dir := filepath.Dir(m.SteamCmdPath())
	for _, p := range [][2]string{{"linux32", "sdk32"}, {"linux64", "sdk64"}} {
		src := filepath.Join(dir, p[0], "steamclient.so")
		if !fileExists(src) {
			continue
		}
		dst := filepath.Join(home, ".steam", p[1], "steamclient.so")
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		_ = os.MkdirAll(filepath.Dir(dst), 0o755)
		_ = os.Remove(dst) // a link to nowhere
		if err := os.Symlink(src, dst); err != nil {
			i.say(fmt.Sprintf("[steam] %s could not be linked to %s: %v (the server may play on the LAN only)", dst, src, err))
			continue
		}
		i.say(fmt.Sprintf("[steam] Steam's library linked for the servers: %s -> %s", dst, src))
	}
}
