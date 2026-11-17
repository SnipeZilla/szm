package notify

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strconv"
)

// The chart of a status card: the players of the last 24 hours, a bar an hour. Drawn here, with nothing but the
// standard library, and sent with the card: no site to show it from.

// Hour is the players of one hour.
type Hour struct {
	Avg float64 // on average
	Max int     // at most
	Has bool    // the manager looked during that hour
}

const (
	chartW, chartH   = 800, 190
	chartPadX        = 18
	chartTop         = 34 // above the bars: the highest number of players
	chartBottom      = 34 // under them: "24H AGO" and "NOW"
	chartGap         = 8  // between two bars
	chartScale       = 2  // of the letters (5 x 7 points each)
	chartMinPlayers  = 4  // the least the height of the chart stands for: one player is not a full bar
	chartIdleHeight  = 4  // an hour with nobody: a low bar, so that the hours show
	chartCornerInset = 10 // the panel's round corners
)

var (
	chartBack  = color.RGBA{0x1b, 0x1e, 0x2b, 0xff} // the panel
	chartAvg   = color.RGBA{0x8c, 0xc2, 0xff, 0xff} // the average: light blue
	chartPeak  = color.RGBA{0x3b, 0x82, 0xf6, 0xff} // up to the most: blue
	chartIdle  = color.RGBA{0x36, 0x3d, 0x57, 0xff} // nobody
	chartText  = color.RGBA{0xb4, 0xbd, 0xd6, 0xff}
	chartClear = color.RGBA{}
)

// Chart draws the players of the last hours (the oldest first, the current hour last) as a PNG.
func Chart(hours []Hour) []byte {
	img := image.NewRGBA(image.Rect(0, 0, chartW, chartH))
	for y := 0; y < chartH; y++ {
		for x := 0; x < chartW; x++ {
			c := chartBack
			if cornerOut(x, y, chartW, chartH, chartCornerInset) {
				c = chartClear
			}
			img.SetRGBA(x, y, c)
		}
	}
	peak := 0
	for _, h := range hours {
		peak = max(peak, h.Max)
	}
	top := max(peak, chartMinPlayers)
	area := chartH - chartTop - chartBottom
	if n := len(hours); n > 0 {
		barW := (chartW - 2*chartPadX - (n-1)*chartGap) / n
		left := (chartW - n*barW - (n-1)*chartGap) / 2
		base := chartH - chartBottom
		for k, h := range hours {
			x := left + k*(barW+chartGap)
			if !h.Has {
				continue
			}
			hp := area * h.Max / top
			ha := int(float64(area)*h.Avg/float64(top) + 0.5)
			if h.Max > 0 {
				hp, ha = max(hp, chartIdleHeight+2), max(ha, chartIdleHeight)
			}
			if hp <= 0 {
				bar(img, x, base-chartIdleHeight, barW, chartIdleHeight, chartIdle)
				continue
			}
			ha = min(ha, hp)
			bar(img, x, base-hp, barW, hp, chartPeak)
			if ha > 0 {
				bar(img, x, base-ha, barW, ha, chartAvg)
				if ha < hp {
					// its top is flat against the darker part above it
					fill(img, x, base-ha, barW, min(3, ha), chartAvg)
				}
			}
		}
	}
	text(img, chartPadX, chartH-chartBottom+10, "24H AGO")
	text(img, chartW-chartPadX-textWidth("NOW"), chartH-chartBottom+10, "NOW")
	text(img, chartPadX, 10, "PEAK "+strconv.Itoa(peak))
	var b bytes.Buffer
	_ = (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&b, img)
	return b.Bytes()
}

// cornerOut tells whether a point is outside the round corners of a w x h panel.
func cornerOut(x, y, w, h, r int) bool {
	cx, cy := x, y
	switch {
	case x < r:
		cx = r
	case x >= w-r:
		cx = w - r - 1
	}
	switch {
	case y < r:
		cy = r
	case y >= h-r:
		cy = h - r - 1
	}
	dx, dy := x-cx, y-cy
	return dx*dx+dy*dy > r*r
}

func fill(img *image.RGBA, x, y, w, h int, c color.RGBA) {
	for yy := y; yy < y+h; yy++ {
		for xx := x; xx < x+w; xx++ {
			img.SetRGBA(xx, yy, c)
		}
	}
}

// bar is a filled rectangle with its two top corners rounded.
func bar(img *image.RGBA, x, y, w, h int, c color.RGBA) {
	r := min(4, w/2, h)
	for yy := y; yy < y+h; yy++ {
		for xx := x; xx < x+w; xx++ {
			if yy < y+r {
				cx := xx
				if xx < x+r {
					cx = x + r
				} else if xx >= x+w-r {
					cx = x + w - r - 1
				}
				dx, dy := xx-cx, yy-(y+r)
				if dx*dx+dy*dy > r*r {
					continue
				}
			}
			img.SetRGBA(xx, yy, c)
		}
	}
}

// The few characters the chart writes, 5 points wide and 7 high.
var glyphs = map[rune][7]string{
	'0': {".###.", "#...#", "#..##", "#.#.#", "##..#", "#...#", ".###."},
	'1': {"..#..", ".##..", "..#..", "..#..", "..#..", "..#..", ".###."},
	'2': {".###.", "#...#", "....#", "...#.", "..#..", ".#...", "#####"},
	'3': {".###.", "#...#", "....#", "..##.", "....#", "#...#", ".###."},
	'4': {"...#.", "..##.", ".#.#.", "#..#.", "#####", "...#.", "...#."},
	'5': {"#####", "#....", "####.", "....#", "....#", "#...#", ".###."},
	'6': {"..##.", ".#...", "#....", "####.", "#...#", "#...#", ".###."},
	'7': {"#####", "....#", "...#.", "..#..", ".#...", ".#...", ".#..."},
	'8': {".###.", "#...#", "#...#", ".###.", "#...#", "#...#", ".###."},
	'9': {".###.", "#...#", "#...#", ".####", "....#", "...#.", ".##.."},
	'A': {".###.", "#...#", "#...#", "#####", "#...#", "#...#", "#...#"},
	'E': {"#####", "#....", "#....", "####.", "#....", "#....", "#####"},
	'G': {".###.", "#...#", "#....", "#.###", "#...#", "#...#", ".###."},
	'H': {"#...#", "#...#", "#...#", "#####", "#...#", "#...#", "#...#"},
	'K': {"#...#", "#..#.", "#.#..", "##...", "#.#..", "#..#.", "#...#"},
	'N': {"#...#", "##..#", "#.#.#", "#..##", "#...#", "#...#", "#...#"},
	'O': {".###.", "#...#", "#...#", "#...#", "#...#", "#...#", ".###."},
	'P': {"####.", "#...#", "#...#", "####.", "#....", "#....", "#...."},
	'W': {"#...#", "#...#", "#...#", "#.#.#", "#.#.#", "##.##", "#...#"},
}

func textWidth(s string) int { return len([]rune(s)) * 6 * chartScale }

func text(img *image.RGBA, x, y int, s string) {
	for _, r := range s {
		if g, ok := glyphs[r]; ok {
			for row, line := range g {
				for col, ch := range line {
					if ch == '#' {
						fill(img, x+col*chartScale, y+row*chartScale, chartScale, chartScale, chartText)
					}
				}
			}
		}
		x += 6 * chartScale
	}
}
