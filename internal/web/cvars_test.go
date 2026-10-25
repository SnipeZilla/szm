package web

import "testing"

func TestCvarRe(t *testing.T) {
	cfg := "// server.cfg\r\nhostname \"Old name\" // shown in the browser\r\n  sv_password   secret\r\nsv_lan 0\r\n"
	if m := cvarRe("hostname").FindStringSubmatch(cfg); m == nil || m[1] != `"Old name"` {
		t.Fatalf("hostname: %q", m)
	}
	if m := cvarRe("sv_password").FindStringSubmatch(cfg); m == nil || m[1] != "secret" {
		t.Fatalf("sv_password: %q", m)
	}
	if cvarRe("sv_lan").FindStringSubmatch("sv_lanx 1") != nil {
		t.Fatal("matched another cvar")
	}
}

func TestSaveCvarsText(t *testing.T) {
	cfg := "hostname \"Old\"\r\nsv_lan 0\r\n"
	re := cvarRe("hostname")
	loc := re.FindStringIndex(cfg)
	if loc == nil || cfg[loc[0]:loc[1]] != "hostname \"Old\"\r" {
		t.Fatalf("%q", loc)
	}
}
