package web

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/SnipeZilla/szm/internal/manager"
)

func TestIcons(t *testing.T) {
	dir := t.TempDir()
	s := &Server{M: &manager.Manager{DataDir: dir}}
	if s.iconPNG("cs2") == nil {
		t.Fatal("built-in cs2 icon missing")
	}
	if s.iconPNG("zzicon") != nil || s.iconPNG("../users") != nil {
		t.Fatal("unexpected icon")
	}
	png := append([]byte(nil), pngMagic...)
	png = append(png, 1, 2, 3)
	_ = os.MkdirAll(s.iconsDir(), 0o755)
	_ = os.WriteFile(filepath.Join(s.iconsDir(), "zzicon.png"), png, 0o644)
	_ = os.WriteFile(filepath.Join(s.iconsDir(), "notes.txt"), []byte("x"), 0o644)
	if string(s.iconPNG("zzicon")) != string(png) {
		t.Fatal("added icon not read")
	}
	found := false
	for _, n := range s.iconNames() {
		if n == "zzicon" {
			found = true
		}
		if n == "notes" {
			t.Fatal("txt listed")
		}
	}
	if !found {
		t.Fatal("zzicon not listed")
	}
	rr := httptest.NewRecorder()
	s.routes().ServeHTTP(rr, httptest.NewRequest("GET", "/games/zzicon.png", nil))
	if rr.Code != 200 || rr.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("serve: %d %s", rr.Code, rr.Header().Get("Content-Type"))
	}
	rr = httptest.NewRecorder()
	s.routes().ServeHTTP(rr, httptest.NewRequest("GET", "/games/tf.png", nil))
	if rr.Code != 200 {
		t.Fatalf("built-in serve: %d", rr.Code)
	}
	rr = httptest.NewRecorder()
	s.routes().ServeHTTP(rr, httptest.NewRequest("GET", "/games/nope.png", nil))
	if rr.Code != 404 {
		t.Fatalf("missing: %d", rr.Code)
	}
}
