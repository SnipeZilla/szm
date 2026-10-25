package manager

import (
	"encoding/json"
	"os"
	"sync"

	"github.com/SnipeZilla/szm/internal/config"
)

// savedServer is what the manager remembers of a server between two runs (state.json): whether it should run, and
// the process it started, to find it again.
type savedServer struct {
	Desired        string `json:"desired"` // running or stopped
	PID            int    `json:"pid,omitempty"`
	Exe            string `json:"exe,omitempty"`
	Started        int64  `json:"started,omitempty"`
	LastRestartDay string `json:"last_restart_day,omitempty"`
	Hostname       string `json:"hostname,omitempty"` // as the server last said it (its query), shown while it is stopped too
	// Vanilla: started without its addons (Metamod off) while it waits for a fix of this addon package
	Vanilla string `json:"vanilla,omitempty"`
}

type stateFile struct {
	path    string
	mu      sync.Mutex
	Servers map[string]*savedServer `json:"servers"`
}

func loadState(path string) *stateFile {
	s := &stateFile{path: path, Servers: map[string]*savedServer{}}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, s)
		if s.Servers == nil {
			s.Servers = map[string]*savedServer{}
		}
	}
	return s
}

func (s *stateFile) get(id string) savedServer {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.Servers[id]; ok {
		return *v
	}
	return savedServer{}
}

func (s *stateFile) update(id string, fn func(*savedServer)) {
	s.mu.Lock()
	v, ok := s.Servers[id]
	if !ok {
		v = &savedServer{}
		s.Servers[id] = v
	}
	fn(v)
	b, err := json.MarshalIndent(s, "", "  ")
	s.mu.Unlock()
	if err == nil {
		_ = config.WriteFileAtomic(s.path, b, 0o644)
	}
}

func (s *stateFile) remove(id string) {
	s.mu.Lock()
	delete(s.Servers, id)
	b, err := json.MarshalIndent(s, "", "  ")
	s.mu.Unlock()
	if err == nil {
		_ = config.WriteFileAtomic(s.path, b, 0o644)
	}
}
