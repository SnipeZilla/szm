package proc

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"
)

// fakeScreen is a console's buffer: rows printed into it scroll once it is full, as a console's do.
type fakeScreen struct {
	height int
	buf    []string // the finished rows in the buffer (the cursor is on the row after them)
	reads  int      // rows read
}

func (f *fakeScreen) print(rows ...string) {
	for _, r := range rows {
		f.buf = append(f.buf, r)
		if len(f.buf) > f.height-1 {
			f.buf = f.buf[1:] // the cursor stays on the last row: the text goes up
		}
	}
}

func (f *fakeScreen) size() (int, int, error) { return f.height, len(f.buf), nil }

func (f *fakeScreen) rows(first, n int) ([]string, error) {
	if first < 0 || n < 0 || first+n > f.height {
		return nil, fmt.Errorf("rows %d+%d of %d", first, n, f.height)
	}
	out := make([]string, 0, n)
	for k := first; k < first+n; k++ {
		if k < len(f.buf) {
			out = append(out, f.buf[k])
		} else {
			out = append(out, "") // the cursor's row and below: empty here
		}
	}
	f.reads += n
	return out, nil
}

func TestRowTracker(t *testing.T) {
	scr := &fakeScreen{height: 50}
	tr := newRowTracker(400)
	n := 0
	next := func(k int) []string {
		rows := make([]string, k)
		for j := range rows {
			rows[j] = fmt.Sprintf("row %d", n)
			n++
		}
		return rows
	}
	poll := func(want []string) {
		t.Helper()
		got, err := tr.poll(scr)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Join(got, "|") != strings.Join(want, "|") {
			t.Fatalf("got %q\nwant %q", got, want)
		}
	}
	// the first look: what is there
	first := next(3)
	scr.print(first...)
	poll(first)
	poll(nil)
	// rows as the cursor goes down: those exactly, the same row twice too
	scr.print("same", "same", "same")
	poll([]string{"same", "same", "same"})
	scr.print("same")
	poll([]string{"same"})
	poll(nil)
	// up to the last row, then scrolling: each look its new rows, whatever their number
	for _, k := range []int{10, 30, 1, 0, 7, 45, 2, 0, 0, 3} {
		rows := next(k)
		scr.print(rows...)
		poll(rows)
	}
	// scrolled and nothing new: two rows read, not the buffer
	scr.reads = 0
	for k := 0; k < 10; k++ {
		poll(nil)
	}
	if scr.reads != 20 {
		t.Fatalf("%d rows read for 10 looks with nothing new", scr.reads)
	}
	// more at once than the buffer holds: what is left of them
	rows := next(120)
	scr.print(rows...)
	poll(rows[len(rows)-49:])
	// cleared (cls), then written again
	scr.buf = nil
	poll(nil)
	rows = next(4)
	scr.print(rows...)
	poll(rows)
	// cleared and written again between two looks
	scr.buf = nil
	rows = next(2)
	scr.print(rows...)
	poll(rows)
	rows = next(5)
	scr.print(rows...)
	poll(rows)
}

// A long buffer (as a server's: 9999 rows) that scrolls, with many rows at a time: found again further up.
func TestRowTrackerLong(t *testing.T) {
	scr := &fakeScreen{height: 9999}
	tr := newRowTracker(400)
	n := 0
	total := 0
	for step := 0; step < 400; step++ {
		k := []int{0, 1, 5, 90, 150, 700, 0, 2500, 3}[step%9]
		rows := make([]string, k)
		for j := range rows {
			rows[j] = fmt.Sprintf("row %d", n)
			n++
		}
		scr.print(rows...)
		got, err := tr.poll(scr)
		if err != nil {
			t.Fatal(err)
		}
		if step == 0 {
			continue
		}
		if strings.Join(got, "|") != strings.Join(rows, "|") {
			t.Fatalf("step %d: %d rows, want %d", step, len(got), len(rows))
		}
		total += len(got)
	}
	if total < 100000 {
		t.Fatal(total)
	}
}

// The first look gives the last rows only: the manager knows which its log has.
func TestRowTrackerFirst(t *testing.T) {
	scr := &fakeScreen{height: 9999}
	for k := 0; k < 1000; k++ {
		scr.print(fmt.Sprint(k))
	}
	tr := newRowTracker(400)
	got, _ := tr.poll(scr)
	if len(got) != 400 || got[0] != "600" || got[399] != "999" {
		t.Fatal(len(got))
	}
	scr.print("1000")
	if got, _ := tr.poll(scr); len(got) != 1 || got[0] != "1000" {
		t.Fatal(got)
	}
}

func TestRowsAfter(t *testing.T) {
	r := func(a ...string) []string { return a }
	cases := []struct {
		prev, cur, want []string
		found           bool
	}{
		{nil, r("a", "b"), r("a", "b"), false},
		{r("a", "b"), r("a", "b"), r(), true},
		{r("a", "b"), r("a", "b", "c", "d"), r("c", "d"), true},
		{r("p", "a", "b", "c"), r("a", "b", "c", "d", "e"), r("d", "e"), true},
		{r("a", "b", "c"), r("q", "r"), r("q", "r"), false},
	}
	for n, c := range cases {
		got, found := RowsAfter(c.prev, c.cur)
		if strings.Join(got, "|") != strings.Join(c.want, "|") || found != c.found {
			t.Errorf("%d: %q %v", n, got, found)
		}
	}
}

func TestBatches(t *testing.T) {
	var w, buf bytes.Buffer
	sent := [][]string{{"one"}, {"", "two words", "\x01not a head", "ünï"}, {"with\nline end"}}
	for _, b := range sent {
		if err := writeBatch(&w, &buf, b); err != nil {
			t.Fatal(err)
		}
	}
	var got [][]string
	if err := readBatches(&w, func(rows []string) { got = append(got, rows) }); err != nil {
		t.Fatal(err)
	}
	sent[2][0] = "with line end"
	if fmt.Sprint(got) != fmt.Sprint(sent) || len(got) != 3 || len(got[1]) != 4 {
		t.Fatalf("%q", got)
	}
	// not rows; cut in the middle
	if err := readBatches(strings.NewReader("Error: no\n"), func([]string) {}); err == nil {
		t.Fatal("no error")
	}
	if err := readBatches(strings.NewReader("\x013\na\nb\n"), func([]string) { t.Fatal("handed") }); err == nil {
		t.Fatal("no error")
	}
	if err := readBatches(io.LimitReader(strings.NewReader(""), 0), func([]string) {}); err != nil {
		t.Fatal(err)
	}
}
