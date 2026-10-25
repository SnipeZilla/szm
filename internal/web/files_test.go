package web

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestJail(t *testing.T) {
	root := filepath.FromSlash("/srv/css")
	for _, rel := range []string{"../../etc/passwd", "/../..", "cstrike/../../x", `..\..\x`} {
		p, err := jail(root, rel)
		if err == nil && !strings.HasPrefix(p, root) {
			t.Fatalf("%s escaped: %s", rel, p)
		}
	}
	if _, err := jail(root, "C:/Windows"); err == nil {
		t.Fatal("drive accepted")
	}
}

func TestPassword(t *testing.T) {
	h := hashPassword("correct horse")
	if !checkPassword(h, "correct horse") || checkPassword(h, "wrong") {
		t.Fatal("password check")
	}
}

func TestProxy(t *testing.T) {
	req := func(remote string, hdr map[string]string) *http.Request {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = remote
		for k, v := range hdr {
			r.Header.Set(k, v)
		}
		return r
	}
	// Direct on the machine: local
	if r := req("127.0.0.1:5000", nil); !isLocal(r) || clientIP(r) != "127.0.0.1" {
		t.Fatal("direct local")
	}
	// Through IIS: not local, the browser's address (IIS adds the port), HTTPS seen
	r := req("127.0.0.1:5000", map[string]string{"X-Forwarded-For": "6.6.6.6, 203.0.113.9:51234", "X-ARR-SSL": "2048|256|..."})
	if isLocal(r) || clientIP(r) != "203.0.113.9" || !isHTTPS(r) {
		t.Fatal("iis", isLocal(r), clientIP(r), isHTTPS(r))
	}
	// A remote client faking the headers is not believed
	r = req("198.51.100.7:4000", map[string]string{"X-Forwarded-For": "127.0.0.1", "X-Forwarded-Proto": "https"})
	if isLocal(r) || clientIP(r) != "198.51.100.7" || isHTTPS(r) {
		t.Fatal("spoof")
	}
	// IIS without X-Forwarded-For still is a proxy
	if r := req("[::1]:5000", map[string]string{"X-Original-URL": "/szm/"}); isLocal(r) {
		t.Fatal("arr")
	}
}

// A password set from the command line (szm password) while the manager runs is the one that works, and a change
// made on the page afterwards does not bring the old one back.
func TestUsersFileChanged(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "users.json")
	u, _ := loadUsers(p)
	if err := u.put(User{Name: "admin", Rights: "a"}, "old-password"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond) // a new modification time
	if err := SetPassword(dir, "admin", "new-password"); err != nil {
		t.Fatal(err)
	}
	u.checkedAt = time.Time{} // as 2 s later
	if !checkPassword(u.get("admin").Hash, "new-password") {
		t.Fatal("the running manager does not know the new password")
	}
	u.put(User{Name: "bob", Rights: "b"}, "bob-password") // a change on the page
	again, _ := loadUsers(p)
	if a := again.get("admin"); a == nil || !checkPassword(a.Hash, "new-password") || again.get("bob") == nil {
		t.Fatal("the old password came back")
	}
}
