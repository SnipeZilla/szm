package proc

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// A console's control sequences are taken out: colors, its title, cursor moves (a line end), moves forward (spaces).
func TestVTFilter(t *testing.T) {
	in := "\x1b]0;C:\\SteamCMD\\steamcmd.exe\x07\x1b[?25l\x1b[2J\x1b[m\x1b[HRedirecting stderr\r\n\x1b[1mLogging in\x1b[0m user\x1b[5Cbob\x1b[K\x1b[3;1HWaiting\x1b(B done"
	var out bytes.Buffer
	// one byte at a time: a sequence split across reads
	f := &vtFilter{r: &oneByte{strings.NewReader(in)}}
	if _, err := io.Copy(&out, f); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "\rRedirecting stderr\r\nLogging in user     bob\rWaiting done" {
		t.Fatalf("%q", got)
	}
}

type oneByte struct{ r io.Reader }

func (o *oneByte) Read(p []byte) (int, error) { return o.r.Read(p[:1]) }

// In a console of its own, a program that holds back what it prints to a pipe (as steamcmd does) prints each line
// at once; its end, and what it started, are waited for.
func TestStartCaptured(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("a shell script")
	}
	dir := t.TempDir()
	sh := filepath.Join(dir, "steamcmd.sh")
	// python holds back what it prints to a pipe, not to a terminal; then another program prints (steamcmd.sh runs
	// steamcmd again after it has updated itself)
	os.WriteFile(sh, []byte("#!/bin/sh\npython3 -c 'import time\nprint(\"\\033[1mLogging in\\033[0m\")\ntime.sleep(1.5)\nprint(\"OK\")'\nsh -c 'sleep 0.3; echo child done'\n"), 0o755)
	c, err := StartCaptured(context.Background(), sh, nil, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !c.Console {
		t.Fatal("not in a console")
	}
	got := make(chan string, 10)
	go func() {
		b := make([]byte, 4096)
		var all strings.Builder
		for {
			n, err := c.Output.Read(b)
			all.Write(b[:n])
			if strings.Contains(all.String(), "Logging in") {
				got <- "first"
			}
			if err != nil {
				got <- all.String()
				return
			}
		}
	}()
	select {
	case <-got:
	case <-time.After(time.Second):
		t.Fatal("its first line was held back")
	}
	var all string
	for s := range got {
		if s != "first" {
			all = s
			break
		}
	}
	if err := c.Wait(); err != nil {
		t.Fatal(err)
	}
	if strings.ReplaceAll(all, "\r", "") != "Logging in\nOK\nchild done\n" {
		t.Fatalf("%q", all)
	}
}

// A question waiting for a typed answer: the program is ended.
func TestCapturedKill(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("a shell script")
	}
	dir := t.TempDir()
	sh := filepath.Join(dir, "steamcmd.sh")
	os.WriteFile(sh, []byte("#!/bin/sh\nprintf 'Steam Guard code:'\nread code\necho got $code\n"), 0o755)
	c, err := StartCaptured(context.Background(), sh, nil, dir)
	if err != nil {
		t.Fatal(err)
	}
	time.AfterFunc(500*time.Millisecond, c.Kill)
	start := time.Now()
	b, _ := io.ReadAll(c.Output)
	c.Wait()
	if time.Since(start) > 3*time.Second || !strings.Contains(string(b), "Steam Guard code:") || strings.Contains(string(b), "got") {
		t.Fatalf("%q %s", b, time.Since(start))
	}
}
