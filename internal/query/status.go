package query

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	// CS2: "    2    01:23   12    0     active 786432 1.2.3.4:27005 'Name'" / "    3      BOT    0    0     active      0 'Bot'"
	s2Name = regexp.MustCompile(`'(.*)'\s*$`)
	// Source: `#      2 "Name" STEAM_1:0:123 05:12 50 0 active 1.2.3.4:27005` / `#      3 "Bot" BOT active`
	s1Line = regexp.MustCompile(`^#\s*\d+\s+(?:\d+\s+)?"(.*)"\s+(\S+)\s*(.*)$`)
)

// ParseStatus reads the players out of the answer to RCON "status", for Source 2 (CS2) and Source servers. ok is
// false when the answer has no player list at all (not a status answer).
func ParseStatus(text string) (players []Player, ok bool) {
	lines := strings.Split(strings.ReplaceAll(text, "\r", ""), "\n")
	inS2 := false
	for _, l := range lines {
		t := strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(t, "---------players"):
			inS2, ok = true, true
			continue
		case strings.HasPrefix(t, "#end"):
			inS2 = false
			continue
		case strings.HasPrefix(t, "# userid"):
			ok = true
			continue
		}
		if inS2 {
			if p, good := parseS2(t); good {
				players = append(players, p)
			}
			continue
		}
		if m := s1Line.FindStringSubmatch(t); m != nil {
			ok = true
			p := Player{Name: m[1], NoScore: true}
			if strings.EqualFold(m[2], "BOT") {
				p.Bot = true
			} else if f := strings.Fields(m[3]); len(f) >= 2 {
				p.Duration = clock(f[0])
				p.Ping, _ = strconv.Atoi(f[1])
			}
			players = append(players, p)
		}
	}
	return players, ok
}

func parseS2(t string) (Player, bool) {
	f := strings.Fields(t)
	if len(f) < 3 {
		return Player{}, false
	}
	id, err := strconv.Atoi(f[0])
	if err != nil || id == 65535 { // the header line, and the empty slot CS2 lists as [NoChan]
		return Player{}, false
	}
	m := s2Name.FindStringSubmatch(t)
	if m == nil {
		return Player{}, false
	}
	p := Player{Name: m[1], NoScore: true}
	if strings.EqualFold(f[1], "BOT") {
		p.Bot = true
		return p, true
	}
	if strings.Contains(t, "[NoChan]") {
		return Player{}, false
	}
	p.Duration = clock(f[1])
	p.Ping, _ = strconv.Atoi(f[2])
	return p, true
}

// clock reads "mm:ss" or "hh:mm:ss" as seconds.
func clock(s string) float64 {
	var total float64
	for _, part := range strings.Split(s, ":") {
		n, err := strconv.Atoi(part)
		if err != nil {
			return 0
		}
		total = total*60 + float64(n)
	}
	return total
}
