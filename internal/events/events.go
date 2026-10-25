// Package events is the manager's log: what happened to which server, kept in daily files (logs/YYYY-MM-DD.log),
// the last ones in memory for the web interface, and handed to whoever listens (the web page, Discord).
package events

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/SnipeZilla/szm/internal/config"
)

// Kinds of events
const (
	Info     = "info"
	Start    = "start"
	Stop     = "stop"
	Crash    = "crash"
	Failed   = "failed" // gave up after too many crashes
	Update   = "update" // an update is available or starting
	Updated  = "updated"
	Restart  = "restart"
	Error    = "error"
	SteamCmd = "steamcmd"
	Manager  = "manager"
	Addons   = "addons" // addon updates: watching for a fix, a new version held back
)

// Event is a line of the log.
type Event struct {
	Time   time.Time `json:"time"`
	Kind   string    `json:"kind"`
	Server string    `json:"server,omitempty"` // ID
	Name   string    `json:"name,omitempty"`   // name of the server
	Text   string    `json:"text"`
	Report string    `json:"report,omitempty"` // a crash report of the server (its ID)
	// Machine: on a hub, the linked machine the event happened on (its ID); empty for this machine's own
	Machine string `json:"machine,omitempty"`
}

// Log keeps the events.
type Log struct {
	dir    string
	mu     sync.Mutex
	recent []Event
	subs   map[chan Event]struct{}
	file   *os.File
	day    string
	// crashes: when the servers crashed (every machine's, on a hub), the last 24 h: read again from the files at the
	// start, so a restart of the manager does not forget them
	crashes []Event
}

const keepInMemory = 500

// New opens the log in a folder.
func New(dir string) *Log {
	_ = os.MkdirAll(dir, 0o755)
	l := &Log{dir: dir, subs: map[chan Event]struct{}{}}
	l.loadCrashes(time.Now())
	return l
}

// loadCrashes finds the crashes of the last 24 h in the files of today and yesterday.
func (l *Log) loadCrashes(now time.Time) {
	since := now.Add(-24 * time.Hour)
	for _, day := range []time.Time{since, now} {
		f, err := os.Open(filepath.Join(l.dir, day.Format("2006-01-02")+".log"))
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 64<<10), 1<<20)
		for sc.Scan() {
			if !bytes.Contains(sc.Bytes(), []byte(`"kind":"crash"`)) {
				continue
			}
			var e Event
			if json.Unmarshal(sc.Bytes(), &e) == nil && e.Kind == Crash && e.Time.After(since) {
				l.crashes = append(l.crashes, Event{Time: e.Time, Kind: e.Kind, Server: e.Server, Machine: e.Machine})
			}
		}
		f.Close()
		if day.Format("2006-01-02") == now.Format("2006-01-02") {
			break // since and now are the same day
		}
	}
}

// noteCrash keeps the time of a crash. The lock must be held.
func (l *Log) noteCrash(e Event) {
	if e.Kind != Crash {
		return
	}
	l.crashes = append(l.crashes, Event{Time: e.Time, Kind: e.Kind, Server: e.Server, Machine: e.Machine})
	if len(l.crashes) > 10000 {
		l.crashes = l.crashes[len(l.crashes)-10000:]
	}
}

// Crashes24h counts the crashes of the last 24 hours (on a hub, those of every machine), of the servers sees
// accepts (nil: all).
func (l *Log) Crashes24h(sees func(Event) bool) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	since := time.Now().Add(-24 * time.Hour)
	k := 0
	for k < len(l.crashes) && !l.crashes[k].Time.After(since) {
		k++
	}
	l.crashes = l.crashes[k:]
	n := 0
	for _, e := range l.crashes {
		if sees == nil || sees(e) {
			n++
		}
	}
	return n
}

// Add writes an event. Tokens are hidden.
func (l *Log) Add(kind, server, name, format string, args ...any) Event {
	return l.AddReport("", kind, server, name, format, args...)
}

// AddReport writes an event that comes with a crash report.
func (l *Log) AddReport(report, kind, server, name, format string, args ...any) Event {
	e := Event{Time: time.Now(), Kind: kind, Server: server, Name: name, Text: config.Redact(fmt.Sprintf(format, args...)), Report: report}
	l.mu.Lock()
	l.recent = append(l.recent, e)
	if len(l.recent) > keepInMemory {
		l.recent = append([]Event(nil), l.recent[len(l.recent)-keepInMemory:]...)
	}
	l.noteCrash(e)
	l.write(e)
	subs := make([]chan Event, 0, len(l.subs))
	for c := range l.subs {
		subs = append(subs, c)
	}
	l.mu.Unlock()
	for _, c := range subs {
		select {
		case c <- e:
		default: // a slow listener misses it rather than block the manager
		}
	}
	return e
}

// AddRemote keeps an event that happened on a linked machine (a hub's node): its time and machine as they are.
func (l *Log) AddRemote(e Event) {
	l.mu.Lock()
	l.recent = append(l.recent, e)
	if len(l.recent) > keepInMemory {
		l.recent = append([]Event(nil), l.recent[len(l.recent)-keepInMemory:]...)
	}
	l.noteCrash(e)
	l.write(e)
	subs := make([]chan Event, 0, len(l.subs))
	for c := range l.subs {
		subs = append(subs, c)
	}
	l.mu.Unlock()
	for _, c := range subs {
		select {
		case c <- e:
		default:
		}
	}
}

func (l *Log) write(e Event) {
	day := e.Time.Format("2006-01-02")
	if l.file == nil || l.day != day {
		if l.file != nil {
			l.file.Close()
		}
		f, err := os.OpenFile(filepath.Join(l.dir, day+".log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			l.file = nil
			return
		}
		l.file, l.day = f, day
	}
	b, _ := json.Marshal(e)
	l.file.Write(append(b, '\n'))
}

// Recent gives the last events, of one server or of all ("").
func (l *Log) Recent(server string, n int) []Event {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []Event
	for i := len(l.recent) - 1; i >= 0 && len(out) < n; i-- {
		if server == "" || l.recent[i].Server == server {
			out = append(out, l.recent[i])
		}
	}
	return out
}

// Subscribe gives a channel that receives the events from now on; Unsubscribe closes it.
func (l *Log) Subscribe() chan Event {
	c := make(chan Event, 64)
	l.mu.Lock()
	l.subs[c] = struct{}{}
	l.mu.Unlock()
	return c
}

func (l *Log) Unsubscribe(c chan Event) {
	l.mu.Lock()
	if _, ok := l.subs[c]; ok {
		delete(l.subs, c)
		close(c)
	}
	l.mu.Unlock()
}

// Days lists the days that have a log, the latest first.
func (l *Log) Days() []string {
	entries, _ := os.ReadDir(l.dir)
	var days []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".log") {
			days = append(days, strings.TrimSuffix(e.Name(), ".log"))
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(days)))
	return days
}

// Day reads the events of a day (YYYY-MM-DD).
func (l *Log) Day(day string) ([]Event, error) {
	if _, err := time.Parse("2006-01-02", day); err != nil {
		return nil, fmt.Errorf("bad day")
	}
	f, err := os.Open(filepath.Join(l.dir, day+".log"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Event
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		var e Event
		if json.Unmarshal(sc.Bytes(), &e) == nil {
			out = append(out, e)
		}
	}
	return out, nil
}

// Clean deletes the daily files older than a number of days.
func (l *Log) Clean(keepDays int) {
	limit := time.Now().AddDate(0, 0, -keepDays)
	for _, d := range l.Days() {
		t, err := time.ParseInLocation("2006-01-02", d, time.Local)
		if err == nil && t.Before(limit) {
			os.Remove(filepath.Join(l.dir, d+".log"))
		}
	}
}
