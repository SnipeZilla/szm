package query

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"net"
	"syscall"
	"time"
)

// BattlEye RCon (DayZ, Arma): UDP packets "BE", a CRC32 of the rest, 0xFF, then a type: 0x00 login, 0x01 a command
// (and its answer, by sequence number), 0x02 a message of the server (each one acknowledged).

// ErrBattlEyeSilent: nothing answered on its RCon port (BattlEye opens it once the server has finished loading).
var ErrBattlEyeSilent = errors.New("BattlEye RCon: no answer")

// BattlEye sends a command to a server's BattlEye RCon and gives its answer. A command it does not answer in time
// (#shutdown: the server ends) counts as sent.
func BattlEye(addr, password, command string, timeout time.Duration) (string, error) {
	if password == "" {
		return "", errors.New("no RCON password set for this server")
	}
	conn, err := net.DialTimeout("udp", addr, timeout)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	deadline := time.Now().Add(timeout)
	_ = conn.SetDeadline(deadline)
	if _, err := conn.Write(bePacket(0x00, []byte(password))); err != nil {
		return "", err
	}
	buf := make([]byte, 65536)
	// its login answer: 0x00 then 1 (in) or 0 (wrong password)
	for {
		typ, body, err := beRead(conn, buf)
		if err != nil {
			if isTimeout(err) || isRefused(err) {
				return "", fmt.Errorf("%w on %s", ErrBattlEyeSilent, addr)
			}
			return "", err
		}
		if typ == 0x02 && len(body) > 0 {
			_, _ = conn.Write(bePacket(0x02, body[:1]))
			continue
		}
		if typ != 0x00 || len(body) < 1 {
			continue
		}
		if body[0] != 1 {
			return "", errors.New("BattlEye RCon refused: wrong password")
		}
		break
	}
	const seq = 0
	if _, err := conn.Write(bePacket(0x01, append([]byte{seq}, command...))); err != nil {
		return "", err
	}
	// its answer: one packet, or several (0x00, how many, which one) put together
	answerBy := time.Now().Add(3 * time.Second)
	if answerBy.After(deadline) {
		answerBy = deadline
	}
	_ = conn.SetDeadline(answerBy)
	var parts [][]byte
	got := 0
	for {
		typ, body, err := beRead(conn, buf)
		if err != nil {
			if isTimeout(err) {
				return string(bytes.Join(parts, nil)), nil // no answer of its own (#shutdown, say): sent
			}
			return "", err
		}
		switch {
		case typ == 0x02 && len(body) > 0:
			_, _ = conn.Write(bePacket(0x02, body[:1]))
		case typ == 0x01 && len(body) >= 1 && body[0] == seq:
			data := body[1:]
			if len(data) >= 3 && data[0] == 0x00 && data[1] > 0 && data[2] < data[1] {
				if parts == nil {
					parts = make([][]byte, data[1])
				}
				if int(data[2]) < len(parts) && parts[data[2]] == nil {
					parts[data[2]] = append([]byte{}, data[3:]...)
					got++
				}
				if got < len(parts) {
					continue
				}
				return string(bytes.Join(parts, nil)), nil
			}
			return string(data), nil
		}
	}
}

// isRefused: nothing listens there (UDP: the system said so).
func isRefused(err error) bool {
	return errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.Errno(10054)) // WSAECONNRESET, Windows' way
}

// bePacket makes a BattlEye RCon packet of a type.
func bePacket(typ byte, payload []byte) []byte {
	rest := append([]byte{0xFF, typ}, payload...)
	p := make([]byte, 6, 6+len(rest))
	p[0], p[1] = 'B', 'E'
	binary.LittleEndian.PutUint32(p[2:], crc32.ChecksumIEEE(rest))
	return append(p, rest...)
}

// beRead reads a packet: its type and what follows (packets that are not BattlEye's, or damaged, are passed over).
func beRead(conn net.Conn, buf []byte) (byte, []byte, error) {
	for {
		n, err := conn.Read(buf)
		if err != nil {
			return 0, nil, err
		}
		p := buf[:n]
		if n < 8 || p[0] != 'B' || p[1] != 'E' || p[6] != 0xFF || crc32.ChecksumIEEE(p[6:]) != binary.LittleEndian.Uint32(p[2:6]) {
			continue
		}
		return p[7], append([]byte{}, p[8:]...), nil
	}
}
