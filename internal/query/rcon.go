package query

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/SnipeZilla/szm/internal/netcount"
	"io"
	"net"
	"regexp"
	"strings"
	"time"
)

// ErrBadPassword is the answer of a server to a wrong RCON password.
var ErrBadPassword = errors.New("wrong RCON password")

// Rcon sends a command to a server and gives its answer. GoldSrc servers speak RCON over UDP, the others over TCP.
func Rcon(addr, password, command string, goldsrc bool, timeout time.Duration) (string, error) {
	if password == "" {
		return "", errors.New("no RCON password set for this server")
	}
	if goldsrc {
		return goldsrcRcon(addr, password, command, timeout)
	}
	return sourceRcon(addr, password, command, timeout)
}

const (
	typeAuth         = 3
	typeAuthResponse = 2
	typeExec         = 2
	typeResponse     = 0
)

func writePacket(w io.Writer, id, typ int32, body string) error {
	size := int32(4 + 4 + len(body) + 2)
	buf := new(bytes.Buffer)
	_ = binary.Write(buf, binary.LittleEndian, size)
	_ = binary.Write(buf, binary.LittleEndian, id)
	_ = binary.Write(buf, binary.LittleEndian, typ)
	buf.WriteString(body)
	buf.Write([]byte{0, 0})
	_, err := w.Write(buf.Bytes())
	return err
}

// Timing of a Source RCON answer, as HLstatsZ does it (includes/class_rcon.php)
const (
	rconConnect = 8 * time.Second        // the system resends an unanswered SYN after about 1, 3 and 7 s
	rconIdle    = 400 * time.Millisecond // quiet this long: the answer is over (or has not started)
	rconGrace   = 100 * time.Millisecond // after the end marker came back: what is still on its way
)

// rconConn reads whole RCON packets, keeping what arrived of the next one across deadlines.
type rconConn struct {
	c      net.Conn
	buf    []byte
	closed bool
}

type rconPacket struct {
	id, typ int32
	body    string
}

// read gives the next packet, or nil when nothing came before the deadline or the server closed the connection.
func (r *rconConn) read(until time.Time) (*rconPacket, error) {
	chunk := make([]byte, 8192)
	for {
		if len(r.buf) >= 4 {
			size := int(int32(binary.LittleEndian.Uint32(r.buf)))
			if size < 8 || size > 1<<20 {
				return nil, fmt.Errorf("bad RCON packet (%d bytes): is this a Source server?", size)
			}
			if len(r.buf) >= size+4 {
				p := &rconPacket{
					id:   int32(binary.LittleEndian.Uint32(r.buf[4:8])),
					typ:  int32(binary.LittleEndian.Uint32(r.buf[8:12])),
					body: string(bytes.TrimRight(r.buf[12:size+4], "\x00")),
				}
				r.buf = r.buf[size+4:]
				return p, nil
			}
		}
		if r.closed || !time.Now().Before(until) {
			return nil, nil
		}
		_ = r.c.SetReadDeadline(until)
		n, err := r.c.Read(chunk)
		r.buf = append(r.buf, chunk[:n]...)
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				if n == 0 {
					return nil, nil
				}
				continue
			}
			r.closed = true // EOF, reset
			if n == 0 {
				return nil, nil
			}
		}
	}
}

func sourceRcon(addr, password, command string, timeout time.Duration) (string, error) {
	start := time.Now()
	conn, err := netcount.DialTimeout("tcp", addr, rconConnect)
	if err != nil {
		if time.Since(start) >= rconConnect-500*time.Millisecond {
			return "", fmt.Errorf("no answer from %s within %s (TCP): a firewall, packet loss, or the server is not listening", addr, rconConnect)
		}
		return "", err
	}
	defer conn.Close()
	_ = conn.SetWriteDeadline(time.Now().Add(timeout + rconConnect))
	r := &rconConn{c: conn}

	if err := writePacket(conn, 1, typeAuth, password); err != nil {
		return "", err
	}
	// The server answers an empty response, then the result of the login
	until := time.Now().Add(timeout)
	for {
		p, err := r.read(until)
		if err != nil {
			return "", err
		}
		if p == nil {
			if r.closed {
				return "", errors.New("the server closed the connection at the login: too many failed attempts can get the manager's IP banned (sv_rcon_banpenalty)")
			}
			return "", errors.New("no answer to the RCON login (timed out)")
		}
		if p.typ == typeAuthResponse {
			if p.id == -1 {
				return "", ErrBadPassword
			}
			break
		}
	}

	if err := writePacket(conn, 2, typeExec, command); err != nil {
		return "", err
	}
	// The end marker (an empty response, which the server sends back after the whole answer) goes only once the
	// answer has started, or after a quiet moment: CS2 sends it back at once, before the command has even run.
	var out strings.Builder
	ended := false
	end := func() {
		_ = writePacket(conn, 3, typeResponse, "")
		ended = true
	}
	until = time.Now().Add(rconIdle)
	for {
		p, err := r.read(until)
		if err != nil {
			if out.Len() > 0 {
				break
			}
			return "", err
		}
		if p == nil {
			if ended || r.closed {
				break
			}
			end()
			until = time.Now().Add(timeout)
			continue
		}
		if p.id == 3 {
			until = time.Now().Add(rconGrace)
			continue
		}
		if p.typ == typeResponse {
			out.WriteString(p.body)
		}
		if !ended {
			end()
		}
		until = time.Now().Add(rconIdle)
	}
	return strings.TrimRight(out.String(), "\n"), nil
}

var challengeRe = regexp.MustCompile(`challenge rcon (\d+)`)

func goldsrcRcon(addr, password, command string, timeout time.Duration) (string, error) {
	conn, err := netcount.DialTimeout("udp", addr, timeout)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))

	if _, err := conn.Write(append(append([]byte{}, header...), []byte("challenge rcon\n")...)); err != nil {
		return "", err
	}
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return "", err
	}
	m := challengeRe.FindSubmatch(buf[:n])
	if m == nil {
		return "", errors.New("the server gave no RCON challenge")
	}
	msg := fmt.Sprintf("rcon %s \"%s\" %s\n", m[1], password, command)
	if _, err := conn.Write(append(append([]byte{}, header...), []byte(msg)...)); err != nil {
		return "", err
	}

	var out strings.Builder
	for {
		_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		n, err := conn.Read(buf)
		if err != nil {
			break
		}
		b := buf[:n]
		if bytes.HasPrefix(b, header) {
			b = b[4:]
			if len(b) > 0 && b[0] == 'l' {
				b = b[1:]
			}
		}
		out.Write(bytes.TrimRight(b, "\x00"))
	}
	text := strings.TrimRight(out.String(), "\n")
	if strings.Contains(text, "Bad rcon_password") {
		return "", ErrBadPassword
	}
	return text, nil
}
