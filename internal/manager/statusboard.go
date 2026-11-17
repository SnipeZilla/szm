package manager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SnipeZilla/szm/internal/config"
	"github.com/SnipeZilla/szm/internal/notify"
)

/* ---------------------------------------------------------------------------------------------------------------
 * Status cards: the live status of the servers in Discord channels
 *
 * A Discord channel of the settings can show the status of its servers (Settings, Discord, Live status): one
 * message a server, edited in place as the server changes (notify.Card). The manager needs nothing more than the
 * channel's webhook, and asks nothing new of the servers: it shows what it knows of them already.
 *
 *   - a card is edited when what it shows changed, a minute after its last edit at the earliest (at once when the
 *     server goes on or off); its chart every ten minutes; and every half hour whatever happens, so that its
 *     "updated" time tells a manager that runs from one that stopped
 *   - a card someone deleted in Discord is posted again; the cards of a server or channel taken out of the settings
 *     are deleted
 *   - on a hub the cards show the servers of every machine (RemoteCards), as its posts do
 *
 * What is kept between two runs (status.json): the messages of the cards, and the players of the last 24 hours.
 * ------------------------------------------------------------------------------------------------------------- */

const (
	boardMissing = 10 * time.Minute // a server of a linked machine that is not known for this long: its card goes
	boardHold    = 2 * time.Minute  // a running server the manager looks at anew is shown as it was for this long
	boardEvery   = 10 * time.Second // how often the cards are looked at
	boardSample  = time.Minute      // how often the players are counted, for the charts
	boardMinEdit = time.Minute      // between two edits of a card (what it shows aside from its state)
	boardChart   = 10 * time.Minute // between two charts of a card
	boardBeat    = 30 * time.Minute // a card is edited this often at least: its "updated" time says the manager runs
	boardSpacing = 1500 * time.Millisecond
	boardRetry   = time.Minute     // after Discord said no, before the channel is tried again
	boardSave    = 5 * time.Minute // the players counted are written this often
	boardHours   = 24
)

// the tests replace them
var (
	postCard   = notify.PostCard
	editCard   = notify.EditCard
	deleteCard = notify.DeleteCard
	boardPause = func(ctx context.Context, d time.Duration) bool {
		select {
		case <-ctx.Done():
			return false
		case <-time.After(d):
			return true
		}
	}
)

// StatusCard is a server as the status cards show it.
type StatusCard struct {
	ID       string
	Name     string
	Hostname string
	Game     string // its key (cs2): for its icon
	GameName string
	Engine   string
	Address  string // where players reach it
	State    string // notify.CardOnline…
	Note     string
	Map      string
	Players  int
	Max      int
	Bots     int
	Locked   bool
	Since    int64 // of its state, when the manager knows (online: its start), unix seconds
	Started  int64 // when its process started (0: none), unix seconds
	Remote   bool  // of a linked machine (on a hub): known only while that machine is
}

// CardState tells the state a card shows, from a server's status and what is being done with it.
func CardState(status, busy string) string {
	switch busy {
	case "install", "update":
		return notify.CardUpdating
	case "restart":
		return notify.CardRestarting
	}
	switch status {
	case StatusRunning:
		return notify.CardOnline
	case StatusStarting:
		return notify.CardStarting
	case StatusCrashed:
		return notify.CardRestarting // it is started again in a moment
	}
	return notify.CardOffline
}

// CardOf makes the card of a server from its view. ips: the addresses of its machine, for a server that listens
// on all of them.
func CardOf(v View, ips []string) StatusCard {
	c := StatusCard{ID: v.ID, Name: v.Name, Hostname: v.Hostname, Game: v.Game, GameName: v.GameName, Engine: v.Engine,
		Address: publicAddress(v.Address, ips), State: CardState(v.Status, v.Busy), Started: v.Started}
	if c.State == notify.CardOnline {
		c.Since = v.Started
		if v.Info != nil {
			c.Map, c.Max, c.Bots, c.Locked = v.Info.Map, v.Info.MaxPlayers, v.Info.Bots, v.Info.Password
			c.Players = max(v.Info.Players-v.Info.Bots, 0)
			if c.Hostname == "" {
				c.Hostname = v.Info.Name
			}
		}
	}
	return c
}

// publicAddress is where players reach a server: its own address when it has one they can reach, else the
// machine's address on the internet (a server that listens on every address, or on a private one behind it).
func publicAddress(addr string, ips []string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	if ip := net.ParseIP(host); ip != nil && publicIP(ip) {
		return addr
	}
	for _, ip := range ips {
		if publicIP(net.ParseIP(ip)) {
			return net.JoinHostPort(ip, port)
		}
	}
	if host == "*" || host == "" || host == "0.0.0.0" {
		if len(ips) > 0 {
			return net.JoinHostPort(ips[0], port)
		}
		return net.JoinHostPort("127.0.0.1", port)
	}
	return addr
}

// JoinLink gives the web address that opens the game on a server, from the one of the settings: {address} is the
// server's address (ip:port), {ip} and {port} its two parts. "" when there is none, or the game is not joined
// that way (Steam connects to the servers of the Source and GoldSrc games).
func JoinLink(tmpl, engine, addr string) string {
	tmpl = strings.TrimSpace(tmpl)
	if tmpl == "" || engine == "other" || engine == "" {
		return ""
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil || host == "*" {
		return ""
	}
	return strings.NewReplacer("{address}", addr, "{ip}", host, "{port}", port).Replace(tmpl)
}

// hourOf is the players of an hour, as they were counted.
type hourOf struct {
	H   int64 `json:"h"` // the hour (unix seconds / 3600)
	Sum int   `json:"s"`
	N   int   `json:"n"`
	Max int   `json:"m"`
}

// cardOf is the card of a server in a channel: its message, and what it showed last.
type cardOf struct {
	notify.CardMessage
	Sum     string `json:"sum"`             // of what it shows
	Chart   string `json:"chart,omitempty"` // of its chart
	State   string `json:"state"`
	Since   int64  `json:"since"`            // of its state
	At      int64  `json:"at"`               // its last edit
	ChartAt int64  `json:"chart_at"`         // its chart was last drawn
	Remote  bool   `json:"remote,omitempty"` // a server of a linked machine
	// the state it is about to show, and since when: kept until Discord took the edit
	Next      string `json:"-"`
	NextSince int64  `json:"-"`
	Missing   int64  `json:"-"` // its server is not among those known since then (a linked machine not heard yet)
	Hold      int64  `json:"-"` // its server is looked at anew since then (the manager started again): shown as it was
}

// channelOf is a channel's cards.
type channelOf struct {
	Webhook string             `json:"webhook"` // kept to delete its cards once the channel is out of the settings
	Cards   map[string]*cardOf `json:"cards"`
	Order   []string           `json:"order"` // the servers, as their cards were posted
}

type board struct {
	mu       sync.Mutex
	path     string
	Channels map[string]*channelOf `json:"channels"` // by the webhook's number
	History  map[string][]hourOf   `json:"history"`  // by server
	dirty    bool
	saved    time.Time
	sampled  time.Time
	// per channel, not kept: why Discord said no, and when it is tried again
	errs  map[string]string
	retry map[string]time.Time
}

func loadBoard(path string) *board {
	b := &board{path: path}
	if raw, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(raw, b)
	}
	if b.Channels == nil {
		b.Channels = map[string]*channelOf{}
	}
	if b.History == nil {
		b.History = map[string][]hourOf{}
	}
	for _, ch := range b.Channels {
		if ch.Cards == nil {
			ch.Cards = map[string]*cardOf{}
		}
	}
	b.errs, b.retry = map[string]string{}, map[string]time.Time{}
	return b
}

// save writes the board (it holds webhooks: for the manager's user only). The lock must be held.
func (b *board) save() {
	if b.path == "" {
		return
	}
	raw, err := json.Marshal(b)
	if err == nil {
		_ = config.WriteFileAtomic(b.path, raw, 0o600)
	}
	b.dirty, b.saved = false, time.Now()
}

// sample counts the players of a server now. The lock must be held.
func (b *board) sample(id string, players int, now time.Time) {
	h := now.Unix() / 3600
	list := b.History[id]
	if n := len(list); n == 0 || list[n-1].H != h {
		list = append(list, hourOf{H: h})
	}
	last := &list[len(list)-1]
	last.Sum += players
	last.N++
	last.Max = max(last.Max, players)
	for len(list) > 0 && list[0].H <= h-boardHours {
		list = list[1:]
	}
	b.History[id] = list
	b.dirty = true
}

// hours gives the last 24 hours of a server, the oldest first. The lock must be held.
func (b *board) hours(id string, now time.Time) []notify.Hour {
	h := now.Unix() / 3600
	out := make([]notify.Hour, boardHours)
	for _, x := range b.History[id] {
		k := int(x.H - (h - boardHours + 1))
		if k < 0 || k >= boardHours || x.N == 0 {
			continue
		}
		out[k] = notify.Hour{Avg: float64(x.Sum) / float64(x.N), Max: x.Max, Has: true}
	}
	return out
}

// BoardStatus is what Settings says of a channel's cards.
type BoardStatus struct {
	Cards int    `json:"cards"`
	At    int64  `json:"at,omitempty"` // the last edit of one
	Error string `json:"error,omitempty"`
}

// BoardStatus tells how the cards of a webhook are doing.
func (m *Manager) BoardStatus(webhook string) BoardStatus {
	b := m.board
	key := notify.HookID(webhook)
	if b == nil || key == "" {
		return BoardStatus{}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	out := BoardStatus{Error: b.errs[key]}
	if ch := b.Channels[key]; ch != nil {
		for _, c := range ch.Cards {
			if c.ID != "" {
				out.Cards++
				out.At = max(out.At, c.At)
			}
		}
	}
	return out
}

// statusCards gives the servers the cards can show: this machine's, and on a hub the other machines'.
func (m *Manager) statusCards() []StatusCard {
	var out []StatusCard
	var ips []string
	for _, s := range m.servers() {
		if ips == nil {
			ips = append([]string{}, localIPs()...)
		}
		out = append(out, CardOf(m.view(s), ips))
	}
	if m.RemoteCards != nil {
		out = append(out, m.RemoteCards()...)
	}
	return out
}

func (m *Manager) statusBoard(ctx context.Context) {
	t := time.NewTicker(boardEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			m.board.mu.Lock()
			if m.board.dirty {
				m.board.save()
			}
			m.board.mu.Unlock()
			return
		case <-t.C:
			m.boardRound(ctx, time.Now())
		}
	}
}

// boardRound brings the cards up to date: once, for every channel.
func (m *Manager) boardRound(ctx context.Context, now time.Time) {
	b := m.board
	m.Cfg.RLock()
	var hooks []config.DiscordHook
	seen := map[string]bool{}
	for _, d := range m.Cfg.Discords {
		key := notify.HookID(d.Webhook)
		if !d.Status || key == "" || seen[key] {
			continue // (two channels with the same webhook: the first one's cards)
		}
		seen[key] = true
		hooks = append(hooks, d)
	}
	m.Cfg.RUnlock()
	b.mu.Lock()
	idle := len(hooks) == 0 && len(b.Channels) == 0
	b.mu.Unlock()
	if idle {
		return // no channel shows the status: nothing is looked at, nothing counted
	}
	cards := m.statusCards()
	shown := map[string]bool{} // by a channel at least
	want := map[string][]StatusCard{}
	for _, d := range hooks {
		key := notify.HookID(d.Webhook)
		for _, c := range cards {
			if len(d.Servers) == 0 || contains(d.Servers, c.ID) {
				want[key] = append(want[key], c)
				shown[c.ID] = true
			}
		}
	}

	// the players, for the charts
	b.mu.Lock()
	if now.Sub(b.sampled) >= boardSample {
		b.sampled = now
		for _, c := range cards {
			if !shown[c.ID] || c.State == notify.CardUnknown {
				continue // of a machine that says nothing: not known, rather than nobody
			}
			b.sample(c.ID, c.Players, now)
		}
		for id, list := range b.History {
			if n := len(list); n == 0 || list[n-1].H <= now.Unix()/3600-boardHours {
				delete(b.History, id) // nothing in 24 hours: a server that is gone, or shown no more
				b.dirty = true
			}
		}
	}
	// the channels that show the status no more: their cards are deleted
	var gone []string
	for key := range b.Channels {
		if !seen[key] {
			gone = append(gone, key)
		}
	}
	b.mu.Unlock()
	for _, key := range gone {
		if !m.dropChannel(ctx, key) {
			return
		}
	}

	for _, d := range hooks {
		if !m.boardChannel(ctx, d, want[notify.HookID(d.Webhook)], now) {
			return
		}
	}
	b.mu.Lock()
	if b.dirty && now.Sub(b.saved) >= boardSave {
		b.save()
	}
	b.mu.Unlock()
}

// dropChannel deletes the cards of a channel that shows the status no more. false: the manager stops.
func (m *Manager) dropChannel(ctx context.Context, key string) bool {
	b := m.board
	b.mu.Lock()
	ch := b.Channels[key]
	var ids []string
	hook := ""
	if ch != nil {
		hook = ch.Webhook
		for _, c := range ch.Cards {
			if c.ID != "" {
				ids = append(ids, c.ID)
			}
		}
	}
	b.mu.Unlock()
	for _, id := range ids {
		if err := deleteCard(hook, id); err != nil {
			fmt.Fprintln(os.Stderr, "discord status:", err) // (a webhook deleted in Discord: its messages can not be)
		}
		if !boardPause(ctx, boardSpacing) {
			return false
		}
	}
	b.mu.Lock()
	delete(b.Channels, key)
	delete(b.errs, key)
	delete(b.retry, key)
	b.save()
	b.mu.Unlock()
	return true
}

// boardChannel brings the cards of a channel up to date. false: the manager stops.
func (m *Manager) boardChannel(ctx context.Context, d config.DiscordHook, cards []StatusCard, now time.Time) bool {
	b := m.board
	key := notify.HookID(d.Webhook)
	b.mu.Lock()
	if now.Before(b.retry[key]) {
		b.mu.Unlock()
		return true
	}
	ch := b.Channels[key]
	if ch == nil {
		ch = &channelOf{Cards: map[string]*cardOf{}}
		b.Channels[key] = ch
	}
	ch.Webhook = d.Webhook
	// the servers this channel shows no more
	wanted := map[string]bool{}
	for _, c := range cards {
		wanted[c.ID] = true
	}
	type old struct{ server, msg string }
	var olds []old
	for id, c := range ch.Cards {
		if wanted[id] {
			c.Missing = 0
			continue
		}
		// A server of a linked machine may only be not heard of yet (the hub just started, its machine links again
		// in a moment): its card stays a while, unless the channel's settings leave the server out.
		if c.Remote && (len(d.Servers) == 0 || contains(d.Servers, id)) {
			if c.Missing == 0 {
				c.Missing = now.Unix()
			}
			if now.Unix()-c.Missing < int64(boardMissing/time.Second) {
				continue
			}
		}
		olds = append(olds, old{id, c.ID})
	}
	// many servers in one channel: each card waits longer between two edits (Discord counts them by channel)
	minEdit := max(boardMinEdit, time.Duration(len(cards))*3*time.Second)
	b.mu.Unlock()

	failed := func(err error) {
		fmt.Fprintln(os.Stderr, "discord status ("+d.Name+"):", err)
		b.mu.Lock()
		b.errs[key], b.retry[key] = capitalizeFirst(err.Error()), now.Add(boardRetry)
		b.mu.Unlock()
	}
	asked := false // Discord was asked something this round: it is not asked again at once
	pause := func() bool {
		if !asked {
			asked = true
			return true
		}
		return boardPause(ctx, boardSpacing)
	}
	for _, o := range olds {
		if o.msg != "" {
			if !pause() {
				return false
			}
			if err := deleteCard(d.Webhook, o.msg); err != nil {
				failed(err)
				return true
			}
		}
		b.mu.Lock()
		delete(ch.Cards, o.server)
		ch.Order = dropID(ch.Order, o.server)
		b.save()
		b.mu.Unlock()
	}

	for _, sc := range cards {
		b.mu.Lock()
		cs := ch.Cards[sc.ID]
		if cs == nil {
			cs = &cardOf{}
			ch.Cards[sc.ID] = cs
			ch.Order = append(ch.Order, sc.ID)
		}
		cs.Remote = sc.Remote
		// The manager started again and looks anew at a server that went on running (the same process the card
		// shows online): it is not "starting" for the players. Its card stays as it is until the server answers.
		if sc.State == notify.CardStarting && cs.State == notify.CardOnline && sc.Started != 0 && sc.Started == cs.Since {
			if cs.Hold == 0 {
				cs.Hold = now.Unix()
			}
			if now.Unix()-cs.Hold < int64(boardHold/time.Second) {
				b.mu.Unlock()
				continue
			}
		} else {
			cs.Hold = 0
		}
		changed := cs.State != sc.State
		since := sc.Since
		if since == 0 {
			since = cs.Since
			if changed {
				// when the manager saw it change (for a card that is new: not known)
				if cs.Next != sc.State {
					cs.Next, cs.NextSince = sc.State, 0
					if cs.State != "" {
						cs.NextSince = now.Unix()
					}
				}
				since = cs.NextSince
			}
		}
		card := m.notifyCard(sc, d.Join, since)
		sum := notify.CardSum(card)
		msg := cs.CardMessage
		isNew := msg.ID == ""
		edit := isNew || (sum != cs.Sum && (changed || now.Unix()-cs.At >= int64(minEdit/time.Second))) ||
			now.Unix()-cs.At >= int64(boardBeat/time.Second)
		chartDue := isNew || changed || now.Unix()-cs.ChartAt >= int64(boardChart/time.Second)
		newChart := false
		var chartSum string
		if chartDue || edit {
			card.Chart = notify.Chart(b.hours(sc.ID, now))
			chartSum = notify.ChartSum(card.Chart)
			newChart = chartSum != cs.Chart
			if chartDue {
				cs.ChartAt = now.Unix()
				edit = edit || newChart
			}
		}
		b.mu.Unlock()
		if !edit {
			continue
		}
		if !pause() {
			return false
		}
		var err error
		if !isNew {
			msg, err = editCard(d.Webhook, msg, card, newChart)
			if errors.Is(err, notify.ErrGone) {
				isNew = true // deleted in Discord: posted again
				if !boardPause(ctx, boardSpacing) {
					return false
				}
			}
		}
		if isNew {
			msg, err = postCard(d.Webhook, d.Username, card)
		}
		if err != nil {
			failed(err)
			return true
		}
		b.mu.Lock()
		cs.CardMessage, cs.Sum, cs.Chart, cs.State, cs.Since, cs.At = msg, sum, chartSum, sc.State, since, now.Unix()
		cs.Next, cs.NextSince = "", 0
		if isNew {
			cs.ChartAt = cs.At
			b.save() // its message is known from now on: not posted twice after a stop
		} else {
			b.dirty = true
		}
		delete(b.errs, key)
		b.mu.Unlock()
	}
	return true
}

func dropID(list []string, s string) []string {
	out := list[:0]
	for _, x := range list {
		if x != s {
			out = append(out, x)
		}
	}
	return out
}

func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// notifyCard is the card of a server as Discord gets it: with its game's icon and its Join link.
func (m *Manager) notifyCard(sc StatusCard, join string, since int64) notify.Card {
	c := notify.Card{Name: sc.Name, Hostname: sc.Hostname, Game: sc.GameName, Address: sc.Address, State: sc.State, Note: sc.Note,
		Map: sc.Map, Players: sc.Players, Max: sc.Max, Bots: sc.Bots, Locked: sc.Locked, Join: JoinLink(join, sc.Engine, sc.Address)}
	if since > 0 {
		c.Since = time.Unix(since, 0)
	}
	if g := m.Catalog()[sc.Game]; g != nil {
		if c.Game == "" {
			c.Game = g.Name
		}
		if g.Icon != "" && m.GameIcon != nil {
			if png := m.GameIcon(g.Icon); len(png) > 0 {
				c.Icon, c.IconName = png, g.Icon+".png"
			}
		}
	}
	return c
}

// ValidJoin tells whether a Join link of the settings can be used: a web address with the server's address in it.
func ValidJoin(tmpl string) error {
	tmpl = strings.TrimSpace(tmpl)
	if tmpl == "" {
		return nil
	}
	if !strings.HasPrefix(tmpl, "https://") && !strings.HasPrefix(tmpl, "http://") {
		return errors.New("the Join link is a web address (https://…): Discord opens no other")
	}
	if !strings.Contains(tmpl, "{address}") && !(strings.Contains(tmpl, "{ip}") && strings.Contains(tmpl, "{port}")) {
		return errors.New("the Join link needs {address} where the server's address goes (https://example.com/join?a={address})")
	}
	if strings.ContainsAny(tmpl, " ()<>\"'") {
		return errors.New("the Join link can not have spaces, quotes or brackets")
	}
	return nil
}

// ValidJoinAddress tells whether an address is a server's (a name or an IP, and a port): what the Join page opens.
func ValidJoinAddress(a string) bool {
	host, port, err := net.SplitHostPort(a)
	if err != nil || host == "" || len(host) > 253 {
		return false
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
		return false
	}
	for _, r := range host {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == ':') {
			return false
		}
	}
	return true
}
