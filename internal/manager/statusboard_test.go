package manager

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SnipeZilla/szm/internal/config"
	"github.com/SnipeZilla/szm/internal/games"
	"github.com/SnipeZilla/szm/internal/notify"
	"github.com/SnipeZilla/szm/internal/query"
)

// fakeCards stands for Discord: the messages of the cards, and what was asked.
type fakeCards struct {
	next  int
	cards map[string]notify.Card // by message
	calls []string
	fail  error
}

func (f *fakeCards) install(t *testing.T) {
	oldPost, oldEdit, oldDelete, oldPause := postCard, editCard, deleteCard, boardPause
	t.Cleanup(func() { postCard, editCard, deleteCard, boardPause = oldPost, oldEdit, oldDelete, oldPause })
	f.cards = map[string]notify.Card{}
	boardPause = func(context.Context, time.Duration) bool { return true }
	postCard = func(webhook, username string, c notify.Card) (notify.CardMessage, error) {
		if f.fail != nil {
			return notify.CardMessage{}, f.fail
		}
		f.next++
		id := fmt.Sprint("m", f.next)
		f.cards[id] = c
		f.calls = append(f.calls, "post "+c.Name)
		return notify.CardMessage{ID: id, Files: map[string]string{"players.png": "a" + id}}, nil
	}
	editCard = func(webhook string, msg notify.CardMessage, c notify.Card, chart bool) (notify.CardMessage, error) {
		if f.fail != nil {
			return notify.CardMessage{}, f.fail
		}
		if _, ok := f.cards[msg.ID]; !ok {
			return notify.CardMessage{}, notify.ErrGone
		}
		f.cards[msg.ID] = c
		what := "edit "
		if chart {
			what = "edit+chart "
		}
		f.calls = append(f.calls, what+c.Name)
		return msg, nil
	}
	deleteCard = func(webhook, id string) error {
		f.calls = append(f.calls, "delete "+f.cards[id].Name)
		delete(f.cards, id)
		return nil
	}
}

func (f *fakeCards) took() string {
	s := strings.Join(f.calls, ", ")
	f.calls = nil
	return s
}

func (f *fakeCards) of(name string) notify.Card {
	for _, c := range f.cards {
		if c.Name == name {
			return c
		}
	}
	return notify.Card{}
}

func boardManager(t *testing.T, dir string) *Manager {
	t.Helper()
	cfg, _ := config.Load(filepath.Join(dir, "szm.json"))
	cfg.Servers = []*config.Server{
		{ID: "css-1", Name: "One", Game: "css", InstallDir: filepath.Join(dir, "one"), IP: "203.0.113.7", Port: 27015},
		{ID: "css-2", Name: "Two", Game: "css", InstallDir: filepath.Join(dir, "two"), IP: "203.0.113.7", Port: 27016},
	}
	cfg.Discords = []config.DiscordHook{{Name: "Status", Webhook: "https://discord.com/api/webhooks/111/tok", Status: true,
		Join: "https://example.com/join?a={address}", Events: []string{}}}
	cat, _ := games.LoadOrBuiltin(t.TempDir())
	return New(cfg, cat, dir)
}

func setUp(m *Manager, id string, players int, mapName string) {
	i := m.instance(id)
	i.mu.Lock()
	i.status, i.pid, i.since = StatusRunning, 4242, time.Unix(1700000000, 0)
	i.info = &query.Info{Name: "Host of " + id, Map: mapName, Players: players, MaxPlayers: 32}
	i.mu.Unlock()
}

func setDown(m *Manager, id string) {
	i := m.instance(id)
	i.mu.Lock()
	i.status, i.pid, i.info = StatusStopped, 0, nil
	i.mu.Unlock()
}

// The cards of a channel: posted once, edited as the servers change (at once when one goes on or off, a minute
// apart otherwise), posted again when deleted in Discord, deleted when the settings show them no more.
func TestStatusBoard(t *testing.T) {
	dir := t.TempDir()
	m := boardManager(t, dir)
	var f fakeCards
	f.install(t)
	ctx := context.Background()
	hook := m.Cfg.Discords[0].Webhook
	now := time.Unix(1700003600, 0)
	round := func(after time.Duration) string {
		now = now.Add(after)
		m.boardRound(ctx, now)
		return f.took()
	}
	setUp(m, "css-1", 5, "de_dust2")
	setDown(m, "css-2")

	if got := round(0); got != "post One, post Two" {
		t.Fatal(got)
	}
	one, two := f.of("One"), f.of("Two")
	if one.State != notify.CardOnline || one.Hostname != "Host of css-1" || one.Map != "de_dust2" || one.Players != 5 || one.Max != 32 ||
		one.Address != "203.0.113.7:27015" || one.Join != "https://example.com/join?a=203.0.113.7:27015" || one.Since.Unix() != 1700000000 ||
		one.Game != "Counter-Strike: Source" || len(one.Chart) < 100 {
		t.Fatalf("%+v", one)
	}
	if two.State != notify.CardOffline || two.Map != "" || !two.Since.IsZero() {
		t.Fatalf("%+v", two)
	}
	if st := m.BoardStatus(hook); st.Cards != 2 || st.Error != "" || st.At == 0 {
		t.Fatalf("%+v", st)
	}
	// nothing changed: Discord is asked nothing
	for k := 0; k < 3; k++ {
		if got := round(10 * time.Second); got != "" {
			t.Fatal(got)
		}
	}
	// players come: not before a minute after its last edit, then once
	setUp(m, "css-1", 9, "de_dust2")
	if got := round(10 * time.Second); got != "" {
		t.Fatal(got)
	}
	if got := round(30 * time.Second); !strings.HasPrefix(got, "edit") || !strings.HasSuffix(got, " One") || f.of("One").Players != 9 {
		t.Fatal(got)
	}
	// it goes off: at once, with since when
	setDown(m, "css-1")
	if got := round(10 * time.Second); !strings.HasSuffix(got, " One") || f.of("One").State != notify.CardOffline || f.of("One").Since.Unix() != now.Unix() {
		t.Fatalf("%s %+v", got, f.of("One"))
	}
	went := now.Unix()
	// its chart, ten minutes later: the players of the hour are in it (sent only when it changed)
	setUp(m, "css-2", 3, "cs_office")
	if got := round(10 * time.Second); !strings.HasSuffix(got, " Two") {
		t.Fatal(got)
	}
	got := ""
	for k := 0; k < 70 && !strings.Contains(got, "edit+chart Two"); k++ {
		got = round(10 * time.Second)
	}
	if !strings.Contains(got, "edit+chart Two") {
		t.Fatal("no new chart in ten minutes")
	}
	if f.of("One").Since.Unix() != went {
		t.Fatal("since when it is off changed")
	}
	// a card deleted in Discord by someone: posted again at its next change
	for id, c := range f.cards {
		if c.Name == "Two" {
			delete(f.cards, id)
		}
	}
	setUp(m, "css-2", 7, "cs_office")
	if got := round(61 * time.Second); got != "post Two" {
		t.Fatal(got)
	}
	// Discord says no: told in Settings, tried again a minute later, not before
	f.fail = errors.New("Discord answered 401 Unauthorized: Invalid Webhook Token")
	setDown(m, "css-2")
	if got := round(10 * time.Second); got != "" {
		t.Fatal(got)
	}
	if st := m.BoardStatus(hook); !strings.Contains(st.Error, "Invalid Webhook Token") {
		t.Fatalf("%+v", st)
	}
	f.fail = nil
	if got := round(10 * time.Second); got != "" {
		t.Fatal("tried again too soon: " + got)
	}
	if got := round(60 * time.Second); !strings.HasSuffix(got, " Two") || m.BoardStatus(hook).Error != "" {
		t.Fatal(got)
	}
	// nothing at all for half an hour: still edited, so that its "updated" time is not old
	got = ""
	for k := 0; k < 31 && got == ""; k++ {
		got = round(time.Minute)
	}
	if !strings.Contains(got, "edit") {
		t.Fatal("no sign of life in half an hour")
	}

	// the manager stops and starts: the cards are the same messages, none is posted again
	m.board.mu.Lock()
	m.board.save()
	m.board.mu.Unlock()
	m2 := boardManager(t, dir)
	setDown(m2, "css-1")
	setDown(m2, "css-2")
	m2.boardRound(ctx, now.Add(10*time.Second))
	if got := f.took(); strings.Contains(got, "post") {
		t.Fatal(got)
	}
	if st := m2.BoardStatus(hook); st.Cards != 2 {
		t.Fatalf("%+v", st)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "status.json")); !strings.Contains(string(b), `"history"`) {
		t.Fatal("no status.json")
	}

	// The manager started again and looks at a server that never stopped: until it answers, its card stays "online"
	// (it is not starting for the players), for two minutes at most.
	setUp(m2, "css-1", 4, "de_dust2")
	m2.boardRound(ctx, now.Add(20*time.Second))
	f.took()
	i := m2.instance("css-1")
	i.mu.Lock()
	i.status, i.info = StatusStarting, nil
	i.mu.Unlock()
	m2.boardRound(ctx, now.Add(30*time.Second))
	m2.boardRound(ctx, now.Add(100*time.Second))
	if got := f.took(); got != "" || f.of("One").State != notify.CardOnline {
		t.Fatal("shown starting: " + got)
	}
	m2.boardRound(ctx, now.Add(160*time.Second))
	if got := f.took(); !strings.Contains(got, " One") || f.of("One").State != notify.CardStarting {
		t.Fatal(got)
	}
	// (the first manager goes on below: it sees them as it left them)
	setDown(m, "css-1")
	now = now.Add(200 * time.Second)
	m.boardRound(ctx, now)
	f.took()

	// the channel shows one server only: the other's card is deleted
	m.Cfg.Lock()
	m.Cfg.Discords[0].Servers = []string{"css-2"}
	m.Cfg.Unlock()
	if got := round(10 * time.Second); got != "delete One" {
		t.Fatal(got)
	}
	// the live status is turned off: its cards are deleted, and nothing is looked at any more
	m.Cfg.Lock()
	m.Cfg.Discords[0].Status = false
	m.Cfg.Unlock()
	if got := round(10 * time.Second); got != "delete Two" || len(f.cards) != 0 {
		t.Fatal(got)
	}
	if got := round(10 * time.Second); got != "" || len(m.board.Channels) != 0 {
		t.Fatal(got)
	}
}

// A hub shows the servers of its machines too; one whose machine says nothing has no news, not its last state.
func TestStatusBoardRemote(t *testing.T) {
	m := boardManager(t, t.TempDir())
	var f fakeCards
	f.install(t)
	m.Cfg.Lock()
	m.Cfg.Discords[0].Servers = []string{"eu~cs2-1"}
	m.Cfg.Unlock()
	remote := StatusCard{ID: "eu~cs2-1", Name: "EU", Hostname: "SnipeZilla EU", Game: "cs2", GameName: "Counter-Strike 2", Engine: "source2",
		Address: "198.51.100.4:27015", State: notify.CardOnline, Map: "de_mirage", Players: 10, Max: 12, Since: 1700000000, Started: 1700000000, Remote: true}
	m.RemoteCards = func() []StatusCard { return []StatusCard{remote} }
	now := time.Unix(1700003600, 0)
	m.boardRound(context.Background(), now)
	if got := f.took(); got != "post EU" {
		t.Fatal(got)
	}
	if c := f.of("EU"); c.Players != 10 || c.Join != "https://example.com/join?a=198.51.100.4:27015" || c.Game != "Counter-Strike 2" {
		t.Fatalf("%+v", c)
	}
	remote.State, remote.Note, remote.Players, remote.Map = notify.CardUnknown, "its machine is not reachable", 0, ""
	m.boardRound(context.Background(), now.Add(10*time.Second))
	if c := f.of("EU"); c.State != notify.CardUnknown || c.Note == "" {
		t.Fatalf("%+v", c)
	}
	// the hub starts again and has not heard of its machines yet: the card stays (for ten minutes), then goes
	remote.State, remote.Note, remote.Players, remote.Map = notify.CardOnline, "", 10, "de_mirage"
	m.boardRound(context.Background(), now.Add(20*time.Second))
	f.took()
	m.RemoteCards = func() []StatusCard { return nil }
	m.boardRound(context.Background(), now.Add(30*time.Second))
	m.boardRound(context.Background(), now.Add(9*time.Minute))
	if got := f.took(); got != "" || len(f.cards) != 1 {
		t.Fatal("deleted while its machine is only not heard of yet: " + got)
	}
	m.RemoteCards = func() []StatusCard { return []StatusCard{remote} }
	m.boardRound(context.Background(), now.Add(9*time.Minute+10*time.Second))
	if got := f.took(); strings.Contains(got, "post") || strings.Contains(got, "delete") {
		t.Fatal(got)
	}
	m.RemoteCards = func() []StatusCard { return nil }
	m.boardRound(context.Background(), now.Add(10*time.Minute))
	m.boardRound(context.Background(), now.Add(21*time.Minute))
	if got := f.took(); got != "delete EU" {
		t.Fatal(got)
	}
	m.RemoteCards = func() []StatusCard { return []StatusCard{remote} }
	m.boardRound(context.Background(), now.Add(22*time.Minute))
	if got := f.took(); got != "post EU" {
		t.Fatal(got)
	}
	remote.State, remote.Note, remote.Players, remote.Map = notify.CardUnknown, "its machine is not reachable", 0, ""
	now = now.Add(23 * time.Minute)
	m.boardRound(context.Background(), now)
	m.board.mu.Lock()
	delete(m.board.History, "eu~cs2-1")
	m.board.sampled = time.Time{}
	m.board.mu.Unlock()
	remote.State, remote.Note, remote.Players = notify.CardOnline, "", 4
	m.boardRound(context.Background(), now.Add(10*time.Second))
	remote.State, remote.Note, remote.Players = notify.CardUnknown, "its machine is not reachable", 0
	now = now.Add(20 * time.Second)
	m.boardRound(context.Background(), now)
	// its players are not counted as nobody while there is no news
	m.board.mu.Lock()
	n := m.board.History["eu~cs2-1"][0].N
	m.board.mu.Unlock()
	m.boardRound(context.Background(), now.Add(2*time.Minute))
	m.board.mu.Lock()
	n2 := m.board.History["eu~cs2-1"][0].N
	m.board.mu.Unlock()
	if n != 1 || n2 != 1 {
		t.Fatal(n, n2)
	}
}

func TestBoardHistory(t *testing.T) {
	b := loadBoard("")
	start := time.Unix(1700000000, 0).Truncate(time.Hour)
	for min := 0; min < 30*60; min++ { // 30 hours, a count a minute
		b.sample("s", (min/60)%5, start.Add(time.Duration(min)*time.Minute))
	}
	end := start.Add(30*time.Hour - time.Minute)
	hours := b.hours("s", end)
	if len(hours) != 24 || len(b.History["s"]) != 24 {
		t.Fatal(len(hours), len(b.History["s"]))
	}
	for k, h := range hours {
		want := (6 + k) % 5 // the hour 6 of the 30 is the oldest kept
		if !h.Has || h.Max != want || h.Avg != float64(want) {
			t.Fatalf("hour %d: %+v, want %d", k, h, want)
		}
	}
	// a manager that was stopped: those hours are not known
	later := b.hours("s", end.Add(3*time.Hour))
	if later[23].Has || later[22].Has || later[21].Has || !later[20].Has {
		t.Fatalf("%+v", later[19:])
	}
}

func TestCardHelpers(t *testing.T) {
	pub := []string{"192.168.1.10", "203.0.113.7"}
	for _, c := range [][3]string{
		{"*:27015", "203.0.113.7:27015", "a public address of the machine"},
		{"192.168.1.10:27015", "203.0.113.7:27015", "behind it"},
		{"198.51.100.9:27015", "198.51.100.9:27015", "its own"},
	} {
		if got := publicAddress(c[0], pub); got != c[1] {
			t.Errorf("%s: %s, want %s (%s)", c[0], got, c[1], c[2])
		}
	}
	if got := publicAddress("*:27015", []string{"192.168.1.10"}); got != "192.168.1.10:27015" {
		t.Error(got)
	}
	if got := publicAddress("*:27015", nil); got != "127.0.0.1:27015" {
		t.Error(got)
	}
	if JoinLink("https://x/join?a={address}", "source", "1.2.3.4:27015") != "https://x/join?a=1.2.3.4:27015" ||
		JoinLink("https://x/{ip}/{port}", "goldsrc", "1.2.3.4:27015") != "https://x/1.2.3.4/27015" ||
		JoinLink("https://x/join?a={address}", "other", "1.2.3.4:27015") != "" || JoinLink("", "source", "1.2.3.4:27015") != "" {
		t.Error("JoinLink")
	}
	for tmpl, ok := range map[string]bool{"": true, "https://x/join?a={address}": true, "https://x/{ip}:{port}": true,
		"steam://connect/{address}": false, "https://x/join": false, "https://x/j oin?a={address}": false, "https://x/a)?{address}": false} {
		if (ValidJoin(tmpl) == nil) != ok {
			t.Errorf("ValidJoin(%q)", tmpl)
		}
	}
	for a, ok := range map[string]bool{"1.2.3.4:27015": true, "play.example.com:27015": true, "[2001:db8::1]:27015": true,
		"1.2.3.4": false, "1.2.3.4:0": false, "1.2.3.4:99999": false, "a b:1": false, "x/y:1": false, `"><script>:1`: false, "": false} {
		if ValidJoinAddress(a) != ok {
			t.Errorf("ValidJoinAddress(%q)", a)
		}
	}
	if CardState(StatusRunning, "") != notify.CardOnline || CardState(StatusRunning, "update") != notify.CardUpdating ||
		CardState(StatusCrashed, "") != notify.CardRestarting || CardState(StatusFailed, "") != notify.CardOffline ||
		CardState(StatusStopped, "restart") != notify.CardRestarting {
		t.Error("CardState")
	}
}
