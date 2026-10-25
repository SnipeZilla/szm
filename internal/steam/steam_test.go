package steam

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProgressLine(t *testing.T) {
	m := progressRe.FindStringSubmatch("Update state (0x61) downloading, progress: 78.12 (27891234567 / 35700000000)")
	if m == nil || m[1] != "downloading" || m[2] != "78.12" || m[3] != "27891234567" || m[4] != "35700000000" {
		t.Fatalf("%q", m)
	}
	if m := progressRe.FindStringSubmatch("Update state (0x5) verifying install, progress: 12.00"); m == nil || m[3] != "" {
		t.Fatalf("%q", m)
	}
}

func TestMeter(t *testing.T) {
	var mt meter
	t0 := time.Now()
	mt.add("downloading", 0, t0)
	if s := mt.add("downloading", 20_000_000, t0.Add(2*time.Second)); s != 10_000_000 {
		t.Fatalf("speed %v", s)
	}
	if s := mt.add("verifying", 5, t0.Add(3*time.Second)); s != 0 {
		t.Fatalf("a new step starts again: %v", s)
	}
	if HumanBytes(35_700_000_000) != "33.2 GB" || HumanBytes(900) != "900 B" || HumanBytes(10_000_000) != "9.5 MB" {
		t.Fatal(HumanBytes(35_700_000_000))
	}
}

func TestLoginNeed(t *testing.T) {
	cases := map[string]string{
		"Logging in user 'snipe' to Steam Public...FAILED (Account Logon Denied)":      "guard",
		"This computer has not been authenticated for your account using Steam Guard.": "guard",
		"Steam Guard code:": "guard",
		"Two-factor code:":  "2fa",
		"This account is protected by a Steam Guard mobile authenticator.": "approve",
		"Please confirm the login in the Steam Mobile app on your phone.":  "approve",
		"Waiting for confirmation...":                                      "approve",
		"FAILED (Invalid Password)":                                        "password",
		"password:":                                                        "password",
		"Connecting anonymously to Steam Public...OK":                      "",
	}
	for line, want := range cases {
		if got := loginNeed(line); got != want {
			t.Errorf("%q: %q, want %q", line, got, want)
		}
	}
	if !passwordGiven([]string{"+force_install_dir", "x", "+login", "u", "p", "+app_update", "90"}) ||
		passwordGiven([]string{"+login", "u", "+app_update", "90"}) {
		t.Fatal("passwordGiven")
	}
	if loginTrouble("FAILED (Rate Limit Exceeded)") == "" {
		t.Fatal("rate limit")
	}
}

// The games of other engines: the build installed (appmanifest), the newest one (app_info_print).
func TestBuilds(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "steamapps"), 0o755)
	os.WriteFile(filepath.Join(dir, "steamapps", "appmanifest_258550.acf"), []byte("\"AppState\"\n{\n\t\"appid\"\t\t\"258550\"\n\t\"buildid\"\t\t\"20441234\"\n}\n"), 0o644)
	if text, n := ReadBuild(dir, 258550); text != "build 20441234" || n != 20441234 {
		t.Fatal(text, n)
	}
	if _, n := ReadBuild(dir, 1); n != 0 {
		t.Fatal("no manifest")
	}
	out := []byte(`AppID : 258550, change number : 1/1
"258550"
{
	"depots"
	{
		"258551" { "manifests" { "public" { "gid" "123" } } }
		"branches"
		{
			"public"
			{
				"buildid"		"20449999"
				"timeupdated"		"1761300000"
			}
			"staging"
			{
				"buildid"		"20450001"
			}
		}
	}
}`)
	if n, err := parseLatestBuild(out, nil); err != nil || n != 20449999 {
		t.Fatal(n, err)
	}
	if _, err := parseLatestBuild([]byte("No app info for AppID 1 found"), nil); err == nil {
		t.Fatal("no build")
	}
}
