package proc

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Following a console window (CS2 on Windows): a reader stays attached to the server's console and gives the rows
// as they are finished, instead of a new program every second that reads the window again. What is here is the part
// that is the same on every system (and tested on any): which rows are new, and how they go to the manager.

// RowsAfter gives the rows of cur after those seen last time (prev: the last rows then), and whether it found
// where they start. The window scrolls: prev is looked for as a whole, then by its last rows only; not found, every
// row is new (the window was cleared, or more came than was read).
func RowsAfter(prev, cur []string) ([]string, bool) {
	if len(prev) == 0 {
		return cur, false
	}
	for k := len(prev); k >= min(3, len(prev)); k-- {
		seq := prev[len(prev)-k:]
		for e := len(cur) - 1; e >= k-1; e-- {
			match := true
			for j := 0; j < k; j++ {
				if cur[e-k+1+j] != seq[j] {
					match = false
					break
				}
			}
			if match {
				return cur[e+1:], true
			}
		}
	}
	return cur, false
}

// rowScreen is the screen buffer of a console, as its reader sees it.
type rowScreen interface {
	// size: the rows of the buffer, and the row of the cursor (the one being written: those before are finished)
	size() (height, cursor int, err error)
	// rows: n rows from the row first
	rows(first, n int) ([]string, error)
}

const (
	trackTail  = 30   // the last rows kept, to find the place again once the buffer scrolls
	trackFirst = 100  // rows read first to find it
	trackMax   = 3200 // ... and at most, when it is not in those (many rows came at once)
)

// rowTracker tells the rows of a console that are new since it last looked.
//
// While the buffer is not full, the cursor goes down a row with each line: the new rows are those between the row it
// was on and the row it is on, exactly. Once the buffer is full the cursor stays on its last row and the text
// scrolls up under it: the new rows are then those after the last ones seen (RowsAfter), looked for in the last
// rows of the buffer.
type rowTracker struct {
	first int      // rows given at the first look at most (the manager tells which its log has already)
	next  int      // the cursor's row at the last look; -1 before the first
	tail  []string // the last rows seen
}

func newRowTracker(first int) *rowTracker { return &rowTracker{first: first, next: -1} }

func (t *rowTracker) keep(rows []string) {
	t.tail = append(t.tail, rows...)
	if len(t.tail) > trackTail {
		t.tail = append(t.tail[:0], t.tail[len(t.tail)-trackTail:]...)
	}
}

// poll gives the rows finished since the last look.
func (t *rowTracker) poll(s rowScreen) ([]string, error) {
	height, cur, err := s.size()
	if err != nil {
		return nil, err
	}
	if cur < 0 || height <= 0 {
		return nil, nil
	}
	if t.next < 0 {
		// the first look: its last rows
		n := min(cur, t.first)
		var rows []string
		if n > 0 {
			if rows, err = s.rows(cur-n, n); err != nil {
				return nil, err
			}
		}
		t.next = cur
		t.tail = nil
		t.keep(rows)
		return rows, nil
	}
	if cur >= t.next && cur < height-1 {
		// not full yet: the rows the cursor went through
		if cur == t.next {
			return nil, nil
		}
		rows, err := s.rows(t.next, cur-t.next)
		if err != nil {
			return nil, err
		}
		t.next = cur
		t.keep(rows)
		return rows, nil
	}
	// The buffer is full and scrolls (or the cursor went back up: the window was cleared).
	if cur == 0 {
		t.next, t.tail = 0, nil // cleared, nothing written yet
		return nil, nil
	}
	if cur == t.next && len(t.tail) >= 2 && cur >= 2 {
		// nothing scrolled if its last two rows are the last two seen: most looks end here, at little cost
		last, err := s.rows(cur-2, 2)
		if err != nil {
			return nil, err
		}
		if last[0] == t.tail[len(t.tail)-2] && last[1] == t.tail[len(t.tail)-1] {
			return nil, nil
		}
	}
	for n := trackFirst; ; n *= 4 {
		n = min(n, cur, trackMax)
		rows, err := s.rows(cur-n, n)
		if err != nil {
			return nil, err
		}
		fresh, found := RowsAfter(t.tail, rows)
		if found || n == cur || n == trackMax {
			t.next = cur
			if !found {
				t.tail = nil
			}
			t.keep(fresh)
			return fresh, nil
		}
	}
}

// The rows go from the reader to the manager in batches: a line "\x01<number of rows>", then the rows, one a line.

var oneLine = strings.NewReplacer("\n", " ", "\r", " ")

func writeBatch(w io.Writer, buf *bytes.Buffer, rows []string) error {
	buf.Reset()
	buf.WriteByte(1)
	buf.WriteString(strconv.Itoa(len(rows)))
	buf.WriteByte('\n')
	for _, r := range rows {
		buf.WriteString(oneLine.Replace(r))
		buf.WriteByte('\n')
	}
	_, err := w.Write(buf.Bytes())
	return err
}

// readBatches hands each batch of r to each, until r ends.
func readBatches(r io.Reader, each func(rows []string)) error {
	br := bufio.NewReaderSize(r, 64<<10)
	for {
		head, err := br.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) && head == "" {
				return nil
			}
			return fmt.Errorf("the reader of the console window ended in the middle of its rows: %w", err)
		}
		head = strings.TrimRight(head, "\r\n")
		if len(head) < 2 || head[0] != 1 {
			return fmt.Errorf("the reader of the console window said something else than rows: %.80q", head)
		}
		n, err := strconv.Atoi(head[1:])
		if err != nil || n < 0 || n > 1<<20 {
			return fmt.Errorf("the reader of the console window said something else than rows: %.80q", head)
		}
		rows := make([]string, 0, n)
		for k := 0; k < n; k++ {
			l, err := br.ReadString('\n')
			if err != nil {
				return fmt.Errorf("the reader of the console window ended in the middle of its rows: %w", err)
			}
			rows = append(rows, strings.TrimRight(l, "\r\n"))
		}
		each(rows)
	}
}
