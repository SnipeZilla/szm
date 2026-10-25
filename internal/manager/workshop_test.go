package manager

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/SnipeZilla/szm/internal/config"
	"github.com/SnipeZilla/szm/internal/games"
	"github.com/SnipeZilla/szm/internal/workshop"
)

func TestWorkshopList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.URL.Path == "/ISteamRemoteStorage/GetCollectionDetails/v1/" {
			if r.PostForm.Get("publishedfileids[0]") == "900000" {
				w.Write([]byte(`{"response":{"collectiondetails":[{"result":1,"children":[{"publishedfileid":"111111","filetype":0},{"publishedfileid":"222222","filetype":0}]}]}}`))
				return
			}
			w.Write([]byte(`{"response":{"collectiondetails":[{"result":1}]}}`))
			return
		}
		ids := r.PostForm["publishedfileids[0]"]
		body := `{"response":{"publishedfiledetails":[`
		switch {
		case len(ids) == 1 && ids[0] == "900000":
			body += `{"publishedfileid":"900000","result":1,"title":"Pack","consumer_app_id":730}`
		case len(ids) == 1 && ids[0] == "333333":
			body += `{"publishedfileid":"333333","result":1,"title":"A TF2 thing","consumer_app_id":440}`
		case len(ids) == 1 && ids[0] == "111111" && r.PostForm.Get("itemcount") == "1":
			body += `{"publishedfileid":"111111","result":1,"title":"Mirage","consumer_app_id":730}`
		default:
			body += `{"publishedfileid":"111111","result":1,"title":"Mirage","consumer_app_id":730},{"publishedfileid":"222222","result":1,"title":"Inferno","consumer_app_id":730}`
		}
		w.Write([]byte(body + `]}}`))
	}))
	defer srv.Close()
	workshop.API = srv.URL

	dir := t.TempDir()
	cfg, err := config.Load(filepath.Join(dir, "szm.json"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Servers = []*config.Server{{ID: "a", Name: "CS2", Game: "cs2", InstallDir: dir}, {ID: "b", Name: "CSS", Game: "css", InstallDir: dir}}
	cat, _ := games.Load("")
	m := New(cfg, cat, dir)

	r, err := m.AddWorkshop("a", "https://steamcommunity.com/sharedfiles/filedetails/?id=111111", "me")
	if err != nil || len(r.Added) != 1 || r.Collection != nil {
		t.Fatalf("%v %+v", err, r)
	}
	r, err = m.AddWorkshop("a", "900000", "me")
	if err != nil || len(r.Added) != 1 || r.Already != 1 || r.Collection == nil || r.Collection.Title != "Pack" {
		t.Fatalf("%v %+v", err, r)
	}
	if _, err := m.AddWorkshop("a", "333333", "me"); err == nil {
		t.Fatal("a TF2 item added to CS2")
	}
	if _, err := m.AddWorkshop("b", "111111", "me"); err == nil {
		t.Fatal("Workshop maps on CS:S")
	}
	if err := m.SetWorkshopStart("a", "222222", "me"); err != nil {
		t.Fatal(err)
	}
	if title, err := m.SetWorkshopCollection("a", "900000", "me"); err != nil || title != "Pack (2 maps)" {
		t.Fatal(title, err)
	}
	if _, err := m.SetWorkshopCollection("a", "111111", "me"); err == nil {
		t.Fatal("a map taken as a collection")
	}
	_ = m.RemoveWorkshop("a", "222222", "me")
	s := m.Cfg.Server("a")
	if len(s.Workshop) != 1 || s.WorkshopStart != "" || s.WorkshopColl != "900000" {
		t.Fatalf("%+v", s)
	}
}
