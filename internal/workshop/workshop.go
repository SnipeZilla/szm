// Package workshop reads Steam Workshop items (CS2 maps and collections): their names, pictures and sizes, from
// Steam's public web API (no key needed).
package workshop

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// API is Steam's web API (changed by the tests, or by SZM_STEAM_API).
var API = func() string {
	if v := os.Getenv("SZM_STEAM_API"); v != "" {
		return v
	}
	return "https://api.steampowered.com"
}()

var client = &http.Client{Timeout: 15 * time.Second}

// Item is a Workshop item.
type Item struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Preview string `json:"preview,omitempty"` // its picture
	Size    int64  `json:"size,omitempty"`    // bytes
	Updated int64  `json:"updated,omitempty"` // unix seconds
	AppID   int    `json:"app,omitempty"`     // the game it is for (730: CS2)
}

var idRe = regexp.MustCompile(`^\d{5,20}$`)
var urlIDRe = regexp.MustCompile(`[?&]id=(\d{5,20})`)

// ParseID reads a Workshop ID from what was typed: the number, or the item's address
// (https://steamcommunity.com/sharedfiles/filedetails/?id=3070284539). "" when there is none.
func ParseID(s string) string {
	s = strings.TrimSpace(s)
	if idRe.MatchString(s) {
		return s
	}
	if m := urlIDRe.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return ""
}

func post(path string, form url.Values, out any) error {
	resp, err := client.PostForm(API+path, form)
	if err != nil {
		return fmt.Errorf("Steam does not answer: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("Steam answered %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// Details gives the items (in the order asked). An ID Steam does not know is left out.
func Details(ids []string) ([]Item, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	form := url.Values{"itemcount": {strconv.Itoa(len(ids))}}
	for k, id := range ids {
		form.Set(fmt.Sprintf("publishedfileids[%d]", k), id)
	}
	var r struct {
		Response struct {
			Details []struct {
				ID       string `json:"publishedfileid"`
				Result   int    `json:"result"`
				Title    string `json:"title"`
				Preview  string `json:"preview_url"`
				Size     any    `json:"file_size"` // a number or a string, depending on the item
				Updated  int64  `json:"time_updated"`
				Consumer int    `json:"consumer_app_id"`
			} `json:"publishedfiledetails"`
		} `json:"response"`
	}
	if err := post("/ISteamRemoteStorage/GetPublishedFileDetails/v1/", form, &r); err != nil {
		return nil, err
	}
	var out []Item
	for _, d := range r.Response.Details {
		if d.Result != 1 {
			continue
		}
		it := Item{ID: d.ID, Title: strings.TrimSpace(d.Title), Preview: d.Preview, Updated: d.Updated, AppID: d.Consumer}
		switch v := d.Size.(type) {
		case float64:
			it.Size = int64(v)
		case string:
			it.Size, _ = strconv.ParseInt(v, 10, 64)
		}
		if it.Title == "" {
			it.Title = d.ID
		}
		out = append(out, it)
	}
	return out, nil
}

// Collection gives the items of a collection; ok is false when the ID is not a collection.
func Collection(id string) (children []string, ok bool, err error) {
	var r struct {
		Response struct {
			Details []struct {
				Result   int `json:"result"`
				Children []struct {
					ID       string `json:"publishedfileid"`
					Order    int    `json:"sortorder"`
					FileType int    `json:"filetype"`
				} `json:"children"`
			} `json:"collectiondetails"`
		} `json:"response"`
	}
	if err := post("/ISteamRemoteStorage/GetCollectionDetails/v1/", url.Values{"collectioncount": {"1"}, "publishedfileids[0]": {id}}, &r); err != nil {
		return nil, false, err
	}
	if len(r.Response.Details) == 0 || r.Response.Details[0].Result != 1 || len(r.Response.Details[0].Children) == 0 {
		return nil, false, nil
	}
	for _, c := range r.Response.Details[0].Children {
		if c.FileType == 0 { // an item (2 would be a collection in it)
			children = append(children, c.ID)
		}
	}
	return children, true, nil
}

// ErrNotFound: Steam has no such item (or it is private).
var ErrNotFound = errors.New("Steam has no Workshop item with this ID (or it is private)")
