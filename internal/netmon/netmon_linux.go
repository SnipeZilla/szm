//go:build linux

package netmon

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

// On Linux the kernel does not count the network per program. The manager watches every packet the machine sends
// and receives (an AF_PACKET socket, as tcpdump and nethogs do: it needs root, or CAP_NET_RAW), reads its addresses
// and ports, and gives it to the server whose processes own that port. Only the first bytes of each packet are
// copied (its headers); the sizes counted are those of the data, as Windows counts them.

const snapLen = 96 // the IP and TCP/UDP headers are enough

const (
	packetHost      = 0 // for this machine
	packetBroadcast = 1
	packetMulticast = 2
	packetOutgoing  = 4
)

// portRefresh: how often the servers' ports are looked up again when the servers watched changed (one started), and
// portRecheck when they did not (a server opening a port late)
const (
	portRefresh = 2 * time.Second
	portRecheck = 15 * time.Second
)

// Start begins counting. It fails without root (or CAP_NET_RAW).
func Start() (*Monitor, error) {
	fd, err := syscall.Socket(syscall.AF_PACKET, syscall.SOCK_DGRAM|syscall.SOCK_CLOEXEC, int(htons(syscall.ETH_P_ALL)))
	if err != nil {
		if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
			return nil, errors.New("Linux lets only root, or a program given the right CAP_NET_RAW, see the network: run szm as root, or give it the right once (sudo setcap cap_net_raw+ep szm, again after each new szm), or use the systemd service of deploy/linux")
		}
		return nil, fmt.Errorf("Linux did not open a packet socket: %w", err)
	}
	// keep only the headers of each packet: the kernel copies 96 bytes, not the whole packet
	_ = syscall.AttachLsf(fd, []syscall.SockFilter{{Code: 0x06, K: snapLen}}) // BPF_RET|BPF_K
	_ = syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_RCVBUF, 4<<20)
	tv := syscall.Timeval{Sec: 1}
	_ = syscall.SetsockoptTimeval(fd, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, &tv)
	dropAmbient()

	m := newMonitor()
	var ports atomic.Pointer[portMap]
	ports.Store(&portMap{})
	var built *map[uint32]*counter
	var builtAt time.Time
	refresh := func() {
		w := m.watched.Load()
		if w == built && time.Since(builtAt) < portRecheck {
			return
		}
		ports.Store(buildPortMap(*w))
		built, builtAt = w, time.Now()
	}
	refresh()
	go func() {
		t := time.NewTicker(portRefresh)
		defer t.Stop()
		for {
			select {
			case <-m.quit:
				return
			case <-t.C:
				refresh()
			}
		}
	}()
	go func() {
		defer close(m.ended)
		buf := make([]byte, snapLen)
		for {
			select {
			case <-m.quit:
				return
			default:
			}
			n, from, err := syscall.Recvfrom(fd, buf, 0)
			if err != nil {
				if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EINTR) {
					continue
				}
				select {
				case <-m.quit:
				default:
					m.setErr(fmt.Errorf("Linux stopped the network count (%v)", err))
				}
				return
			}
			ll, ok := from.(*syscall.SockaddrLinklayer)
			if !ok {
				continue
			}
			var out bool
			switch ll.Pkttype {
			case packetOutgoing:
				out = true
			case packetHost, packetBroadcast, packetMulticast:
			default:
				continue // for another machine (promiscuous)
			}
			p, ok := parsePacket(buf[:n])
			if !ok {
				continue
			}
			ip, port := p.dst, p.dport
			if out {
				ip, port = p.src, p.sport
			}
			m.count(ports.Load().lookup(p.proto, ip, port), p.size, out)
		}
	}()
	m.stopOS = func() {
		select {
		case <-m.ended:
		case <-time.After(3 * time.Second):
		}
		syscall.Close(fd)
	}
	go m.sampler()
	return m, nil
}

func htons(v uint16) uint16 { return v<<8 | v>>8 }

/* ---------- Packets ---------- */

type packet struct {
	proto        uint8 // 6 TCP, 17 UDP
	src, dst     net.IP
	sport, dport uint16
	size         uint32 // bytes of data (without the IP and TCP/UDP headers)
}

// parsePacket reads the headers of an IPv4 or IPv6 packet carrying TCP or UDP.
func parsePacket(b []byte) (packet, bool) {
	var p packet
	if len(b) < 1 {
		return p, false
	}
	var t []byte
	var ipLen, ipHdr int
	switch b[0] >> 4 {
	case 4:
		if len(b) < 20 {
			return p, false
		}
		ipHdr = int(b[0]&0x0f) * 4
		ipLen = int(binary.BigEndian.Uint16(b[2:4]))
		if binary.BigEndian.Uint16(b[6:8])&0x1fff != 0 || ipHdr < 20 {
			return p, false // a later fragment: no ports
		}
		p.proto, p.src, p.dst = b[9], net.IP(b[12:16]), net.IP(b[16:20])
	case 6:
		if len(b) < 40 {
			return p, false
		}
		ipHdr = 40
		ipLen = 40 + int(binary.BigEndian.Uint16(b[4:6]))
		p.proto, p.src, p.dst = b[6], net.IP(b[8:24]), net.IP(b[24:40])
	default:
		return p, false
	}
	if len(b) < ipHdr+4 {
		return p, false
	}
	t = b[ipHdr:]
	p.sport, p.dport = binary.BigEndian.Uint16(t[0:2]), binary.BigEndian.Uint16(t[2:4])
	hdr := 0
	switch p.proto {
	case 17:
		hdr = 8
	case 6:
		if len(t) < 13 {
			return p, false
		}
		hdr = int(t[12]>>4) * 4
	default:
		return p, false
	}
	if d := ipLen - ipHdr - hdr; d > 0 {
		p.size = uint32(d)
	}
	return p, true
}

/* ---------- Which process owns which port ---------- */

type portKey struct {
	proto uint8
	port  uint16
}

type portOwner struct {
	ip  net.IP // nil: every address
	pid uint32 // the watched process (the server), even when a child of it owns the socket
}

type portMap map[portKey][]portOwner

// lookup gives the server owning a local port (0: none). An owner of that very address wins over one of every
// address.
func (pm portMap) lookup(proto uint8, ip net.IP, port uint16) uint32 {
	var any uint32
	for _, o := range pm[portKey{proto, port}] {
		if o.ip == nil {
			any = o.pid
		} else if o.ip.Equal(ip) {
			return o.pid
		}
	}
	return any
}

// buildPortMap finds the sockets of the watched servers and of their children (srcds_run starts srcds_linux), and
// their local ports.
func buildPortMap(watched map[uint32]*counter) *portMap {
	pm := portMap{}
	if len(watched) == 0 {
		return &pm
	}
	owner := familyOf(watched)
	inodes := map[string]uint32{} // socket inode -> server
	for pid, server := range owner {
		dir := "/proc/" + strconv.Itoa(pid) + "/fd"
		fds, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, fd := range fds {
			l, err := os.Readlink(dir + "/" + fd.Name())
			if err == nil && strings.HasPrefix(l, "socket:[") {
				inodes[strings.TrimSuffix(strings.TrimPrefix(l, "socket:["), "]")] = server
			}
		}
	}
	if len(inodes) == 0 {
		return &pm
	}
	for _, t := range []struct {
		file  string
		proto uint8
	}{{"udp", 17}, {"udp6", 17}, {"tcp", 6}, {"tcp6", 6}} {
		b, err := os.ReadFile("/proc/net/" + t.file)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(b), "\n")[1:] {
			ip, port, inode, ok := parseProcNet(line)
			if !ok {
				continue
			}
			server, mine := inodes[inode]
			if !mine {
				continue
			}
			if ip.IsUnspecified() {
				ip = nil
			}
			k := portKey{t.proto, port}
			pm[k] = append(pm[k], portOwner{ip, server})
		}
	}
	return &pm
}

// familyOf gives the processes of each watched server: itself, the processes of its process group and its
// descendants, each with the server it belongs to.
func familyOf(watched map[uint32]*counter) map[int]uint32 {
	out := map[int]uint32{}
	parent, group := map[int]int{}, map[int]int{}
	entries, _ := os.ReadDir("/proc")
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		b, err := os.ReadFile("/proc/" + e.Name() + "/stat")
		if err != nil {
			continue
		}
		s := string(b)
		k := strings.LastIndexByte(s, ')')
		if k < 0 || k+2 >= len(s) {
			continue
		}
		f := strings.Fields(s[k+2:])
		if len(f) < 3 {
			continue
		}
		parent[pid], _ = strconv.Atoi(f[1])
		group[pid], _ = strconv.Atoi(f[2])
	}
	for pid := range parent {
		if _, ok := watched[uint32(group[pid])]; ok {
			out[pid] = uint32(group[pid])
			continue
		}
		for p, n := pid, 0; p > 1 && n < 16; p, n = parent[p], n+1 {
			if _, ok := watched[uint32(p)]; ok {
				out[pid] = uint32(p)
				break
			}
		}
	}
	for pid := range watched {
		out[int(pid)] = pid
	}
	return out
}

// parseProcNet reads a line of /proc/net/udp, udp6, tcp or tcp6: its local address and port, and its inode.
func parseProcNet(line string) (ip net.IP, port uint16, inode string, ok bool) {
	f := strings.Fields(line)
	if len(f) < 10 {
		return nil, 0, "", false
	}
	addr, p, found := strings.Cut(f[1], ":")
	if !found {
		return nil, 0, "", false
	}
	pn, err := strconv.ParseUint(p, 16, 16)
	if err != nil {
		return nil, 0, "", false
	}
	raw, err := hex.DecodeString(addr)
	if err != nil || (len(raw) != 4 && len(raw) != 16) {
		return nil, 0, "", false
	}
	// the kernel writes each 32-bit word in the machine's order (little-endian on x86 and ARM)
	ip = make(net.IP, len(raw))
	for w := 0; w < len(raw); w += 4 {
		ip[w], ip[w+1], ip[w+2], ip[w+3] = raw[w+3], raw[w+2], raw[w+1], raw[w]
	}
	if v4 := ip.To4(); v4 != nil && len(raw) == 16 {
		ip = v4 // ::ffff:1.2.3.4
	}
	return ip, uint16(pn), f[9], true
}

// dropAmbient: the systemd service gives szm CAP_NET_RAW as an "ambient" right, which every program it starts would
// inherit. The game servers do not need it: it is taken off the programs started from now on (szm keeps it, to
// start counting again).
func dropAmbient() {
	const prCapAmbient, prCapAmbientClearAll = 47, 4
	_, _, _ = syscall.AllThreadsSyscall(syscall.SYS_PRCTL, prCapAmbient, prCapAmbientClearAll, 0)
}
