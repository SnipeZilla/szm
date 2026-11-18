package web

import (
	"bytes"
	"encoding/binary"
	"net/http"
	"os"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SnipeZilla/szm/internal/config"
	"github.com/SnipeZilla/szm/internal/games"
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

// A folder uploaded keeps its tree, and never leaves the folder it is uploaded into.
func TestUploadRel(t *testing.T) {
	for in, want := range map[string]string{"": "", "maps": "maps", `cfg\sub`: "cfg/sub", "/a//b/./c/": "a/b/c"} {
		if got, err := uploadRel(in); err != nil || got != want {
			t.Fatalf("%q: %q, %v", in, got, err)
		}
	}
	if _, err := uploadRel("a/../../b"); err == nil {
		t.Fatal("a folder that goes up was taken")
	}
}

func TestWorkshopNames(t *testing.T) {
	dir := t.TempDir()
	put := func(rel string) {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	w := workshopDirs[0] + "/"
	put(w + "3256988376/de_warden.vpk")
	put(w + "3643331442/de_sanctum_dir.vpk") // a part only: no name
	put(w + "3366748499/readme.txt")
	put(workshopDirs[1] + "/3070284539/de_dust2_night.vpk")
	// as CS2 keeps it: a .vpk named by the ID, the map inside it
	var v bytes.Buffer
	binary.Write(&v, binary.LittleEndian, []uint32{0x55aa1234, 2, 0, 0, 0, 0, 0})
	entry := make([]byte, 18)
	v.WriteString("vmdl_c\x00models\x00box\x00")
	v.Write(entry)
	v.WriteString("\x00\x00vpk\x00maps\x00awp_lego_3\x00")
	v.Write(entry)
	v.WriteString("\x00\x00\x00")
	b := v.Bytes()
	binary.LittleEndian.PutUint32(b[8:], uint32(len(b)-28))
	p := filepath.Join(dir, filepath.FromSlash(w+"3240327667/3240327667_dir.vpk"))
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, b, 0o644)
	got := workshopNames(config.Server{InstallDir: dir}, &games.Game{})
	want := map[string]string{"3256988376": "de_warden", "3070284539": "de_dust2_night", "3240327667": "awp_lego_3", "3643331442": "de_sanctum"}
	if len(got) != len(want) || got["3240327667"] != "awp_lego_3" || got["3643331442"] != "de_sanctum" || got["3256988376"] != want["3256988376"] || got["3070284539"] != want["3070284539"] {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestUserScope(t *testing.T) {
	all := &User{Rights: "eg"}
	machine := &User{Rights: "g", Machines: []string{"us"}}
	here := &User{Rights: "g", Machines: []string{ThisMachine}}
	some := &User{Rights: "g", Servers: []string{"css-1", "eu~cs2-1"}}
	for _, c := range []struct {
		u    *User
		id   string
		want bool
	}{
		{all, "css-1", true}, {all, "us~css-1", true},
		{machine, "us~css-1", true}, {machine, "us~new", true}, {machine, "css-1", false}, {machine, "eu~cs2-1", false},
		{here, "css-1", true}, {here, "us~css-1", false},
		{some, "css-1", true}, {some, "eu~cs2-1", true}, {some, "eu~cs2-2", false}, {some, "css-2", false},
	} {
		if got := c.u.Sees(c.id); got != c.want {
			t.Errorf("%+v sees %s: %v, want %v", *c.u, c.id, got, c.want)
		}
	}
	if !some.OnMachine("eu") || some.OnMachine("us") || !some.OnMachine(ThisMachine) || machine.OnMachine(ThisMachine) {
		t.Error("OnMachine")
	}
	boss := &User{Name: "b", Rights: "dgi", Machines: []string{"us"}}
	for x, want := range map[*User]bool{
		{Name: "x", Rights: "g", Servers: []string{"us~css-1"}}: true,
		{Name: "x", Rights: "gi", Machines: []string{"us"}}:      true,
		{Name: "B", Rights: "a"}:                                 true, // themselves
		{Name: "x", Rights: "a"}:                                 false,
		{Name: "x", Rights: "eg"}:                                false,
		{Name: "x", Rights: "gj", Machines: []string{"us"}}:      false,
		{Name: "x", Rights: "g", Servers: []string{"css-1"}}:     false,
		{Name: "x", Rights: "g", Machines: []string{"eu"}}:       false,
	} {
		if boss.Manages(x) != want {
			t.Errorf("manages %+v: want %v", *x, want)
		}
	}
	if !machine.elsewhere() || here.elsewhere() || some.elsewhere() || all.elsewhere() || (&User{Rights: "c"}).elsewhere() {
		t.Error("elsewhere")
	}
}
