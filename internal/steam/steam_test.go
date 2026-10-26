package steam

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

// steamcmd leaves its login lines unfinished while it waits (for the Steam app's approval): they are given after a
// moment all the same, and what finishes them after; dots alone are not lines.
func TestReadLines(t *testing.T) {
	pr, pw := io.Pipe()
	got := make(chan string, 20)
	done := make(chan bool)
	go func() {
		readLines(pr, 100*time.Millisecond, func(l string) { got <- l })
		close(done)
	}()
	next := func(want string) {
		t.Helper()
		select {
		case l := <-got:
			if l != want {
				t.Fatalf("%q, want %q", l, want)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("nothing, want %q", want)
		}
	}
	io.WriteString(pw, "Redirecting stderr to 'logs\\stderr.txt'\nLogging in user 'bob' to Steam Public...")
	next("Redirecting stderr to 'logs\\stderr.txt'")
	next("Logging in user 'bob' to Steam Public...") // unfinished, given after the pause
	io.WriteString(pw, "\nPlease confirm the login in the Steam Mobile app on your phone.\nWaiting for confirmation...")
	next("Please confirm the login in the Steam Mobile app on your phone.")
	next("Waiting for confirmation...")
	time.Sleep(150 * time.Millisecond)
	io.WriteString(pw, "...") // more dots: nothing
	time.Sleep(150 * time.Millisecond)
	io.WriteString(pw, "OK\r Update state (0x61) downloading, progress: 12.50 (100 / 800)\r")
	next("OK")
	next(" Update state (0x61) downloading, progress: 12.50 (100 / 800)")
	io.WriteString(pw, "Success! App '258550' fully installed.")
	pw.Close()
	next("Success! App '258550' fully installed.")
	<-done
	select {
	case l := <-got:
		t.Fatalf("more: %q", l)
	default:
	}
}

// An account logs in: the console says at once what Steam may ask (steamcmd holds back the rest); its color codes are
// not shown, its line said a dozen times once; once logged in, Steam's question is answered.
func TestLoginLines(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a shell script")
	}
	dir := t.TempDir()
	sh := filepath.Join(dir, "steamcmd.sh")
	os.WriteFile(sh, []byte("#!/bin/sh\nprintf 'Loading Steam API...\\033[0mOK\\n\\033[0m\\033[1mLogging in using username/password.\\n'\n"+
		"printf 'Please confirm the login in the Steam Mobile app on your phone.\\nWaiting for confirmation...\\nWaiting for confirmation...\\nWaiting for user info...OK\\nSuccess! App 223350 fully installed.\\n'\n"), 0o755)
	r := &Runner{Path: sh}
	var lines []string
	ok, err := r.once(context.Background(), []string{"+login", "bob", "+app_update", "223350", "+quit"}, &Progress{Label: "install DayZ"}, func(l string) {
		lines = append(lines, l)
	})
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	if c := r.Current(); c.Prompt != "" {
		t.Fatal("prompt after the login:", c.Prompt)
	}
	got := strings.Join(lines, "|")
	if got != "Logging in as bob. If Steam asks: approve the login in the Steam app, or wait for the code it emails (asked on the page once steamcmd stops)|Loading Steam API...OK|Logging in using username/password.|Please confirm the login in the Steam Mobile app on your phone.|Waiting for confirmation...|Waiting for user info...OK|Success! App 223350 fully installed." {
		t.Fatal(got)
	}
	// anonymous: nothing said
	lines = nil
	if _, err := r.once(context.Background(), []string{"+login", "anonymous", "+app_update", "223350", "+quit"}, &Progress{}, func(l string) { lines = append(lines, l) }); err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(lines[0], "Logging in as") {
		t.Fatal(lines[0])
	}
}

// A code Steam emails: steamcmd asks for it in its console and waits; it is ended in a few seconds, and the page
// asks for the code. An approval in the Steam app is waited for (steamcmd gives up itself).
func TestGuardCodeAsked(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("a shell script")
	}
	dir := t.TempDir()
	sh := filepath.Join(dir, "steamcmd.sh")
	os.WriteFile(sh, []byte("#!/bin/sh\nprintf 'Logging in user bob to Steam Public...\\nThis computer has not been authenticated for your account using Steam Guard.\\n"+
		"Please check your email for the message from Steam, and enter the Steam Guard code from that message.\\nSteam Guard code:'\nread code\necho logged in with $code\n"), 0o755)
	r := &Runner{Path: sh}
	start := time.Now()
	_, err := r.once(context.Background(), []string{"+login", "bob", "pw", "+app_update", "223350", "+quit"}, &Progress{}, func(string) {})
	var le *LoginError
	if !errors.As(err, &le) || le.Need != "guard" {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 6*time.Second {
		t.Fatal("ended late:", d)
	}
	// the Steam app: waited for
	os.WriteFile(sh, []byte("#!/bin/sh\nprintf 'This account is protected by a Steam Guard mobile authenticator.\\nPlease confirm the login in the Steam Mobile app on your phone.\\nWaiting for confirmation...'\n"+
		"sleep 4\nprintf 'OK\\nWaiting for user info...OK\\nSuccess! App 223350 fully installed.\\n'\n"), 0o755)
	ok, err := r.once(context.Background(), []string{"+login", "bob", "pw", "+app_update", "223350", "+quit"}, &Progress{}, func(string) {})
	if err != nil || !ok {
		t.Fatal("approval not waited for:", ok, err)
	}
}
