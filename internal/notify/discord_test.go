package notify

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/SnipeZilla/szm/internal/events"
)

func TestSendWithIcon(t *testing.T) {
	var payload map[string]any
	var file []byte
	var fileName string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("multipart: %v", err)
			return
		}
		_ = json.Unmarshal([]byte(r.FormValue("payload_json")), &payload)
		f, h, err := r.FormFile("files[0]")
		if err == nil {
			file, _ = io.ReadAll(f)
			fileName = h.Filename
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	e := events.Event{Time: time.Now(), Kind: events.Crash, Server: "x", Name: "CS2 - Server 1", Text: "Crashed."}
	sv := &Server{Name: "CS2 - Server 1", Hostname: "SnipeZilla | Dust2", Game: "Counter-Strike 2", Address: "1.2.3.4:27015",
		Icon: []byte("\x89PNG fake"), IconName: "cs2.png"}
	if err := send(srv.URL, "SZM", e, sv); err != nil {
		t.Fatal(err)
	}
	embed := payload["embeds"].([]any)[0].(map[string]any)
	author := embed["author"].(map[string]any)
	if author["name"] != "SnipeZilla | Dust2" || author["icon_url"] != "attachment://cs2.png" || embed["title"] != "↳ Crashed" {
		t.Errorf("author %v", author)
	}
	if embed["footer"].(map[string]any)["text"] != "Counter-Strike 2 · 1.2.3.4:27015" {
		t.Errorf("embed %v", embed)
	}
	if fileName != "cs2.png" || string(file) != "\x89PNG fake" {
		t.Errorf("file %q %q", fileName, file)
	}
}

func TestSendNoHostname(t *testing.T) {
	var payload map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(1 << 20)
		_ = json.Unmarshal([]byte(r.FormValue("payload_json")), &payload)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	e := events.Event{Time: time.Now(), Kind: events.Start, Server: "x", Name: "TF2 - Server 3", Text: "Up."}
	sv := &Server{Name: "TF2 - Server 3", Game: "Team Fortress 2", Icon: []byte("png"), IconName: "tf.png"}
	if err := send(srv.URL, "SZM", e, sv); err != nil {
		t.Fatal(err)
	}
	embed := payload["embeds"].([]any)[0].(map[string]any)
	if embed["author"].(map[string]any)["name"] != "TF2 - Server 3" || embed["title"] != "↳ Started" {
		t.Errorf("embed %v", embed)
	}
}
