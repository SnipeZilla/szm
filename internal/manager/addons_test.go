package manager

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/SnipeZilla/szm/internal/config"
	"github.com/SnipeZilla/szm/internal/events"
)

// An archive into a package: replaced, updated (its files over the package's, the others kept), or added (nothing
// overwritten).
func TestExtractPackageModes(t *testing.T) {
	dir := t.TempDir()
	m := &Manager{DataDir: dir, Log: events.New(filepath.Join(dir, "logs")), Cfg: &config.Config{}}
	zipOf := func(files map[string]string) string {
		p := filepath.Join(t.TempDir(), "a.zip")
		f, _ := os.Create(p)
		z := zip.NewWriter(f)
		for n, c := range files {
			w, _ := z.Create("plugin-1.2/" + n) // wrapped in a folder: unwrapped
			w.Write([]byte(c))
		}
		z.Close()
		f.Close()
		return p
	}
	pkg, _ := m.PackagePath("sm")
	read := func(rel string) string {
		b, err := os.ReadFile(filepath.Join(pkg, filepath.FromSlash(rel)))
		if err != nil {
			return "-"
		}
		return string(b)
	}
	v1 := zipOf(map[string]string{"addons/sm/a.smx": "a1", "cfg/sm/a.cfg": "cfg1"})
	if n, _, err := m.ExtractPackage("sm", v1, "v1.zip", PkgReplace, "t"); err != nil || n != 2 {
		t.Fatal(n, err)
	}
	_ = os.WriteFile(filepath.Join(pkg, "addons", "sm", "mine.smx"), []byte("mine"), 0o644) // one of the package's own
	_ = os.WriteFile(filepath.Join(pkg, "cfg", "sm", "a.cfg"), []byte("edited"), 0o644)

	v2 := zipOf(map[string]string{"addons/sm/a.smx": "a2", "addons/sm/b.smx": "b2", "cfg/sm/a.cfg": "cfg2"})
	// add: only b.smx is new
	if n, kept, err := m.ExtractPackage("sm", v2, "v2.zip", PkgAdd, "t"); err != nil || n != 1 || kept != 2 {
		t.Fatal(n, kept, err)
	}
	if read("addons/sm/a.smx") != "a1" || read("addons/sm/b.smx") != "b2" || read("cfg/sm/a.cfg") != "edited" {
		t.Fatal("add overwrote:", read("addons/sm/a.smx"), read("cfg/sm/a.cfg"))
	}
	// update: the archive's files over the package's, its own kept
	if n, _, err := m.ExtractPackage("sm", v2, "v2.zip", PkgUpdate, "t"); err != nil || n != 3 {
		t.Fatal(n, err)
	}
	if read("addons/sm/a.smx") != "a2" || read("cfg/sm/a.cfg") != "cfg2" || read("addons/sm/mine.smx") != "mine" {
		t.Fatal("update:", read("addons/sm/a.smx"), read("addons/sm/mine.smx"))
	}
	// replace: only the archive
	if _, _, err := m.ExtractPackage("sm", v1, "v1.zip", PkgReplace, "t"); err != nil || read("addons/sm/mine.smx") != "-" || read("addons/sm/b.smx") != "-" {
		t.Fatal("replace kept files:", err)
	}
}
