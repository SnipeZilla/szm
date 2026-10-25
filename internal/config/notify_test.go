package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOldDiscordAndForumMoved(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "szm.json")
	old := `{"discord":{"webhook":"https://discord.com/api/webhooks/1/x","username":"SZ","events":["crash","updated"]},
"xenforo":{"url":"https://snipezilla.com","key":"k","thread":12,"events":["crash"]}}`
	_ = os.WriteFile(p, []byte(old), 0o644)
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Discords) != 1 || c.Discords[0].Webhook != "https://discord.com/api/webhooks/1/x" || c.Discords[0].Username != "SZ" || len(c.Discords[0].Events) != 2 {
		t.Fatalf("%+v", c.Discords)
	}
	if len(c.Forums) != 1 || c.Forums[0].Thread != 12 || c.Forums[0].Key != "k" {
		t.Fatalf("%+v", c.Forums)
	}
	if c.OldDiscord != nil || c.OldXenForo != nil || len(c.Upgraded) != 2 {
		t.Fatalf("%v %v %v", c.OldDiscord, c.OldXenForo, c.Upgraded)
	}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if strings.Contains(string(b), `"discord":`) || !strings.Contains(string(b), `"discords"`) {
		t.Fatal(string(b))
	}
	// an empty old one is just dropped
	_ = os.WriteFile(p, []byte(`{"discord":{"webhook":"","events":["crash"]}}`), 0o644)
	c, _ = Load(p)
	if len(c.Discords) != 0 || c.Discords == nil || len(c.Upgraded) != 0 {
		t.Fatalf("%+v %v", c.Discords, c.Upgraded)
	}
}

func TestWants(t *testing.T) {
	if !Wants([]string{"update", "updated"}, nil, "updated", "css1") || Wants([]string{"updated"}, nil, "crash", "css1") {
		t.Fatal("kinds")
	}
	if !Wants([]string{"crash"}, []string{"css1"}, "crash", "css1") || Wants([]string{"crash"}, []string{"css1"}, "crash", "tf2") ||
		Wants([]string{"error"}, []string{"css1"}, "error", "") {
		t.Fatal("servers")
	}
}
