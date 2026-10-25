package netcount

import (
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestCounts(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		w.Write([]byte(strings.Repeat("x", 5000) + string(b)))
	})}
	go srv.Serve(Listener(l))
	defer srv.Close()
	CountHTTP()
	in0, out0 := Totals()
	resp, err := http.Post("http://"+l.Addr().String()+"/", "text/plain", strings.NewReader(strings.Repeat("y", 3000)))
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	in1, out1 := Totals()
	// the server counts its write when the write returns: the client may have read it all a moment before
	for k := 0; k < 100 && in1-in0 != out1-out0; k++ {
		time.Sleep(10 * time.Millisecond)
		in1, out1 = Totals()
	}
	// both ends are this process: 3000 up and 8000 down, each counted sent once and received once, with the headers
	if in1-in0 < 11000 || in1-in0 > 12000 || in1-in0 != out1-out0 {
		t.Fatal(in1-in0, out1-out0)
	}
}
