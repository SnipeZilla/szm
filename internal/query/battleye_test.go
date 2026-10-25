package query

import (
	"encoding/binary"
	"errors"
	"hash/crc32"
	"net"
	"strings"
	"testing"
	"time"
)

// A BattlEye RCon like DayZ's: login, a message of its own before the answer (to acknowledge), an answer in two
// packets, a command it does not answer (#shutdown), a wrong password.
func TestBattlEye(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	acked := make(chan byte, 4)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, from, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			p := buf[:n]
			if n < 8 || string(p[:2]) != "BE" || crc32.ChecksumIEEE(p[6:]) != binary.LittleEndian.Uint32(p[2:6]) || p[6] != 0xFF {
				t.Error("bad packet")
				continue
			}
			send := func(typ byte, b ...byte) { pc.WriteTo(bePacket(typ, b), from) }
			switch typ, body := p[7], p[8:]; typ {
			case 0x00:
				if string(body) == "pw42" {
					send(0x00, 1)
				} else {
					send(0x00, 0)
				}
			case 0x02:
				acked <- body[0]
			case 0x01:
				seq, cmd := body[0], string(body[1:])
				switch cmd {
				case "players":
					send(0x02, append([]byte{7}, "RCon admin #0 logged in"...)...)
					time.Sleep(50 * time.Millisecond)
					send(0x01, append([]byte{seq, 0x00, 2, 1}, " 1 Bob"...)...)
					send(0x01, append([]byte{seq, 0x00, 2, 0}, "Players on server:\n"...)...)
				case "say -1 hi":
					send(0x01, seq)
				case "#shutdown":
					// it ends: no answer
				}
			}
		}
	}()
	addr := pc.LocalAddr().String()
	out, err := BattlEye(addr, "pw42", "players", 5*time.Second)
	if err != nil || out != "Players on server:\n 1 Bob" {
		t.Fatalf("%q %v", out, err)
	}
	select {
	case s := <-acked:
		if s != 7 {
			t.Fatal(s)
		}
	case <-time.After(time.Second):
		t.Fatal("its message was not acknowledged")
	}
	if out, err := BattlEye(addr, "pw42", "say -1 hi", 5*time.Second); err != nil || out != "" {
		t.Fatalf("%q %v", out, err)
	}
	start := time.Now()
	if _, err := BattlEye(addr, "pw42", "#shutdown", 5*time.Second); err != nil || time.Since(start) > 4*time.Second {
		t.Fatal(err, time.Since(start))
	}
	if _, err := BattlEye(addr, "nope", "players", 5*time.Second); err == nil || !strings.Contains(err.Error(), "wrong password") {
		t.Fatal(err)
	}
	if _, err := BattlEye(addr, "", "players", time.Second); err == nil {
		t.Fatal("no password")
	}
	// nobody there
	dead, _ := net.ListenPacket("udp", "127.0.0.1:0")
	da := dead.LocalAddr().String()
	dead.Close()
	if _, err := BattlEye(da, "pw42", "players", time.Second); !errors.Is(err, ErrBattlEyeSilent) {
		t.Fatal("nobody", err)
	}
}
