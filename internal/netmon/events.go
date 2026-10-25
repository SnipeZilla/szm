package netmon

// The events of Microsoft-Windows-Kernel-Network counted (netmon_windows.go): data sent and received. Each starts
// with the process ID and the size (two 32-bit numbers).
const (
	tcp4Sent, tcp4Received = 10, 11
	tcp6Sent, tcp6Received = 26, 27
	udp4Sent, udp4Received = 42, 43
	udp6Sent, udp6Received = 58, 59
)

// loopback tells whether an event is between two programs of the machine (127.x.x.x, ::1: IIS and the manager,
// RCON on the machine…), not on the network. After the process ID and the size come the destination address, then
// the source: 4 bytes each for IPv4, 16 for IPv6.
func loopback(id uint16, d []byte) bool {
	switch id {
	case tcp4Sent, tcp4Received, udp4Sent, udp4Received:
		return len(d) >= 16 && (d[8] == 127 || d[12] == 127)
	case tcp6Sent, tcp6Received, udp6Sent, udp6Received:
		is1 := func(a []byte) bool {
			for k := 0; k < 15; k++ {
				if a[k] != 0 {
					return false
				}
			}
			return a[15] == 1
		}
		return len(d) >= 40 && (is1(d[8:24]) || is1(d[24:40]))
	}
	return false
}
