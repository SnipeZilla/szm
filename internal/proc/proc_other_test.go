//go:build !windows

package proc

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestStatsGroup(t *testing.T) {
	// A script that runs a child, as srcds_run runs srcds_linux: both are counted
	p, err := Start(Spec{Exe: "/bin/sh", Args: []string{"-c", "sleep 30 & sleep 30; wait"}, Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer Kill(p.Pid)
	time.Sleep(300 * time.Millisecond)
	u, err := Stats(p.Pid)
	if err != nil || u.Memory == 0 {
		t.Fatalf("%+v %v", u, err)
	}
	if own := privateMem(p.Pid); own > 0 && u.Memory <= own {
		t.Fatalf("the children are not counted: group %d, script alone %d", u.Memory, own)
	}
}

// What a program prints goes to Output, and keeps going there when the manager is gone (the program has the file).
func TestStartOutput(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "game", "csgo", "console.log")
	os.MkdirAll(filepath.Dir(out), 0o755)
	os.WriteFile(out, []byte("before\n"), 0o644)
	p, err := Start(Spec{Exe: "/bin/sh", Args: []string{"-c", "echo out; echo err >&2"}, Dir: dir, Output: out})
	if err != nil {
		t.Fatal(err)
	}
	p.Wait()
	b, _ := os.ReadFile(out)
	if string(b) != "before\nout\nerr\n" {
		t.Fatalf("%q", b)
	}
}

func TestParseProcNetUDP(t *testing.T) {
	v4 := `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode ref pointer drops
  123: 00000000:6990 00000000:0000 07 00000000:00000000 00:00000000 00000000  1000        0 41001 2 0000000000000000 0
  124: D265A945:6991 00000000:0000 07 00000000:00000000 00:00000000 00000000  1000        0 41002 2 0000000000000000 0
`
	v6 := `  sl  local_address                         remote_address                        st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode ref pointer drops
  456: 00000000000000000000000000000000:6990 00000000000000000000000000000000:0000 07 00000000:00000000 00:00000000 00000000  1000        0 42001 2 0000000000000000 0
  457: 0000000000000000FFFF0000D265A945:6992 00000000000000000000000000000000:0000 07 00000000:00000000 00:00000000 00000000  1000        0 42002 2 0000000000000000 0
`
	got := append(parseProcNetUDP(v4), parseProcNetUDP(v6)...)
	want := []string{"0.0.0.0:27024 41001", "69.169.101.210:27025 41002", "[::]:27024 42001", "69.169.101.210:27026 42002"}
	if len(got) != len(want) {
		t.Fatal(got)
	}
	for k, e := range got {
		if s := net.JoinHostPort(e.IP.String(), strconv.Itoa(e.Port)) + " " + e.inode; s != want[k] {
			t.Errorf("%d: %s, not %s", k, s, want[k])
		}
	}
}

// A program listening on every address with an IPv6 socket (as CS2 on Linux does) is found as the port's owner.
func TestPortOwnerIPv6(t *testing.T) {
	if _, err := os.Stat("/proc/net/udp6"); err != nil {
		t.Skip("no /proc/net/udp6")
	}
	c, err := net.ListenPacket("udp", ":0") // Go: one IPv6 socket for both
	if err != nil {
		t.Skip(err)
	}
	defer c.Close()
	port := c.LocalAddr().(*net.UDPAddr).Port
	for _, ip := range []string{"", "0.0.0.0"} {
		if pid := PortOwner(ip, port); pid != os.Getpid() {
			t.Fatalf("ip %q: %d, not %d", ip, pid, os.Getpid())
		}
	}
}
