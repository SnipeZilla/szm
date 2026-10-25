package manager

import (
	"regexp"
	"time"

	"github.com/SnipeZilla/szm/internal/config"
)

// SentCommand is a command sent to a server through RCON, kept for its crash report: RCON commands do not show in
// the server's console, yet one is often what made it crash.
type SentCommand struct {
	Time    time.Time `json:"time"`
	By      string    `json:"by,omitempty"`      // the user who typed it; "" for the manager's own
	Command string    `json:"command"`           // secrets hidden
	Routine bool      `json:"routine,omitempty"` // the manager's regular check (status, for the players)
	Error   string    `json:"error,omitempty"`   // it got no answer
}

const keepCommands = 40

// secretArgRe: what a command gives that must not be kept (passwords, the login token)
var secretArgRe = regexp.MustCompile(`(?i)^(\s*(rcon_password|sv_password|sv_setsteamaccount)\s+).*$`)

// noteCommand keeps a command sent to the server (the lock must not be held).
func (i *Instance) noteCommand(by, command string, routine bool, err error) {
	c := SentCommand{Time: time.Now(), By: by, Command: config.Redact(secretArgRe.ReplaceAllString(command, "${1}••••••")), Routine: routine}
	if err != nil {
		c.Error = err.Error()
	}
	i.mu.Lock()
	i.sent = append(i.sent, c)
	if len(i.sent) > keepCommands {
		i.sent = append([]SentCommand(nil), i.sent[len(i.sent)-keepCommands:]...)
	}
	i.mu.Unlock()
}

// sentSince: the commands sent since t (the lock is held).
func (i *Instance) sentSince(t time.Time) []SentCommand {
	var out []SentCommand
	for _, c := range i.sent {
		if !c.Time.Before(t) {
			out = append(out, c)
		}
	}
	return out
}
