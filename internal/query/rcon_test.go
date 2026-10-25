package query

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

// fakeServer answers RCON like a game server. mode: "cs2" sends the end marker back at once and the answer 80 ms
// later; "source" the answer at once, then the marker; "nomarker" never sends the marker back.
func fakeServer(t *testing.T, mode, password, answer string) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		c, err := l.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		read := func() (int32, int32, string, error) {
			var size int32
			if err := binary.Read(c, binary.LittleEndian, &size); err != nil {
				return 0, 0, "", err
			}
			b := make([]byte, size)
			if _, err := io.ReadFull(c, b); err != nil {
				return 0, 0, "", err
			}
			return int32(binary.LittleEndian.Uint32(b)), int32(binary.LittleEndian.Uint32(b[4:])), string(bytes.TrimRight(b[8:], "\x00")), nil
		}
		send := func(id, typ int32, body string) { _ = writePacket(c, id, typ, body) }
		id, _, pw, err := read()
		if err != nil {
			return
		}
		send(id, typeResponse, "")
		if pw != password {
			send(-1, typeAuthResponse, "")
			return
		}
		send(id, typeAuthResponse, "")
		cmdID, _, _, err := read()
		if err != nil {
			return
		}
		half := len(answer) / 2
		switch mode {
		case "source":
			send(cmdID, typeResponse, answer[:half])
			send(cmdID, typeResponse, answer[half:])
			if eid, _, _, err := read(); err == nil {
				send(eid, typeResponse, "")
				send(eid, typeResponse, "\x00\x00\x00\x01\x00\x00\x00\x00")
			}
		case "cs2":
			go func() {
				if eid, _, _, err := read(); err == nil {
					send(eid, typeResponse, "") // at once
				}
			}()
			time.Sleep(80 * time.Millisecond)
			send(cmdID, typeResponse, answer[:half])
			time.Sleep(30 * time.Millisecond)
			send(cmdID, typeResponse, answer[half:])
			time.Sleep(time.Second)
		case "nomarker":
			time.Sleep(50 * time.Millisecond)
			send(cmdID, typeResponse, answer)
			time.Sleep(2 * time.Second)
		}
	}()
	return l.Addr().String()
}

func TestSourceRcon(t *testing.T) {
	answer := "hostname: SnipeZilla\n---------players--------\n#end\n"
	for _, mode := range []string{"source", "cs2", "nomarker"} {
		t.Run(mode, func(t *testing.T) {
			start := time.Now()
			got, err := Rcon(fakeServer(t, mode, "pw", answer), "pw", "status", false, 3*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			if got != strings.TrimRight(answer, "\n") {
				t.Fatalf("got %q", got)
			}
			if d := time.Since(start); d > 2*time.Second {
				t.Errorf("took %s", d)
			}
		})
	}
	if _, err := Rcon(fakeServer(t, "source", "pw", answer), "bad", "status", false, 3*time.Second); err != ErrBadPassword {
		t.Fatalf("wrong password: %v", err)
	}
}
