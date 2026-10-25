package query

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// A Rust-like WebSocket RCON: the password in the path, JSON messages, a log line before the answer.
func TestWebRcon(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				br := bufio.NewReader(c)
				req, err := http.ReadRequest(br)
				if err != nil {
					return
				}
				if req.URL.Path != "/s3cret pw" {
					c.Write([]byte("HTTP/1.1 401 Unauthorized\r\n\r\n"))
					return
				}
				sum := sha1.Sum([]byte(req.Header.Get("Sec-WebSocket-Key") + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
				c.Write([]byte("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: " +
					base64.StdEncoding.EncodeToString(sum[:]) + "\r\n\r\n"))
				b, err := wsRead(br) // masked by the client
				if err != nil {
					return
				}
				var m struct {
					Identifier int
					Message    string
				}
				json.Unmarshal(b, &m)
				send := func(v any) {
					p, _ := json.Marshal(v)
					hdr := []byte{0x81, byte(len(p))} // the server's frames are not masked
					c.Write(append(hdr, p...))
				}
				send(map[string]any{"Identifier": 0, "Message": "a log line"})
				send(map[string]any{"Identifier": m.Identifier, "Message": "echo: " + m.Message})
			}(c)
		}
	}()
	out, err := WebRcon(l.Addr().String(), "s3cret pw", "status", 2*time.Second)
	if err != nil || out != "echo: status" {
		t.Fatalf("%q %v", out, err)
	}
	if _, err := WebRcon(l.Addr().String(), "wrong", "status", 2*time.Second); err == nil || !strings.Contains(err.Error(), "password") {
		t.Fatal("wrong password:", err)
	}
}

// A 7 Days to Die-like telnet console: password first, then the command.
func TestTelnet(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	got := make(chan string, 1)
	go func() {
		c, err := l.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		r := bufio.NewReader(c)
		c.Write([]byte("*** Connected with 7DTD server.\r\nPlease enter password:\r\n"))
		pw, _ := r.ReadString('\n')
		if strings.TrimSpace(pw) != "tpw" {
			c.Write([]byte("Password incorrect, please enter password:\r\n"))
			return
		}
		c.Write([]byte("Logon successful.\r\n"))
		cmd, _ := r.ReadString('\n')
		got <- strings.TrimSpace(cmd)
		c.Write([]byte("Shutting server down...\r\n"))
	}()
	out, err := Telnet(l.Addr().String(), "tpw", "shutdown", 2*time.Second)
	if err != nil || !strings.Contains(out, "Shutting server down") || <-got != "shutdown" {
		t.Fatalf("%q %v", out, err)
	}
}
