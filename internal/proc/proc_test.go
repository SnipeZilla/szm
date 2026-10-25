package proc

import (
	"encoding/binary"
	"net"
	"os"
	"testing"
)

func TestParseUDPTable(t *testing.T) {
	b := make([]byte, 4+2*12)
	binary.LittleEndian.PutUint32(b, 2)
	copy(b[4:], []byte{69, 169, 101, 211, 0x69, 0x89}) // 69.169.101.211:27017
	binary.LittleEndian.PutUint32(b[12:], 4321)
	copy(b[16:], []byte{0, 0, 0, 0, 0x69, 0x87}) // 0.0.0.0:27015
	binary.LittleEndian.PutUint32(b[24:], 99)
	es := parseUDPTable(b)
	if len(es) != 2 || es[0].Port != 27017 || es[0].PID != 4321 || es[0].IP.String() != "69.169.101.211" || es[1].Port != 27015 || !es[1].IP.IsUnspecified() {
		t.Fatalf("%+v", es)
	}
}

func TestPortOwner(t *testing.T) {
	c, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Skip(err)
	}
	defer c.Close()
	port := c.LocalAddr().(*net.UDPAddr).Port
	if pid := PortOwner("127.0.0.1", port); pid != os.Getpid() {
		t.Fatalf("owner %d, want %d", pid, os.Getpid())
	}
	if pid := PortOwner("10.9.8.7", port); pid != 0 {
		t.Fatalf("other IP matched: %d", pid)
	}
	if !NameMatches(ProcessName(os.Getpid()), []string{"proc.test", "srcds.exe"}) {
		t.Fatalf("name %q", ProcessName(os.Getpid()))
	}
}

func TestParseTCPTable(t *testing.T) {
	b := make([]byte, 4+24*2)
	b[0] = 2
	// row 1: 0.0.0.0:27015, PID 2864
	r := 4
	b[r+8], b[r+9] = 0x69, 0x87
	b[r+20], b[r+21] = 0x30, 0x0b
	// row 2: 69.169.101.212:27015, PID 77
	r = 28
	copy(b[r+4:], []byte{69, 169, 101, 212})
	b[r+8], b[r+9] = 0x69, 0x87
	b[r+20] = 77
	got := parseTCPTable(b)
	if len(got) != 2 || got[0].Port != 27015 || got[0].PID != 2864 || !got[0].IP.IsUnspecified() ||
		got[1].IP.String() != "69.169.101.212" || got[1].PID != 77 {
		t.Fatalf("%+v", got)
	}
}
