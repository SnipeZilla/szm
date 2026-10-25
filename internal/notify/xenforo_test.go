package notify

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/SnipeZilla/szm/internal/events"
)

func TestPostForum(t *testing.T) {
	type got struct{ path, key, user, thread, node, title, message, prefix, attach string }
	var last got
	var attachCtx, attachBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/forum/api/attachments/new-key" {
			_ = r.ParseForm()
		}
		last = got{r.URL.Path, r.Header.Get("XF-Api-Key"), r.Header.Get("XF-Api-User"), r.PostForm.Get("thread_id"),
			r.PostForm.Get("node_id"), r.PostForm.Get("title"), r.PostForm.Get("message"), r.PostForm.Get("prefix_id"), ""}
		switch r.URL.Path {
		case "/forum/api/threads/12/":
			w.Write([]byte(`{"thread":{"title":"Server status"}}`))
		case "/forum/api/me/":
			w.Write([]byte(`{"me":{"username":"SZM Bot"}}`))
		case "/forum/api/posts/", "/forum/api/threads/":
			last.attach = r.PostForm.Get("attachment_key")
			w.Write([]byte(`{"success":true}`))
		case "/forum/api/attachments/new-key":
			if r.Header.Get("XF-Api-Key") == "noattach" {
				w.WriteHeader(403)
				w.Write([]byte(`{"errors":[{"message":"The API key provided does not have the required scope."}]}`))
				return
			}
			attachCtx = r.FormValue("context[thread_id]") + r.FormValue("context[node_id]")
			if f, _, err := r.FormFile("attachment"); err == nil {
				b := make([]byte, 4)
				f.Read(b)
				attachBody = string(b)
			}
			w.Write([]byte(`{"key":"abc123","attachment":{"attachment_id":77}}`))
		default:
			w.WriteHeader(404)
			w.Write([]byte(`{"errors":[{"message":"Requested page could not be found."}]}`))
		}
	}))
	defer srv.Close()

	e := events.Event{Time: time.Now(), Kind: events.Crash, Server: "x", Name: "CS2 - Server 1", Text: "Crashed [b]oops[/b]."}
	sv := &Server{Name: "CS2 - Server 1", Hostname: "SnipeZilla | Dust2", Game: "Counter-Strike 2", Address: "1.2.3.4:27015"}

	f := Forum{URL: srv.URL + "/forum/index.php", Key: "k", User: 1, Thread: 12}
	if err := PostForum(f, e, sv); err != nil {
		t.Fatal(err)
	}
	if last.path != "/forum/api/posts/" || last.key != "k" || last.user != "1" || last.thread != "12" {
		t.Errorf("reply: %+v", last)
	}
	if strings.Contains(last.message, "CS2 - Server 1") {
		t.Errorf("the manager's name with a hostname:\n%s", last.message)
	}
	for _, want := range []string{"[B][PLAIN]SnipeZilla | Dust2[/PLAIN][/B]\n[COLOR=#ff5c6c][B]↳ Crashed[/B][/COLOR]\n",
		"[PLAIN]Crashed [b]oops[/b].[/PLAIN]", "Counter-Strike 2 · 1.2.3.4:27015"} {
		if !strings.Contains(last.message, want) {
			t.Errorf("message lacks %q:\n%s", want, last.message)
		}
	}

	f = Forum{URL: srv.URL + "/forum", Key: "k", Node: 7, Prefix: 3}
	if err := PostForum(f, e, sv); err != nil {
		t.Fatal(err)
	}
	if last.path != "/forum/api/threads/" || last.node != "7" || last.prefix != "3" || last.title != "Crashed · SnipeZilla | Dust2" {
		t.Errorf("thread: %+v", last)
	}

	msg, err := CheckForum(Forum{URL: srv.URL + "/forum/", Key: "k", Thread: 12}, "cs2.png", []byte("\x89PNG...."))
	if err != nil || msg != `Posts go as replies in the thread "Server status", posted by SZM Bot. The game icon can be attached (a test upload worked: attachment 77, never posted).` {
		t.Errorf("check: %q %v", msg, err)
	}
	if _, err := CheckForum(Forum{URL: srv.URL + "/forum/", Key: "k", Node: 99}, "", nil); err == nil || !strings.Contains(err.Error(), "could not be found") {
		t.Errorf("check missing forum: %v", err)
	}

	// The game icon: attached...
	sv.Icon, sv.IconName = []byte("\x89PNG...."), "cs2.png"
	f = Forum{URL: srv.URL + "/forum", Key: "k", Thread: 12}
	if err := PostForum(f, e, sv); err != nil {
		t.Fatal(err)
	}
	if last.attach != "abc123" || attachCtx != "12" || attachBody != "\x89PNG" ||
		!strings.HasPrefix(last.message, "[ATTACH=full]77[/ATTACH] [B][PLAIN]SnipeZilla | Dust2[/PLAIN][/B]\n") {
		t.Errorf("attached icon: %+v %q %q", last, attachCtx, attachBody)
	}
	// ...or from its address
	f.Icons = "https://snipezilla.com/icons"
	if err := PostForum(f, e, sv); err != nil {
		t.Fatal(err)
	}
	if last.attach != "" || !strings.HasPrefix(last.message, `[IMG alt="cs2"]https://snipezilla.com/icons/cs2.png[/IMG] [B]`) {
		t.Errorf("icon by address: %+v", last)
	}
	// ...or left out when the key may not attach: the post still goes
	f = Forum{URL: srv.URL + "/forum", Key: "noattach", Thread: 12}
	if err := PostForum(f, e, sv); err != nil {
		t.Fatal(err)
	}
	if last.attach != "" || strings.Contains(last.message, "ATTACH") || last.path != "/forum/api/posts/" {
		t.Errorf("no icon: %+v", last)
	}
	if msg, _ := CheckForum(f, "cs2.png", []byte("\x89PNG....")); !strings.Contains(msg, "cannot be attached") {
		t.Errorf("check: %q", msg)
	}
}
