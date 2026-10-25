// Package notify posts the events the admin chose to a Discord channel (a webhook).
package notify

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"time"

	"github.com/SnipeZilla/szm/internal/events"
)

// Colors of the events, the same in Discord, XenForo and the Activity page
var colors = map[string]int{
	events.Crash:    0xFF5C6C, // red
	events.Failed:   0xC9283E, // darker red: gave up
	events.Error:    0xFF5C6C, // red
	events.Stop:     0xE5383B, // the red of the Z
	events.Update:   0xF0B429, // amber: an update was found
	events.Restart:  0xFF8A3D, // orange
	events.SteamCmd: 0xFF8A3D, // orange: steamcmd at work
	events.Updated:  0x2EC4B6, // teal
	events.Start:    0x34C77B, // green: up and running
	events.Info:     0x4FA3FF, // blue
	events.Manager:  0x7D88B0, // grey-blue
	events.Addons:   0xA78BFA, // violet
}

// Color of an event kind, as #rrggbb (XenForo BBCode)
func Color(kind string) string {
	c, ok := colors[kind]
	if !ok {
		c = colors[events.Info]
	}
	return fmt.Sprintf("#%06x", c)
}

var titles = map[string]string{
	events.Crash:   "Crashed",
	events.Failed:  "Gave up",
	events.Error:   "Error",
	events.Update:  "Update found",
	events.Updated: "Updated",
	events.Restart: "Restart",
	events.Start:   "Started",
	events.Stop:    "Stopped",
	events.Info:    "Info",
	events.Manager: "Manager",
	events.Addons:  "Addons",
}

var client = &http.Client{Timeout: 10 * time.Second}

// Valid tells whether a URL is a Discord webhook.
func Valid(url string) bool {
	return strings.HasPrefix(url, "https://discord.com/api/webhooks/") || strings.HasPrefix(url, "https://discordapp.com/api/webhooks/") ||
		strings.HasPrefix(url, "https://ptb.discord.com/api/webhooks/") || strings.HasPrefix(url, "https://canary.discord.com/api/webhooks/")
}

// Server is what a post says of the server an event is about.
type Server struct {
	Name     string // in the manager (CS2 - Server 1)
	Hostname string // what players see (SnipeZilla | Dust2 24/7)
	Game     string // Counter-Strike 2
	Address  string
	Icon     []byte // PNG of the game, sent with the post
	IconName string // its file name (cs2.png)
}

// Send posts an event to a webhook. sv, when the event is about a server, adds its names and the game's icon.
func Send(webhook, username string, e events.Event, sv *Server) error {
	if !Valid(webhook) {
		return errors.New("not a Discord webhook")
	}
	return send(webhook, username, e, sv)
}

func send(webhook, username string, e events.Event, sv *Server) error {
	title := titles[e.Kind]
	if title == "" {
		title = e.Kind
	}
	embed := map[string]any{
		"description": truncate(e.Text, 4000),
		"color":       colorOf(e.Kind),
		"timestamp":   e.Time.UTC().Format(time.RFC3339),
	}
	icon := ""
	switch {
	case sv != nil:
		// The game's icon and the server (the hostname players know, else its name in the manager), then the event
		name := sv.Hostname
		if name == "" {
			name = sv.Name
		}
		author := map[string]any{"name": truncate(name, 250)}
		embed["title"] = truncate("↳ "+title, 250) // what happened to the server above
		if len(sv.Icon) > 0 && sv.IconName != "" {
			icon = sv.IconName
			author["icon_url"] = "attachment://" + icon
		}
		embed["author"] = author
		if foot := strings.Trim(sv.Game+" · "+sv.Address, " ·"); foot != "" {
			embed["footer"] = map[string]any{"text": truncate(foot, 2000)}
		}
	case e.Name != "":
		embed["title"] = truncate(title+": "+e.Name, 250)
	default:
		embed["title"] = truncate(title, 250)
	}
	payload := map[string]any{
		"allowed_mentions": map[string]any{"parse": []string{}},
		"embeds":           []map[string]any{embed},
	}
	if strings.TrimSpace(username) != "" {
		payload["username"] = username // else the name the webhook has in Discord
	}
	b, _ := json.Marshal(payload)

	var resp *http.Response
	var err error
	if icon == "" {
		resp, err = client.Post(webhook, "application/json", bytes.NewReader(b))
	} else {
		// The icon goes with the post (Discord shows "attachment://" files): no public address needed
		payload["attachments"] = []map[string]any{{"id": 0, "filename": icon}}
		b, _ = json.Marshal(payload)
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		_ = mw.WriteField("payload_json", string(b))
		h := textproto.MIMEHeader{}
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="files[0]"; filename="%s"`, icon))
		h.Set("Content-Type", "image/png")
		if w, werr := mw.CreatePart(h); werr == nil {
			_, _ = w.Write(sv.Icon)
		}
		_ = mw.Close()
		resp, err = client.Post(webhook, mw.FormDataContentType(), &body)
	}
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("Discord answered %s", resp.Status)
	}
	return nil
}

func colorOf(kind string) int {
	if c, ok := colors[kind]; ok {
		return c
	}
	return colors[events.Info]
}

// Title of an event kind (Crashed, Gave up...)
func Title(kind string) string {
	if t := titles[kind]; t != "" {
		return t
	}
	return kind
}

func truncate(s string, n int) string {
	if len([]rune(s)) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}
