package web

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/SnipeZilla/szm/internal/config"
)

// Rights of a user, the letters of the old manager:
//
//	a  admin: everything
//	b  settings of the manager
//	c  add, install and delete servers
//	d  users
//	e  sees every server (else only the servers of their list)
//	f  command line and options of a server
//	g  console and RCON
//	h  update servers
//	i  start, stop, restart
//	j  files
const allRights = "abcdefghij"

// RightNames describe the rights, for the web interface.
var RightNames = map[string]string{
	"a": "Admin (everything)",
	"b": "Manager settings",
	"c": "Add, install, delete servers",
	"d": "Users",
	"e": "Sees every server",
	"f": "Server settings and command line",
	"g": "Console and RCON",
	"h": "Update servers",
	"i": "Start, stop, restart",
	"j": "Files",
}

// User of the web interface.
type User struct {
	Name    string   `json:"name"`
	Hash    string   `json:"hash"`
	Rights  string   `json:"rights"`
	Servers []string `json:"servers"` // the servers they look after (without e)
	Created int64    `json:"created"`
	LastIn  int64    `json:"last_login,omitempty"`
}

// Can tells whether a user has a right.
func (u *User) Can(r string) bool {
	return strings.Contains(u.Rights, "a") || strings.Contains(u.Rights, r)
}

// Sees tells whether a user sees a server.
func (u *User) Sees(id string) bool {
	if u.Can("e") {
		return true
	}
	for _, s := range u.Servers {
		if s == id {
			return true
		}
	}
	return false
}

const iterations = 210000

func hashPassword(pw string) string {
	salt := make([]byte, 16)
	_, _ = rand.Read(salt)
	key, _ := pbkdf2.Key(sha256.New, pw, salt, iterations, 32)
	return "pbkdf2-sha256$" + itoa(iterations) + "$" + base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(key)
}

func checkPassword(hash, pw string) bool {
	p := strings.Split(hash, "$")
	if len(p) != 4 || p[0] != "pbkdf2-sha256" {
		return false
	}
	iter := atoi(p[1])
	salt, err1 := base64.RawStdEncoding.DecodeString(p[2])
	want, err2 := base64.RawStdEncoding.DecodeString(p[3])
	if err1 != nil || err2 != nil || iter < 1000 {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, pw, salt, iter, len(want))
	return err == nil && subtle.ConstantTimeCompare(got, want) == 1
}

// Users is users.json.
type Users struct {
	path string
	mu   sync.RWMutex
	list []*User
	// the file as last read or written: changed by someone else (szm password, while the manager runs), it is read
	// again, at most every 2 s
	stamp     fileStamp
	checkedAt time.Time
}

type fileStamp struct {
	mod  time.Time
	size int64
}

func stampOf(path string) fileStamp {
	st, err := os.Stat(path)
	if err != nil {
		return fileStamp{}
	}
	return fileStamp{st.ModTime(), st.Size()}
}

func loadUsers(path string) (*Users, error) {
	u := &Users{path: path, checkedAt: time.Now()}
	list, err := readUsers(path)
	if err != nil {
		return nil, err
	}
	u.list, u.stamp = list, stampOf(path)
	return u, nil
}

func readUsers(path string) ([]*User, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var list []*User
	if err := json.Unmarshal(b, &list); err != nil {
		return nil, err
	}
	return list, nil
}

// fresh reads the file again when it changed under the manager. A file that does not read (half written, broken
// by hand) keeps the users known.
func (u *Users) fresh() {
	u.mu.Lock()
	defer u.mu.Unlock()
	if time.Since(u.checkedAt) < 2*time.Second {
		return
	}
	u.checkedAt = time.Now()
	if st := stampOf(u.path); st != u.stamp {
		if list, err := readUsers(u.path); err == nil && list != nil {
			u.list, u.stamp = list, st
		}
	}
}

func (u *Users) save() error {
	b, err := json.MarshalIndent(u.list, "", "  ")
	if err != nil {
		return err
	}
	if err := config.WriteFileAtomic(u.path, b, 0o600); err != nil {
		return err
	}
	u.stamp = stampOf(u.path)
	return nil
}

func (u *Users) get(name string) *User {
	u.fresh()
	u.mu.RLock()
	defer u.mu.RUnlock()
	for _, x := range u.list {
		if strings.EqualFold(x.Name, name) {
			cp := *x
			return &cp
		}
	}
	return nil
}

func (u *Users) count() int {
	u.fresh()
	u.mu.RLock()
	defer u.mu.RUnlock()
	return len(u.list)
}

func (u *Users) all() []User {
	u.fresh()
	u.mu.RLock()
	defer u.mu.RUnlock()
	out := make([]User, len(u.list))
	for i, x := range u.list {
		out[i] = *x
		out[i].Hash = ""
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

// put adds or changes a user (password "" keeps the old one).
func (u *Users) put(nu User, password string) error {
	u.fresh()
	u.mu.Lock()
	defer u.mu.Unlock()
	for _, x := range u.list {
		if strings.EqualFold(x.Name, nu.Name) {
			x.Rights, x.Servers = nu.Rights, nu.Servers
			if password != "" {
				x.Hash = hashPassword(password)
			}
			return u.save()
		}
	}
	if password == "" {
		return errors.New("a new user needs a password")
	}
	nu.Hash = hashPassword(password)
	nu.Created = time.Now().Unix()
	u.list = append(u.list, &nu)
	return u.save()
}

func (u *Users) remove(name string) error {
	u.fresh()
	u.mu.Lock()
	defer u.mu.Unlock()
	out := u.list[:0]
	for _, x := range u.list {
		if !strings.EqualFold(x.Name, name) {
			out = append(out, x)
		}
	}
	u.list = out
	return u.save()
}

func (u *Users) admins() int {
	u.fresh()
	u.mu.RLock()
	defer u.mu.RUnlock()
	n := 0
	for _, x := range u.list {
		if strings.Contains(x.Rights, "a") {
			n++
		}
	}
	return n
}

func (u *Users) touch(name string) {
	u.fresh()
	u.mu.Lock()
	defer u.mu.Unlock()
	for _, x := range u.list {
		if strings.EqualFold(x.Name, name) {
			x.LastIn = time.Now().Unix()
		}
	}
	_ = u.save()
}

/* ---------------------------------------------------------------------------------------------------------------
 * Sessions
 * ------------------------------------------------------------------------------------------------------------- */

type session struct {
	user    string
	expires time.Time
	ttl     time.Duration
}

type sessions struct {
	mu   sync.Mutex
	m    map[string]*session
	fail map[string][]time.Time // failed logins by address
}

func newSessions() *sessions {
	return &sessions{m: map[string]*session{}, fail: map[string][]time.Time{}}
}

func (s *sessions) create(user string, remember bool) (string, time.Duration) {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	tok := hex.EncodeToString(b)
	ttl := 12 * time.Hour
	if remember {
		ttl = 30 * 24 * time.Hour
	}
	s.mu.Lock()
	// Expired sessions nobody comes back with are dropped here, so they do not pile up
	now := time.Now()
	for k, x := range s.m {
		if now.After(x.expires) {
			delete(s.m, k)
		}
	}
	s.m[tok] = &session{user: user, expires: now.Add(ttl), ttl: ttl}
	s.mu.Unlock()
	return tok, ttl
}

func (s *sessions) get(tok string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	x, ok := s.m[tok]
	if !ok {
		return ""
	}
	if time.Now().After(x.expires) {
		delete(s.m, tok)
		return ""
	}
	x.expires = time.Now().Add(x.ttl) // sliding
	return x.user
}

func (s *sessions) drop(tok string) {
	s.mu.Lock()
	delete(s.m, tok)
	s.mu.Unlock()
}

// dropUser ends the sessions of a user (deleted, or password changed).
func (s *sessions) dropUser(name, keep string) {
	s.mu.Lock()
	for k, v := range s.m {
		if strings.EqualFold(v.user, name) && k != keep {
			delete(s.m, k)
		}
	}
	s.mu.Unlock()
}

// throttled tells whether an address failed too many logins lately (5 in 10 minutes).
func (s *sessions) throttled(addr string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	cut := time.Now().Add(-10 * time.Minute)
	keep := s.fail[addr][:0]
	for _, t := range s.fail[addr] {
		if t.After(cut) {
			keep = append(keep, t)
		}
	}
	if len(keep) == 0 {
		delete(s.fail, addr) // not kept for every address that ever logged in
	} else {
		s.fail[addr] = keep
	}
	return len(keep) >= 5
}

func (s *sessions) failed(addr string) {
	s.mu.Lock()
	s.fail[addr] = append(s.fail[addr], time.Now())
	s.mu.Unlock()
}

// peer is the address the connection comes from.
func peer(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func peerIsLoopback(r *http.Request) bool {
	ip := net.ParseIP(peer(r))
	return ip != nil && ip.IsLoopback()
}

// proxyHeaders are what a reverse proxy adds: IIS (Application Request Routing), nginx, Caddy, Apache.
var proxyHeaders = []string{"X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto", "Forwarded", "X-Real-Ip",
	"X-Original-Url", "X-Arr-Log-Id", "X-Arr-Ssl"}

// proxied tells whether a request came through a reverse proxy on this machine. Only a proxy here is trusted: a
// client elsewhere can write these headers, but it can not connect from 127.0.0.1.
func proxied(r *http.Request) bool {
	if !peerIsLoopback(r) {
		return false
	}
	for _, h := range proxyHeaders {
		if r.Header.Get(h) != "" {
			return true
		}
	}
	return false
}

// clientIP is the address of the browser: the peer, or what the proxy on this machine says it is.
func clientIP(r *http.Request) string {
	if !proxied(r) {
		return peer(r)
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// The last entry is the one our proxy added (the others come from the client, or proxies before it)
		parts := strings.Split(xff, ",")
		if ip := stripPort(strings.TrimSpace(parts[len(parts)-1])); ip != "" {
			return ip
		}
	}
	if ip := stripPort(strings.TrimSpace(r.Header.Get("X-Real-Ip"))); ip != "" {
		return ip
	}
	return "proxy"
}

// stripPort reads "1.2.3.4", "1.2.3.4:5678" (IIS adds the port), "[::1]:5678" or "::1".
func stripPort(s string) string {
	if ip := net.ParseIP(s); ip != nil {
		return ip.String()
	}
	if host, _, err := net.SplitHostPort(s); err == nil {
		if ip := net.ParseIP(host); ip != nil {
			return ip.String()
		}
	}
	return ""
}

// isLocal tells whether the browser runs on this machine. A request through a proxy never is: behind IIS every
// request comes from 127.0.0.1.
func isLocal(r *http.Request) bool {
	return peerIsLoopback(r) && !proxied(r)
}

// isHTTPS tells whether the browser uses HTTPS: directly, or to the proxy in front.
func isHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if !proxied(r) {
		return false
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") || r.Header.Get("X-Arr-Ssl") != "" ||
		strings.Contains(strings.ToLower(r.Header.Get("Forwarded")), "proto=https")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// SetPassword sets a user's password in users.json (the command line's way in when locked out). A missing user is
// created as an admin.
func SetPassword(dataDir, name, password string) error {
	if err := validName(name); err != nil {
		return err
	}
	if len(password) < 8 {
		return errors.New("the password needs 8 characters or more")
	}
	u, err := loadUsers(filepath.Join(dataDir, "users.json"))
	if err != nil {
		return err
	}
	old := u.get(name)
	if old == nil {
		return u.put(User{Name: name, Rights: "a"}, password)
	}
	return u.put(*old, password)
}
