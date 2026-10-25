package notify

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/SnipeZilla/szm/internal/events"
)

// Forum is where XenForo posts go: its REST API, as HLstatsZ uses it (includes/match_announce.php).
type Forum struct {
	URL    string // the forum's address
	Key    string // XF-Api-Key
	User   int    // XF-Api-User, for a super user key
	Thread int    // replies of this thread...
	Node   int    // ...or a new thread of this forum each time
	Prefix int    // prefix of those threads
	Icons  string // where the game icons are on the web (https://example.com/icons/, with cs2.png...): shown from
	// there. Empty: each post carries its icon as an attachment (the key needs the scope attachment:write)
}

// Ready tells whether there is enough to post.
func (f Forum) Ready() bool {
	return strings.TrimSpace(f.URL) != "" && strings.TrimSpace(f.Key) != "" && (f.Thread > 0 || f.Node > 0)
}

var indexRe = regexp.MustCompile(`/(index\.php)?$`)

// api is the address of the REST API, from the forum's ("https://example.com/forum" gives ".../forum/api/").
func (f Forum) api() string {
	base := strings.TrimRight(indexRe.ReplaceAllString(strings.TrimSpace(f.URL), ""), "/")
	if strings.HasSuffix(base, "/api") {
		return base + "/"
	}
	return base + "/api/"
}

// ValidURL tells whether a forum address can be used.
func ValidURL(u string) bool {
	p, err := url.Parse(strings.TrimSpace(u))
	return err == nil && (p.Scheme == "https" || p.Scheme == "http") && p.Host != ""
}

func (f Forum) call(method, path string, form url.Values) (map[string]any, error) {
	var body io.Reader
	ctype := ""
	if form != nil {
		body, ctype = strings.NewReader(form.Encode()), "application/x-www-form-urlencoded"
	}
	return f.do(method, path, body, ctype)
}

func (f Forum) do(method, path string, body io.Reader, ctype string) (map[string]any, error) {
	req, err := http.NewRequest(method, f.api()+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("XF-Api-Key", strings.TrimSpace(f.Key))
	if f.User > 0 {
		req.Header.Set("XF-Api-User", strconv.Itoa(f.User))
	}
	req.Header.Set("User-Agent", "SnipeZilla Server Manager")
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var data map[string]any
	_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&data)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := fmt.Sprintf("the forum answered %s", resp.Status)
		if errs, ok := data["errors"].([]any); ok && len(errs) > 0 {
			if e, ok := errs[0].(map[string]any); ok {
				if m, ok := e["message"].(string); ok && m != "" {
					msg += ": " + m
				}
			}
		}
		return nil, errors.New(msg)
	}
	return data, nil
}

// bb keeps a text from being read as BBCode.
func bb(s string) string {
	if s == "" {
		return ""
	}
	return "[PLAIN]" + strings.ReplaceAll(s, "[/PLAIN]", "[/ PLAIN]") + "[/PLAIN]"
}

// shownName is how a post names the server: its hostname, else its name in the manager.
func shownName(e events.Event, sv *Server) string {
	if sv == nil {
		return e.Name
	}
	if sv.Hostname != "" {
		return sv.Hostname
	}
	return sv.Name
}

// forumTitle is the title of a new thread: "Crashed · SnipeZilla | Dust2".
func forumTitle(e events.Event, sv *Server) string {
	name := shownName(e, sv)
	t := Title(e.Kind)
	if name != "" {
		t += " · " + name
	}
	return truncate(t, 150)
}

// forumMessage is the post, in BBCode: the event in its color and the server (its hostname, else its name), the
// text, then the game and the address.
// icon is the BBCode of the game's icon, put before the server's name ("" for none).
func forumMessage(e events.Event, sv *Server, withHead bool, icon string) string {
	var b strings.Builder
	if withHead {
		// The game's icon and the server (its hostname, else its name), then the event in its color, as on Discord
		name := shownName(e, sv)
		switch {
		case icon != "" && name != "":
			fmt.Fprintf(&b, "%s [B]%s[/B]\n", icon, bb(name))
		case icon != "":
			b.WriteString(icon + "\n")
		case name != "":
			fmt.Fprintf(&b, "[B]%s[/B]\n", bb(name))
		}
		fmt.Fprintf(&b, "[COLOR=%s][B]↳ %s[/B][/COLOR]\n", Color(e.Kind), Title(e.Kind))
	}
	b.WriteString(bb(e.Text))
	if sv != nil {
		if foot := strings.Trim(sv.Game+" · "+sv.Address, " ·"); foot != "" {
			fmt.Fprintf(&b, "\n[SIZE=2][COLOR=#8a8a8a]%s · %s[/COLOR][/SIZE]", bb(foot), e.Time.Format("2006-01-02 15:04"))
		}
	}
	return b.String()
}

// PostForum posts an event on the forum: a reply of the thread, or a new thread.
func PostForum(f Forum, e events.Event, sv *Server) error {
	if !f.Ready() {
		return errors.New("the forum address, the API key and a thread or forum ID are needed")
	}
	icon, key := f.icon(sv)
	if f.Thread > 0 {
		form := url.Values{
			"thread_id": {strconv.Itoa(f.Thread)},
			"message":   {forumMessage(e, sv, true, icon)},
		}
		if key != "" {
			form.Set("attachment_key", key)
		}
		_, err := f.call("POST", "posts/", form)
		return err
	}
	form := url.Values{
		"node_id":         {strconv.Itoa(f.Node)},
		"title":           {forumTitle(e, sv)},
		"message":         {forumMessage(e, sv, true, icon)},
		"discussion_open": {"1"},
	}
	if key != "" {
		form.Set("attachment_key", key)
	}
	if f.Prefix > 0 {
		form.Set("prefix_id", strconv.Itoa(f.Prefix))
	}
	_, err := f.call("POST", "threads/", form)
	return err
}

// CheckForum checks the settings without posting: where the posts go, who posts them, and whether the game icon
// can go with them (icon: a PNG to try, named iconName; it is uploaded but never posted, and the forum drops it).
func CheckForum(f Forum, iconName string, icon []byte) (string, error) {
	if !f.Ready() {
		return "", errors.New("the forum address, the API key and a thread or forum ID are needed")
	}
	var where string
	if f.Thread > 0 {
		data, err := f.call("GET", "threads/"+strconv.Itoa(f.Thread)+"/", nil)
		if err != nil {
			return "", fmt.Errorf("%v (%s)", err, f.api())
		}
		t, _ := data["thread"].(map[string]any)
		title, _ := t["title"].(string)
		where = fmt.Sprintf("replies in the thread \"%s\"", title)
	} else {
		data, err := f.call("GET", "forums/"+strconv.Itoa(f.Node)+"/", nil)
		if err != nil {
			return "", fmt.Errorf("%v (%s)", err, f.api())
		}
		fo, _ := data["forum"].(map[string]any)
		title, _ := fo["title"].(string)
		where = fmt.Sprintf("a new thread in the forum \"%s\" each time", title)
	}
	msg := "Posts go as " + where
	if me, err := f.call("GET", "me/", nil); err == nil {
		if m, ok := me["me"].(map[string]any); ok {
			if name, _ := m["username"].(string); name != "" {
				msg += ", posted by " + name
			}
		}
	}
	msg += "."
	// The game icons: from their address (is the picture there?), or attached (a real upload, never posted)
	if base := strings.TrimSpace(f.Icons); base != "" {
		if !strings.HasSuffix(base, "/") {
			base += "/"
		}
		u := base + url.PathEscape(iconName)
		if iconName == "" {
			msg += " Game icons from " + base + "."
		} else if resp, err := client.Get(u); err != nil {
			msg += " Game icons: " + u + " cannot be read (" + err.Error() + ")."
		} else {
			resp.Body.Close()
			if resp.StatusCode == 200 && strings.HasPrefix(resp.Header.Get("Content-Type"), "image/") {
				msg += " Game icons from " + base + " (" + iconName + " is there)."
			} else {
				msg += " Game icons: " + u + " answers " + resp.Status + ", not a picture: copy the icons there (cs2.png, css.png…), or empty the address to attach them."
			}
		}
	} else if len(icon) == 0 {
		msg += " No game icon to try."
	} else if _, id, err := f.attach(iconName, icon); err != nil {
		msg += " The game icon cannot be attached: " + err.Error() + ". Give the API key the scope attachment:write, and the posting user the right to upload attachments in that forum; or set the icons' address."
	} else {
		msg += fmt.Sprintf(" The game icon can be attached (a test upload worked: attachment %d, never posted).", id)
	}
	return msg, nil
}

// icon gives the BBCode of the game's icon for a post and, when it is attached, the attachment key the post must
// give. From the icons' address when one is set; otherwise uploaded as an attachment. No icon ("") when the game has
// none, or the forum refuses the attachment (the key lacks attachment:write): the post goes without it.
func (f Forum) icon(sv *Server) (bbcode, key string) {
	if sv == nil || sv.IconName == "" {
		return "", ""
	}
	if base := strings.TrimSpace(f.Icons); base != "" {
		if !strings.HasSuffix(base, "/") {
			base += "/"
		}
		return "[IMG alt=\"" + strings.TrimSuffix(sv.IconName, ".png") + "\"]" + base + url.PathEscape(sv.IconName) + "[/IMG]", ""
	}
	if len(sv.Icon) == 0 {
		return "", ""
	}
	k, id, err := f.attach(sv.IconName, sv.Icon)
	if err != nil {
		iconErr(f, err)
		return "", ""
	}
	return "[ATTACH=full]" + strconv.Itoa(id) + "[/ATTACH]", k
}

// attach uploads a file for the next post: its attachment key and ID.
func (f Forum) attach(name string, data []byte) (string, int, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("type", "post")
	if f.Thread > 0 {
		_ = w.WriteField("context[thread_id]", strconv.Itoa(f.Thread))
	} else {
		_ = w.WriteField("context[node_id]", strconv.Itoa(f.Node))
	}
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", `form-data; name="attachment"; filename="`+strings.ReplaceAll(name, `"`, "")+`"`)
	h.Set("Content-Type", "image/png")
	part, err := w.CreatePart(h)
	if err != nil {
		return "", 0, err
	}
	_, _ = part.Write(data)
	_ = w.Close()
	res, err := f.do("POST", "attachments/new-key", &buf, w.FormDataContentType())
	if err != nil {
		return "", 0, err
	}
	key, _ := res["key"].(string)
	a, _ := res["attachment"].(map[string]any)
	id, _ := a["attachment_id"].(float64)
	if key == "" || id <= 0 {
		return "", 0, errors.New("the forum gave no attachment")
	}
	return key, int(id), nil
}

// OnIconError, when set, is told why a forum's posts go without the game icon (once per forum and reason): the
// manager shows it in Activity.
var OnIconError func(f Forum, err error)

var iconErrs sync.Map

func iconErr(f Forum, err error) {
	if _, seen := iconErrs.LoadOrStore(f.URL+"|"+strconv.Itoa(f.Thread)+"|"+strconv.Itoa(f.Node)+"|"+err.Error(), true); !seen {
		fmt.Fprintln(os.Stderr, "xenforo: the game icon is left out of the posts:", err)
		if OnIconError != nil {
			OnIconError(f, err)
		}
	}
}
