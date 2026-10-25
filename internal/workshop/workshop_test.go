package workshop

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseID(t *testing.T) {
	for in, want := range map[string]string{
		"3070284539": "3070284539",
		" https://steamcommunity.com/sharedfiles/filedetails/?id=3070284539&searchtext= ": "3070284539",
		"https://steamcommunity.com/workshop/filedetails/?l=french&id=123456789":          "123456789",
		"de_dust2": "", "12": "",
	} {
		if got := ParseID(in); got != want {
			t.Errorf("%q: %q", in, got)
		}
	}
}

func TestDetailsAndCollection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch r.URL.Path {
		case "/ISteamRemoteStorage/GetPublishedFileDetails/v1/":
			if r.PostForm.Get("itemcount") != "2" || r.PostForm.Get("publishedfileids[1]") != "222222" {
				w.WriteHeader(400)
				return
			}
			w.Write([]byte(`{"response":{"result":1,"publishedfiledetails":[
{"publishedfileid":"111111","result":1,"title":"Mirage Remake","preview_url":"https://img/1.jpg","file_size":"52428800","time_updated":1700000000,"consumer_app_id":730},
{"publishedfileid":"222222","result":9}]}}`))
		case "/ISteamRemoteStorage/GetCollectionDetails/v1/":
			if r.PostForm.Get("publishedfileids[0]") == "999999" {
				w.Write([]byte(`{"response":{"collectiondetails":[{"publishedfileid":"999999","result":1,"children":[{"publishedfileid":"111111","sortorder":1,"filetype":0},{"publishedfileid":"555555","sortorder":2,"filetype":2}]}]}}`))
				return
			}
			w.Write([]byte(`{"response":{"collectiondetails":[{"publishedfileid":"111111","result":1}]}}`))
		}
	}))
	defer srv.Close()
	API = srv.URL
	items, err := Details([]string{"111111", "222222"})
	if err != nil || len(items) != 1 || items[0].Title != "Mirage Remake" || items[0].Size != 52428800 || items[0].AppID != 730 {
		t.Fatalf("%v %+v", err, items)
	}
	ch, ok, err := Collection("999999")
	if err != nil || !ok || len(ch) != 1 || ch[0] != "111111" {
		t.Fatalf("%v %v %v", ch, ok, err)
	}
	if _, ok, _ := Collection("111111"); ok {
		t.Fatal("a map taken for a collection")
	}
}
