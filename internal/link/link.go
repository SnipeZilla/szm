// Package link joins managers: one, the hub, shows the servers of the others, its nodes, on its web page.
//
// A node connects out to its hub (over the hub's web address, HTTPS on the internet): nothing to open on the
// node's firewall, and the node is not on the internet. Through that link:
//
//   - the hub sends the node the requests of its users for the node's servers (GET /api/link/feed, a stream);
//   - the node runs them on its own web interface, as that user, and sends back the answers
//     (POST /api/link/res/{id}), the body of an upload is fetched from the hub (GET /api/link/body/{id});
//   - the node sends its servers, its machine and its events every few seconds (POST /api/link/push).
//
// Each manager keeps watching, restarting, updating its own servers: when the link is down, each goes on alone.
//
// Pairing: the hub makes a code (its address, the node's ID and a secret good once, for 30 minutes); the node
// gives the secret back (POST /api/link/join) and gets its token for good. The hub keeps only hashes.
package link

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Sep joins a node's ID and the ID of one of its servers, on the hub: "us~css-1".
const Sep = "~"

// Node is a machine linked to this one (on the hub).
type Node struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	TokenHash string `json:"token_hash,omitempty"`
	PairHash  string `json:"pair_hash,omitempty"`  // while it is not linked yet: the code's secret
	PairUntil int64  `json:"pair_until,omitempty"` // ... good until (unix seconds)
	Added     int64  `json:"added"`
}

// Linked tells whether the node has joined.
func (n Node) Linked() bool { return n.TokenHash != "" }

// HubLink is the hub this manager is linked to (on a node).
type HubLink struct {
	URL   string `json:"url"`   // the hub's web address, ending with /
	ID    string `json:"id"`    // this machine's ID on the hub
	Token string `json:"token"` // its secret
	Hub   string `json:"hub"`   // the hub's name
}

// File is links.json.
type File struct {
	Name  string   `json:"name"` // this machine's name, as the others show it
	Nodes []Node   `json:"nodes"`
	Hub   *HubLink `json:"hub,omitempty"`
}

// Store keeps links.json (it holds secrets: written for the manager's user only).
type Store struct {
	path    string
	mu      sync.Mutex
	f       File
	changed chan struct{}
}

// Open reads links.json, or starts one named after the machine.
func Open(dataDir string) (*Store, error) {
	s := &Store{path: filepath.Join(dataDir, "links.json"), changed: make(chan struct{}, 1)}
	b, err := os.ReadFile(s.path)
	switch {
	case err == nil:
		if err := json.Unmarshal(b, &s.f); err != nil {
			return nil, fmt.Errorf("links.json: %w", err)
		}
	case os.IsNotExist(err):
	default:
		return nil, err
	}
	if strings.TrimSpace(s.f.Name) == "" {
		s.f.Name, _ = os.Hostname()
		if s.f.Name == "" {
			s.f.Name = "This machine"
		}
	}
	if s.f.Nodes == nil {
		s.f.Nodes = []Node{}
	}
	return s, nil
}

// Get gives a copy of the links.
func (s *Store) Get() File {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := s.f
	f.Nodes = append([]Node(nil), s.f.Nodes...)
	if s.f.Hub != nil {
		h := *s.f.Hub
		f.Hub = &h
	}
	return f
}

// Update changes the links and writes them.
func (s *Store) Update(fn func(f *File) error) error {
	s.mu.Lock()
	if err := fn(&s.f); err != nil {
		s.mu.Unlock()
		return err
	}
	b, _ := json.MarshalIndent(s.f, "", "  ")
	s.mu.Unlock()
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return err
	}
	select {
	case s.changed <- struct{}{}:
	default:
	}
	return nil
}

// Node finds a node by ID.
func (f File) Node(id string) (Node, bool) {
	for _, n := range f.Nodes {
		if n.ID == id {
			return n, true
		}
	}
	return Node{}, false
}

// User is who a request comes from, as the hub knows them: the node does what they may do.
type User struct {
	Name    string   `json:"name"`
	Rights  string   `json:"rights"`
	Servers []string `json:"servers"` // the node's servers they look after (the node's own IDs)
}

type userKey struct{}

// WithUser marks a request as coming from the hub, for a user. Only the link does it: nothing that comes over the
// network can.
func WithUser(ctx context.Context, u User) context.Context {
	return context.WithValue(ctx, userKey{}, u)
}

// UserFrom gives the hub's user of a request that came through the link.
func UserFrom(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(userKey{}).(User)
	return u, ok
}

/* ---------------------------------------------------------------------------------------------------------------
 * Codes and secrets
 * ------------------------------------------------------------------------------------------------------------- */

func secret() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func hash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func sameHash(s, h string) bool {
	return h != "" && subtle.ConstantTimeCompare([]byte(hash(s)), []byte(h)) == 1
}

// pairCode is what a node is given to join: "SZM-" and the rest, in base64.
type pairCode struct {
	URL    string `json:"u"`
	ID     string `json:"i"`
	Secret string `json:"s"`
	Hub    string `json:"h"`
}

const codePrefix = "SZM-"

func (c pairCode) String() string {
	b, _ := json.Marshal(c)
	return codePrefix + base64.RawURLEncoding.EncodeToString(b)
}

func parseCode(s string) (pairCode, error) {
	var c pairCode
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, codePrefix) {
		return c, errors.New("this is not a link code: it starts with SZM-")
	}
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(s, codePrefix))
	if err != nil || json.Unmarshal(b, &c) != nil || c.URL == "" || c.ID == "" || c.Secret == "" {
		return c, errors.New("this link code is not complete: copy it again, all of it")
	}
	return c, nil
}

var idClean = regexp.MustCompile(`[^a-z0-9]+`)

// NodeID makes an ID from a name: "US 2" -> "us-2".
func NodeID(name string, taken func(string) bool) string {
	base := strings.Trim(idClean.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if base == "" {
		base = "machine"
	}
	if len(base) > 24 {
		base = strings.Trim(base[:24], "-")
	}
	id := base
	for k := 2; taken(id); k++ {
		id = fmt.Sprintf("%s-%d", base, k)
	}
	return id
}

// HubURL checks a hub's web address: HTTPS, unless the hub is on this machine or a private network (the link
// carries RCON passwords and files: not in clear text over the internet). It ends with "/".
func HubURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", errors.New("the hub's address is a web address: https://manager.example.com/")
	}
	if u.Scheme == "http" && !privateHost(u.Hostname()) {
		return "", errors.New("on the internet the hub must be reached with HTTPS (its domain with a certificate, behind IIS, nginx or Caddy): the link carries RCON passwords and files")
	}
	u.RawQuery, u.Fragment = "", ""
	u.Path = strings.TrimSuffix(strings.TrimSuffix(u.Path, "/"), "/index.html") + "/"
	return u.String(), nil
}

// AllowLoopback lets a code point to this machine itself (localhost): for the tests, and SZM_LINK_LOOPBACK=1.
var AllowLoopback = os.Getenv("SZM_LINK_LOOPBACK") == "1"

// loopback tells whether an address is this machine itself (localhost, 127.x, ::1), which another machine can not
// reach.
func loopback(u string) bool {
	if AllowLoopback {
		return false
	}
	p, err := url.Parse(u)
	if err != nil {
		return false
	}
	h := p.Hostname()
	if strings.EqualFold(h, "localhost") || strings.HasSuffix(strings.ToLower(h), ".localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && (ip.IsLoopback() || ip.IsUnspecified())
}

func privateHost(h string) bool {
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast())
}

func newRequestID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// now is time.Now, for the tests.
var now = time.Now
