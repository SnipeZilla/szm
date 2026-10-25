//go:build linux

package netmon

import (
	"encoding/binary"
	"net"
	"os"
	"testing"
	"time"
)

func udp4(src, dst string, sport, dport uint16, data int) []byte {
	b := make([]byte, 28)
	b[0] = 0x45
	binary.BigEndian.PutUint16(b[2:], uint16(28+data))
	b[9] = 17
	copy(b[12:], net.ParseIP(src).To4())
	copy(b[16:], net.ParseIP(dst).To4())
	binary.BigEndian.PutUint16(b[20:], sport)
	binary.BigEndian.PutUint16(b[22:], dport)
	return b
}

func TestParsePacket(t *testing.T) {
	p, ok := parsePacket(udp4("1.2.3.4", "5.6.7.8", 27005, 27015, 1200))
	if !ok || p.proto != 17 || p.sport != 27005 || p.dport != 27015 || p.size != 1200 || !p.dst.Equal(net.ParseIP("5.6.7.8")) {
		t.Fatalf("%+v %v", p, ok)
	}
	// TCP over IPv6, 32-byte header (options), 100 bytes of data
	b := make([]byte, 40+32)
	b[0] = 0x60
	binary.BigEndian.PutUint16(b[4:], 32+100)
	b[6] = 6
	copy(b[8:], net.ParseIP("2001:db8::1"))
	copy(b[24:], net.ParseIP("2001:db8::2"))
	binary.BigEndian.PutUint16(b[40:], 27015)
	binary.BigEndian.PutUint16(b[42:], 50000)
	b[40+12] = 8 << 4
	p, ok = parsePacket(b)
	if !ok || p.proto != 6 || p.sport != 27015 || p.size != 100 {
		t.Fatalf("%+v %v", p, ok)
	}
	frag := udp4("1.2.3.4", "5.6.7.8", 1, 2, 10)
	binary.BigEndian.PutUint16(frag[6:], 100) // a later fragment
	if _, ok := parsePacket(frag); ok {
		t.Fatal("fragment read")
	}
	if _, ok := parsePacket([]byte{0x45, 0}); ok {
		t.Fatal("short packet read")
	}
}

func TestParseProcNet(t *testing.T) {
	ip, port, inode, ok := parseProcNet("  12: 0100007F:6987 00000000:0000 07 00000000:00000000 00:00000000 00000000  1000        0 424242 2 0000000000000000 0")
	if !ok || !ip.Equal(net.ParseIP("127.0.0.1")) || port != 27015 || inode != "424242" {
		t.Fatal(ip, port, inode, ok)
	}
	ip, _, _, ok = parseProcNet("   0: 0000000000000000FFFF00000100007F:6987 00000000000000000000000000000000:0000 07 00000000:00000000 00:00000000 00000000  1000        0 1 2 0 0")
	if !ok || !ip.Equal(net.ParseIP("127.0.0.1")) {
		t.Fatal(ip, ok)
	}
	ip, _, _, ok = parseProcNet("   0: 00000000000000000000000000000000:6987 00000000000000000000000000000000:0000 07 00000000:00000000 00:00000000 00000000  1000        0 1 2 0 0")
	if !ok || !ip.IsUnspecified() {
		t.Fatal(ip, ok)
	}
}

func TestLookup(t *testing.T) {
	pm := portMap{{17, 27015}: {{nil, 10}, {net.ParseIP("10.0.0.2"), 20}}}
	if pm.lookup(17, net.ParseIP("10.0.0.2"), 27015) != 20 || pm.lookup(17, net.ParseIP("10.0.0.3"), 27015) != 10 ||
		pm.lookup(6, net.ParseIP("10.0.0.2"), 27015) != 0 {
		t.Fatal("lookup")
	}
}

// As root: this test's own UDP traffic on 127.0.0.1 is counted for its PID.
func TestLiveCount(t *testing.T) {
	m, err := Start()
	if err != nil {
		t.Skip(err)
	}
	defer m.Close()
	srv, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	cl, _ := net.DialUDP("udp4", nil, srv.LocalAddr().(*net.UDPAddr))
	defer cl.Close()
	pid := uint32(os.Getpid())
	m.Watch([]uint32{pid})
	time.Sleep(portRefresh + 500*time.Millisecond) // its ports are found
	buf := make([]byte, 2000)
	for k := 0; k < 50; k++ {
		_, _ = cl.Write(make([]byte, 1000))
		_, from, _ := srv.ReadFromUDP(buf)
		_, _ = srv.WriteToUDP(make([]byte, 500), from)
		_, _ = cl.Read(buf)
	}
	time.Sleep(300 * time.Millisecond)
	c := (*m.watched.Load())[pid]
	in, out := c.in.Load(), c.out.Load()
	// both ends are this process: each packet is sent (out) and received (in) by it
	if in != 50*1500 || out != 50*1500 {
		t.Fatalf("in %d out %d (want 75000 each)", in, out)
	}
	if m.Err() != nil {
		t.Fatal(m.Err())
	}
}
