package proc

import "testing"

func TestParseNetDev(t *testing.T) {
	s := `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo: 9999999     210    0    0    0     0          0         0  9999999     210    0    0    0     0       0          0
  eno1: 1000000    5000    0    0    0     0          0        12  3000000    6000    0    0    0     0       0          0
docker0:  500000     100    0    0    0     0          0         0   500000     100    0    0    0     0       0          0
vethab12:  500000     100    0    0    0     0          0         0   500000     100    0    0    0     0       0          0
  eno2:     200       2    0    0    0     0          0         0      400       4    0    0    0     0       0          0
`
	if in, out := parseNetDev(s); in != 1000200 || out != 3000400 {
		t.Fatal(in, out)
	}
	if _, _, err := SystemCPU(); err != nil {
		t.Fatal(err)
	}
}

func TestMountOf(t *testing.T) {
	m := `/dev/sda1 / ext4 rw 0 0
proc /proc proc rw 0 0
/dev/sdb1 /srv/games ext4 rw 0 0
/dev/sdc1 /srv/games\040two ext4 rw 0 0
/dev/sdd1 /home xfs rw 0 0
`
	for p, want := range map[string]string{"/opt/szm/servers/cs2": "/", "/srv/games/cs2": "/srv/games", "/srv/gamesX/a": "/",
		"/srv/games two/tf2": "/srv/games two", "/home": "/home"} {
		if got := mountOf(m, p); got != want {
			t.Errorf("%s: %s", p, got)
		}
	}
}
