package query

import (
	"bufio"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"time"
)

// The RCON of games of other engines: Rust's WebSocket RCON, 7 Days to Die's telnet console.

// WebRcon sends a command to a Rust server (rcon.web 1: ws://ip:port/password) and gives its answer.
func WebRcon(addr, password, command string, timeout time.Duration) (string, error) {
	if password == "" {
		return "", errors.New("no RCON password set for this server")
	}
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))
	key := make([]byte, 16)
	_, _ = rand.Read(key)
	k := base64.StdEncoding.EncodeToString(key)
	req := "GET /" + url.PathEscape(password) + " HTTP/1.1\r\nHost: " + addr + "\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + k + "\r\nSec-WebSocket-Version: 13\r\n\r\n"
	if _, err := io.WriteString(conn, req); err != nil {
		return "", err
	}
	br := bufio.NewReader(conn)
	status, err := br.ReadString('\n')
	if err != nil {
		return "", err
	}
	if !strings.Contains(status, " 101 ") {
		if strings.Contains(status, " 401 ") || strings.Contains(status, " 403 ") {
			return "", errors.New("RCON refused: wrong password")
		}
		return "", fmt.Errorf("RCON: %s", strings.TrimSpace(status))
	}
	accept := ""
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return "", err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		if h, v, ok := strings.Cut(line, ":"); ok && strings.EqualFold(strings.TrimSpace(h), "Sec-WebSocket-Accept") {
			accept = strings.TrimSpace(v)
		}
	}
	sum := sha1.Sum([]byte(k + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	if accept != base64.StdEncoding.EncodeToString(sum[:]) {
		return "", errors.New("RCON: not a WebSocket answer")
	}
	const id = 7301
	msg, _ := json.Marshal(map[string]any{"Identifier": id, "Message": command, "Name": "SnipeZilla"})
	if err := wsWrite(conn, msg); err != nil {
		return "", err
	}
	// Its answer carries the command's Identifier; what the server logs meanwhile carries another
	for {
		b, err := wsRead(br)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return "", nil // it ended (quit)
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				return "", nil // no answer of its own (quit, say): sent
			}
			return "", err
		}
		var r struct {
			Message    string `json:"Message"`
			Identifier int    `json:"Identifier"`
		}
		if json.Unmarshal(b, &r) == nil && r.Identifier == id {
			return r.Message, nil
		}
	}
}

// wsWrite writes a text frame (masked, as a client must).
func wsWrite(w io.Writer, payload []byte) error {
	var hdr []byte
	n := len(payload)
	switch {
	case n < 126:
		hdr = []byte{0x81, 0x80 | byte(n)}
	case n < 1<<16:
		hdr = []byte{0x81, 0x80 | 126, byte(n >> 8), byte(n)}
	default:
		hdr = make([]byte, 10)
		hdr[0], hdr[1] = 0x81, 0x80|127
		binary.BigEndian.PutUint64(hdr[2:], uint64(n))
	}
	mask := make([]byte, 4)
	_, _ = rand.Read(mask)
	out := append(hdr, mask...)
	for k, c := range payload {
		out = append(out, c^mask[k%4])
	}
	_, err := w.Write(out)
	return err
}

// wsRead reads a message (text frames; control frames passed over; a close frame ends it).
func wsRead(r *bufio.Reader) ([]byte, error) {
	var msg []byte
	for {
		h := make([]byte, 2)
		if _, err := io.ReadFull(r, h); err != nil {
			return nil, err
		}
		fin, op := h[0]&0x80 != 0, h[0]&0x0F
		n := uint64(h[1] & 0x7F)
		switch n {
		case 126:
			b := make([]byte, 2)
			if _, err := io.ReadFull(r, b); err != nil {
				return nil, err
			}
			n = uint64(binary.BigEndian.Uint16(b))
		case 127:
			b := make([]byte, 8)
			if _, err := io.ReadFull(r, b); err != nil {
				return nil, err
			}
			n = binary.BigEndian.Uint64(b)
		}
		if n > 16<<20 {
			return nil, errors.New("RCON: message too long")
		}
		var mask []byte
		if h[1]&0x80 != 0 {
			mask = make([]byte, 4)
			if _, err := io.ReadFull(r, mask); err != nil {
				return nil, err
			}
		}
		b := make([]byte, n)
		if _, err := io.ReadFull(r, b); err != nil {
			return nil, err
		}
		for k := range b {
			if mask != nil {
				b[k] ^= mask[k%4]
			}
		}
		switch op {
		case 0x8:
			return nil, io.EOF
		case 0x9, 0xA:
			continue
		}
		msg = append(msg, b...)
		if fin {
			return msg, nil
		}
	}
}

// Telnet sends a command to a 7 Days to Die server's telnet console (its password first, when it asks for one) and
// gives what it printed in the second after.
func Telnet(addr, password, command string, timeout time.Duration) (string, error) {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	read := func(d time.Duration, until func(string) bool) string {
		var sb strings.Builder
		buf := make([]byte, 4096)
		end := time.Now().Add(d)
		for time.Now().Before(end) {
			_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
			n, err := conn.Read(buf)
			sb.Write(buf[:n])
			if until != nil && until(sb.String()) {
				break
			}
			if err != nil && !isTimeout(err) {
				break
			}
		}
		return sb.String()
	}
	greet := read(2*time.Second, func(s string) bool { return strings.Contains(strings.ToLower(s), "password:") })
	if strings.Contains(strings.ToLower(greet), "password:") {
		if password == "" {
			return "", errors.New("its telnet asks for a password: set its RCON password")
		}
		_ = conn.SetWriteDeadline(time.Now().Add(timeout))
		if _, err := io.WriteString(conn, password+"\r\n"); err != nil {
			return "", err
		}
		ok := read(3*time.Second, func(s string) bool {
			l := strings.ToLower(s)
			return strings.Contains(l, "logon successful") || strings.Contains(l, "password incorrect") || strings.Contains(l, "password:")
		})
		if !strings.Contains(strings.ToLower(ok), "logon successful") {
			return "", errors.New("telnet refused: wrong password")
		}
	}
	_ = conn.SetWriteDeadline(time.Now().Add(timeout))
	if _, err := io.WriteString(conn, command+"\r\n"); err != nil {
		return "", err
	}
	return strings.TrimSpace(read(time.Second, nil)), nil
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}
