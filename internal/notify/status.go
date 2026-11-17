package notify

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

/* ---------------------------------------------------------------------------------------------------------------
 * Status cards: a server's live status in a Discord channel
 *
 * One message per server, posted through the channel's webhook and then edited in place: online or not, the map,
 * the players, the address, and the players of the last 24 hours as a chart. A webhook can edit and delete the
 * messages it posted itself, so nothing but the webhook is needed: no bot to add to the Discord server.
 *
 * The game's icon and the chart go with the message as attachments (no site to show them from). An edit that
 * changes neither keeps them by their numbers, and sends no file.
 * ------------------------------------------------------------------------------------------------------------- */

// States of a card
const (
	CardOnline     = "online"
	CardStarting   = "starting"
	CardUpdating   = "updating"
	CardRestarting = "restarting"
	CardOffline    = "offline"
	CardUnknown    = "unknown" // its machine does not answer the hub
)

var cardLooks = map[string]struct {
	dot, word string
	color     int
}{
	CardOnline:     {"🟢", "Online", 0x34C77B},
	CardStarting:   {"🟡", "Starting", 0xF0B429},
	CardUpdating:   {"🟠", "Updating", 0xFF8A3D},
	CardRestarting: {"🟠", "Restarting", 0xFF8A3D},
	CardOffline:    {"🔴", "Offline", 0xE5383B},
	CardUnknown:    {"⚪", "No news", 0x7D88B0},
}

// Card is a server as its status card shows it.
type Card struct {
	Name     string // in the manager
	Hostname string // what players see: the card's title
	Game     string // Counter-Strike 2
	Address  string // where players reach it
	Icon     []byte // PNG of the game
	IconName string
	State    string
	Note     string    // a few words after the state
	Map      string    // while online
	Players  int       // people on, bots aside
	Max      int       // 0: not known
	Bots     int       //
	Locked   bool      // a password is needed
	Since    time.Time // of the state; zero: not known
	Join     string    // a web page that opens the game on the server ("" for none: Discord opens web links only)
	Chart    []byte    // PNG: the players of the last 24 hours (nil: none)
}

// chartName is the chart's file name in the message.
const chartName = "players.png"

// CardMessage is the message of a card in Discord: what is needed to edit it.
type CardMessage struct {
	ID    string            `json:"id"`
	Files map[string]string `json:"files,omitempty"` // its attachments: their numbers by file name
}

// ErrGone: the message of a card is no longer there (someone deleted it): it is posted again.
var ErrGone = errors.New("the message is gone")

// md keeps a text from being read as Discord's formatting.
var md = strings.NewReplacer("\\", "\\\\", "*", "\\*", "_", "\\_", "~", "\\~", "`", "\\`", "|", "\\|", ">", "\\>", "[", "\\[", "]", "\\]", "#", "\\#")

// code shows a text as code (its own font, no formatting).
func code(s string) string {
	s = strings.ReplaceAll(s, "`", "'")
	if strings.TrimSpace(s) == "" {
		return "`-`"
	}
	return "`" + s + "`"
}

// cardEmbed is the card, as Discord wants it. The time of its last change is left out: CardSum tells whether
// what it shows changed.
func cardEmbed(c Card) map[string]any {
	look, ok := cardLooks[c.State]
	if !ok {
		look = cardLooks[CardUnknown]
	}
	title := c.Hostname
	if title == "" {
		title = c.Name
	}
	state := look.dot + " **" + look.word + "**"
	if c.Locked && c.State == CardOnline {
		state += " · 🔒 password"
	}
	if c.Note != "" {
		state += " · " + md.Replace(c.Note)
	}
	if !c.Since.IsZero() {
		state += " · <t:" + strconv.FormatInt(c.Since.Unix(), 10) + ":R>" // "2 hours ago", kept right by Discord itself
	}
	embed := map[string]any{
		"title":       truncate(md.Replace(title), 250),
		"description": state,
		"color":       look.color,
	}
	author := map[string]any{"name": truncate(c.Game, 250)}
	if c.Game == "" {
		author["name"] = "Game server"
	}
	if len(c.Icon) > 0 && c.IconName != "" {
		author["icon_url"] = "attachment://" + c.IconName
	}
	embed["author"] = author
	var fields []map[string]any
	if c.State == CardOnline {
		if c.Map != "" {
			fields = append(fields, map[string]any{"name": "Map", "value": code(truncate(c.Map, 60)), "inline": true})
		}
		if c.Max > 0 || c.Players > 0 {
			p := strconv.Itoa(c.Players)
			if c.Max > 0 {
				p += " / " + strconv.Itoa(c.Max)
			}
			p = code(p)
			if c.Bots > 0 {
				p += fmt.Sprintf(" + %d bot", c.Bots)
				if c.Bots > 1 {
					p += "s"
				}
			}
			fields = append(fields, map[string]any{"name": "Players", "value": p, "inline": true})
		}
	}
	if c.Address != "" {
		a := code(c.Address)
		if c.Join != "" && c.State == CardOnline {
			a += "\n[Join via Steam](" + c.Join + ")"
		}
		fields = append(fields, map[string]any{"name": "Address", "value": a, "inline": true})
	}
	if len(fields) > 0 {
		embed["fields"] = fields
	}
	if len(c.Chart) > 0 {
		embed["image"] = map[string]any{"url": "attachment://" + chartName}
		embed["footer"] = map[string]any{"text": "Players, last 24 hours · updated"}
	} else {
		embed["footer"] = map[string]any{"text": "Updated"}
	}
	return embed
}

// CardSum tells what a card shows, the chart aside: two cards with the same sum need no edit.
func CardSum(c Card) string {
	c.Chart = []byte{1} // with or without a chart: the same words
	b, _ := json.Marshal(cardEmbed(c))
	h := sha1.Sum(b)
	return hex.EncodeToString(h[:8])
}

// ChartSum tells a chart from another.
func ChartSum(png []byte) string {
	h := sha1.Sum(png)
	return hex.EncodeToString(h[:8])
}

// hook is a webhook, as its messages are posted, edited and deleted.
type hook struct {
	base  string // https://discord.com/api/v10/webhooks/<id>/<token>
	query url.Values
}

// hookOf reads a webhook's address. The version of Discord's API is said (v10): the rules of an edit's attachments
// are those of that version.
func hookOf(webhook string) (hook, error) {
	if !Valid(webhook) {
		return hook{}, errors.New("not a Discord webhook")
	}
	u, err := url.Parse(strings.TrimSpace(webhook))
	if err != nil {
		return hook{}, errors.New("not a Discord webhook")
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for k, p := range parts {
		if p == "webhooks" && k+2 < len(parts) && parts[k+1] != "" && parts[k+2] != "" {
			h := hook{base: "https://" + u.Host + "/api/v10/webhooks/" + parts[k+1] + "/" + parts[k+2], query: url.Values{}}
			if testAPI != "" {
				h.base = strings.TrimRight(testAPI, "/") + "/api/v10/webhooks/" + parts[k+1] + "/" + parts[k+2]
			}
			if t := u.Query().Get("thread_id"); t != "" {
				h.query.Set("thread_id", t) // a webhook of a forum channel or a thread
			}
			return h, nil
		}
	}
	return hook{}, errors.New("the webhook's address is not complete (https://discord.com/api/webhooks/<number>/<token>)")
}

// testAPI: where the cards go instead of Discord, for the tests of the whole program (SZM_DISCORD_API=http://…).
var testAPI = os.Getenv("SZM_DISCORD_API")

// HookID is the number of a webhook ("" when the address is not one): it names the webhook without its secret.
func HookID(webhook string) string {
	h, err := hookOf(webhook)
	if err != nil {
		return ""
	}
	p := strings.Split(h.base, "/")
	return p[len(p)-2]
}

func (h hook) url(path string, wait bool) string {
	q := url.Values{}
	for k, v := range h.query {
		q[k] = v
	}
	if wait {
		q.Set("wait", "true") // the answer is the message: its number, its attachments
	}
	u := h.base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	return u
}

// PostCard posts a card as a new message.
func PostCard(webhook, username string, c Card) (CardMessage, error) {
	h, err := hookOf(webhook)
	if err != nil {
		return CardMessage{}, err
	}
	return h.post(username, c)
}

// EditCard changes the message of a card. Its icon stays; chart: the chart changed and is sent again (else the
// one of the message stays).
func EditCard(webhook string, msg CardMessage, c Card, chart bool) (CardMessage, error) {
	h, err := hookOf(webhook)
	if err != nil {
		return CardMessage{}, err
	}
	return h.edit(msg, c, chart)
}

// DeleteCard removes the message of a card. One that is gone already is no error.
func DeleteCard(webhook, id string) error {
	h, err := hookOf(webhook)
	if err != nil {
		return err
	}
	return h.delete(id)
}

type cardFile struct {
	name string
	data []byte
}

func (h hook) post(username string, c Card) (CardMessage, error) {
	payload := map[string]any{}
	if strings.TrimSpace(username) != "" {
		payload["username"] = username
	}
	var files []cardFile
	if len(c.Icon) > 0 && c.IconName != "" {
		files = append(files, cardFile{c.IconName, c.Icon})
	}
	if len(c.Chart) > 0 {
		files = append(files, cardFile{chartName, c.Chart})
	}
	return h.send(http.MethodPost, h.url("", true), payload, c, nil, files)
}

func (h hook) edit(msg CardMessage, c Card, chart bool) (CardMessage, error) {
	if msg.ID == "" {
		return CardMessage{}, ErrGone
	}
	// Every attachment the message keeps is named, by its number; a file sent again takes the place of the one
	// of the same name, which is not named.
	var keep []map[string]any
	var files []cardFile
	if len(c.Icon) > 0 && c.IconName != "" {
		if id := msg.Files[c.IconName]; id != "" {
			keep = append(keep, map[string]any{"id": id})
		} else {
			files = append(files, cardFile{c.IconName, c.Icon})
		}
	}
	if len(c.Chart) > 0 {
		if id := msg.Files[chartName]; id != "" && !chart {
			keep = append(keep, map[string]any{"id": id})
		} else {
			files = append(files, cardFile{chartName, c.Chart})
		}
	}
	return h.send(http.MethodPatch, h.url("/messages/"+msg.ID, false), map[string]any{}, c, keep, files)
}

func (h hook) send(method, addr string, payload map[string]any, c Card, keep []map[string]any, files []cardFile) (CardMessage, error) {
	embed := cardEmbed(c)
	embed["timestamp"] = time.Now().UTC().Format(time.RFC3339)
	payload["embeds"] = []map[string]any{embed}
	payload["allowed_mentions"] = map[string]any{"parse": []string{}}
	attachments := append([]map[string]any{}, keep...)
	for k, f := range files {
		attachments = append(attachments, map[string]any{"id": k, "filename": f.name})
	}
	payload["attachments"] = attachments
	b, _ := json.Marshal(payload)
	body, ctype := b, "application/json"
	if len(files) > 0 {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		_ = mw.WriteField("payload_json", string(b))
		for k, f := range files {
			hd := textproto.MIMEHeader{}
			hd.Set("Content-Disposition", fmt.Sprintf(`form-data; name="files[%d]"; filename="%s"`, k, f.name))
			hd.Set("Content-Type", "image/png")
			if w, err := mw.CreatePart(hd); err == nil {
				_, _ = w.Write(f.data)
			}
		}
		_ = mw.Close()
		body, ctype = buf.Bytes(), mw.FormDataContentType()
	}
	resp, err := do(method, addr, body, ctype)
	if err != nil {
		return CardMessage{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound && method == http.MethodPatch {
		return CardMessage{}, ErrGone
	}
	if resp.StatusCode >= 300 {
		return CardMessage{}, discordErr(resp)
	}
	var m struct {
		ID          string `json:"id"`
		Attachments []struct {
			ID       string `json:"id"`
			Filename string `json:"filename"`
		} `json:"attachments"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&m); err != nil || m.ID == "" {
		return CardMessage{}, errors.New("Discord's answer does not say which message it is")
	}
	out := CardMessage{ID: m.ID, Files: map[string]string{}}
	for _, a := range m.Attachments {
		out.Files[a.Filename] = a.ID
	}
	return out, nil
}

func (h hook) delete(id string) error {
	if id == "" {
		return nil
	}
	resp, err := do(http.MethodDelete, h.url("/messages/"+id, false), nil, "")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode != http.StatusNotFound {
		return discordErr(resp)
	}
	return nil
}

// do asks Discord. Told to slow down (429), it waits as long as Discord says (a few seconds at most) and asks once
// more.
func do(method, addr string, body []byte, ctype string) (*http.Response, error) {
	for try := 0; ; try++ {
		req, err := http.NewRequest(method, addr, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		if ctype != "" {
			req.Header.Set("Content-Type", ctype)
		}
		resp, err := client.Do(req)
		if err != nil {
			// the address has the webhook's secret: not in the error
			var ue *url.Error
			if errors.As(err, &ue) {
				err = ue.Err
			}
			return nil, fmt.Errorf("Discord can not be reached: %v", err)
		}
		if resp.StatusCode != http.StatusTooManyRequests || try > 0 {
			return resp, nil
		}
		wait, _ := strconv.ParseFloat(resp.Header.Get("Retry-After"), 64)
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		resp.Body.Close()
		if wait <= 0 {
			wait = 1
		}
		time.Sleep(time.Duration(min(wait, maxSlowDown) * float64(time.Second)))
	}
}

// maxSlowDown: the longest wait before asking again, in seconds (the tests shorten it).
var maxSlowDown = 10.0

// discordErr tells why Discord said no, in its words when it gives some.
func discordErr(resp *http.Response) error {
	var m struct {
		Message string `json:"message"`
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if json.Unmarshal(b, &m) == nil && m.Message != "" {
		return fmt.Errorf("Discord answered %s: %s", resp.Status, truncate(m.Message, 200))
	}
	return fmt.Errorf("Discord answered %s", resp.Status)
}
