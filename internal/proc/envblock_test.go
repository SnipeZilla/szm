package proc

import (
	"strings"
	"testing"
	"unicode/utf16"
)

// The environment block of a server: NUL-separated (a NUL in a string was refused: "invalid argument"), its own
// variables winning.
func TestEnvBlock(t *testing.T) {
	u, err := envBlock([]string{"=C:=C:\\x", "Path=C:\\Windows", "TEMP=C:\\t", "SteamAppId=1"}, []string{"PATH=E:\\rust;C:\\Windows", "SteamAppId=892970", "Ünï=ç"})
	if err != nil {
		t.Fatal(err)
	}
	if len(u) < 2 || u[len(u)-1] != 0 || u[len(u)-2] != 0 {
		t.Fatal("not ended by two NULs")
	}
	got := strings.Split(strings.TrimRight(string(utf16.Decode(u)), "\x00"), "\x00")
	want := []string{"=C:=C:\\x", "TEMP=C:\\t", "PATH=E:\\rust;C:\\Windows", "SteamAppId=892970", "Ünï=ç"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("%q", got)
	}
	if _, err := envBlock(nil, []string{"A=b\x00c"}); err == nil {
		t.Fatal("a NUL")
	}
	if u, _ := envBlock(nil, nil); len(u) != 2 {
		t.Fatal(u)
	}
}
