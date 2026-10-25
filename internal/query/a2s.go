// Package query talks to game servers: the Steam server query (A2S_INFO, A2S_PLAYER) and RCON, for Source and
// GoldSrc servers.
package query

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/SnipeZilla/szm/internal/netcount"
	"math"
	"strings"
	"time"
)

// Info is what a server says about itself.
type Info struct {
	Name       string `json:"name"`
	Map        string `json:"map"`
	Folder     string `json:"folder"`
	Game       string `json:"game"`
	Players    int    `json:"players"`
	MaxPlayers int    `json:"maxplayers"`
	Bots       int    `json:"bots"`
	Password   bool   `json:"password"`
	VAC        bool   `json:"vac"`
	Version    string `json:"version"`
	Ping       int    `json:"ping"` // milliseconds
}

// Player is one of the players of a server.
type Player struct {
	Name     string  `json:"name"`
	Score    int     `json:"score"`
	Duration float64 `json:"duration"` // seconds on the server
	Bot      bool    `json:"bot,omitempty"`
	Ping     int     `json:"ping,omitempty"`     // milliseconds (from RCON status)
	NoScore  bool    `json:"no_score,omitempty"` // the list came from RCON status, which has no score
}

var header = []byte{0xFF, 0xFF, 0xFF, 0xFF}

func exchange(addr string, req []byte, timeout time.Duration) ([]byte, time.Duration, error) {
	conn, err := netcount.DialTimeout("udp", addr, timeout)
	if err != nil {
		return nil, 0, err
	}
	defer conn.Close()
	start := time.Now()
	_ = conn.SetDeadline(time.Now().Add(timeout))
	if _, err := conn.Write(req); err != nil {
		return nil, 0, err
	}
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, 0, err
	}
	return buf[:n], time.Since(start), nil
}

// QueryInfo asks a server for its A2S_INFO.
func QueryInfo(addr string, timeout time.Duration) (*Info, error) {
	req := append(append([]byte{}, header...), append([]byte{'T'}, []byte("Source Engine Query\x00")...)...)
	resp, rtt, err := exchange(addr, req, timeout)
	if err != nil {
		return nil, err
	}
	// Since 2020 a server can ask for its challenge first
	if len(resp) >= 9 && bytes.Equal(resp[:4], header) && resp[4] == 'A' {
		req2 := append(req, resp[5:9]...)
		resp, rtt, err = exchange(addr, req2, timeout)
		if err != nil {
			return nil, err
		}
	}
	if len(resp) < 6 || !bytes.Equal(resp[:4], header) {
		return nil, errors.New("not a server reply")
	}
	r := &reader{b: resp[5:]}
	info := &Info{Ping: int(rtt / time.Millisecond)}
	switch resp[4] {
	case 'I': // Source, and GoldSrc servers that answer the same way
		r.byte() // protocol
		info.Name, info.Map, info.Folder, info.Game = r.str(), r.str(), r.str(), r.str()
		r.short() // app id
		info.Players, info.MaxPlayers, info.Bots = int(r.byte()), int(r.byte()), int(r.byte())
		r.byte() // type
		r.byte() // environment
		info.Password = r.byte() == 1
		info.VAC = r.byte() == 1
		info.Version = r.str()
	case 'm': // GoldSrc
		r.str() // address
		info.Name, info.Map, info.Folder, info.Game = r.str(), r.str(), r.str(), r.str()
		info.Players, info.MaxPlayers = int(r.byte()), int(r.byte())
		r.byte() // protocol
		r.byte() // type
		r.byte() // environment
		info.Password = r.byte() == 1
		if r.byte() == 1 { // a mod
			r.str()
			r.str()
			r.byte()
			r.long()
			r.long()
			r.byte()
			r.byte()
		}
		info.VAC = r.byte() == 1
		info.Bots = int(r.byte())
	default:
		return nil, fmt.Errorf("unexpected reply %q", resp[4])
	}
	if r.err {
		return nil, errors.New("short reply")
	}
	return info, nil
}

// QueryPlayers asks a server for its players (A2S_PLAYER).
func QueryPlayers(addr string, timeout time.Duration) ([]Player, error) {
	req := append(append([]byte{}, header...), 'U', 0xFF, 0xFF, 0xFF, 0xFF)
	resp, _, err := exchange(addr, req, timeout)
	if err != nil {
		return nil, err
	}
	if len(resp) >= 9 && bytes.Equal(resp[:4], header) && resp[4] == 'A' {
		req2 := append(append([]byte{}, header...), 'U')
		req2 = append(req2, resp[5:9]...)
		resp, _, err = exchange(addr, req2, timeout)
		if err != nil {
			return nil, err
		}
	}
	if len(resp) < 6 || !bytes.Equal(resp[:4], header) || resp[4] != 'D' {
		return nil, errors.New("no player list")
	}
	r := &reader{b: resp[5:]}
	n := int(r.byte())
	players := make([]Player, 0, n)
	for i := 0; i < n && !r.err; i++ {
		r.byte()
		p := Player{Name: r.str(), Score: int(int32(r.long()))}
		p.Duration = float64(math.Float32frombits(r.long()))
		if !r.err {
			players = append(players, p)
		}
	}
	return players, nil
}

type reader struct {
	b   []byte
	err bool
}

func (r *reader) byte() byte {
	if len(r.b) < 1 {
		r.err = true
		return 0
	}
	v := r.b[0]
	r.b = r.b[1:]
	return v
}

func (r *reader) short() uint16 {
	if len(r.b) < 2 {
		r.err = true
		return 0
	}
	v := binary.LittleEndian.Uint16(r.b)
	r.b = r.b[2:]
	return v
}

func (r *reader) long() uint32 {
	if len(r.b) < 4 {
		r.err = true
		return 0
	}
	v := binary.LittleEndian.Uint32(r.b)
	r.b = r.b[4:]
	return v
}

func (r *reader) str() string {
	i := bytes.IndexByte(r.b, 0)
	if i < 0 {
		r.err = true
		s := string(r.b)
		r.b = nil
		return s
	}
	s := string(r.b[:i])
	r.b = r.b[i+1:]
	return strings.ToValidUTF8(s, "?")
}
