// Command titlecard draws the README title card (docs/titlecard.png):
// chaos pixels from /dev/urandom on the left, from which the notes of a
// melody evolved by MUseRND crystallise towards the right, the project title
// in a pixel font, and two small panels (genome -> notes, oscillator).
//
//	go run ./tools/titlecard [-out docs/titlecard.png]
//
// Every run draws a new picture: the noise and the song both come from
// /dev/urandom.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"

	"musernd/internal/melody"
)

const (
	W, H = 1280, 640
	cell = 8 // background pixel size
)

var (
	bg      = rgb(0x0e, 0x0e, 0x13)
	panelBg = rgb(0x16, 0x16, 0x1e)
	line    = rgb(0x2c, 0x2c, 0x36)
	text    = rgb(0xec, 0xec, 0xef)
	muted   = rgb(0x9a, 0x9a, 0xa3)
	partCol = map[melody.PartKind]color.RGBA{
		melody.PartLead:   rgb(0xe8, 0x89, 0x0c),
		melody.PartChords: rgb(0x8a, 0x63, 0xd2),
		melody.PartBass:   rgb(0x1f, 0x9d, 0x74),
		melody.PartDrums:  rgb(0x7a, 0x84, 0x94),
	}
)

func rgb(r, g, b uint8) color.RGBA { return color.RGBA{r, g, b, 255} }

type canvas struct{ *image.RGBA }

// blend paints c over the pixel at (x, y) with opacity a (0..1).
func (cv canvas) blend(x, y int, c color.RGBA, a float64) {
	if x < 0 || y < 0 || x >= W || y >= H {
		return
	}
	o := cv.RGBAAt(x, y)
	mix := func(f, b uint8) uint8 { return uint8(math.Round(float64(f)*a + float64(b)*(1-a))) }
	cv.SetRGBA(x, y, color.RGBA{mix(c.R, o.R), mix(c.G, o.G), mix(c.B, o.B), 255})
}

func (cv canvas) rect(x0, y0, w, h int, c color.RGBA, a float64) {
	for y := y0; y < y0+h; y++ {
		for x := x0; x < x0+w; x++ {
			cv.blend(x, y, c, a)
		}
	}
}

// roundRect fills a rectangle with rounded corners of radius r.
func (cv canvas) roundRect(x0, y0, w, h, r int, c color.RGBA, a float64) {
	for y := y0; y < y0+h; y++ {
		for x := x0; x < x0+w; x++ {
			dx := max(x0+r-x, x-(x0+w-1-r), 0)
			dy := max(y0+r-y, y-(y0+h-1-r), 0)
			if dx*dx+dy*dy <= r*r {
				cv.blend(x, y, c, a)
			}
		}
	}
}

// dot draws a filled disc (used for lines and note heads).
func (cv canvas) dot(cx, cy, r float64, c color.RGBA, a float64) {
	for y := int(cy - r - 1); y <= int(cy+r+1); y++ {
		for x := int(cx - r - 1); x <= int(cx+r+1); x++ {
			d := math.Hypot(float64(x)-cx, float64(y)-cy)
			if d <= r {
				cv.blend(x, y, c, a*math.Min(1, r+0.5-d))
			}
		}
	}
}

func (cv canvas) line(x0, y0, x1, y1, width float64, c color.RGBA, a float64) {
	n := int(math.Hypot(x1-x0, y1-y0)*2) + 1
	for i := 0; i <= n; i++ {
		t := float64(i) / float64(n)
		cv.dot(x0+(x1-x0)*t, y0+(y1-y0)*t, width/2, c, a)
	}
}

// text draws s in the 5x7 pixel font; each font pixel is px screen pixels
// (with a 1-pixel gap for the pixel look). It returns the width drawn.
func (cv canvas) text(x, y int, s string, px int, c color.RGBA, a float64) int {
	gap := max(1, px/7)
	cx := x
	for _, ch := range s {
		g, ok := font[ch]
		if !ok {
			g = font['?']
		}
		for row, bits := range g {
			for col, b := range bits {
				if b == '#' {
					cv.rect(cx+col*px, y+row*px, px-gap, px-gap, c, a)
				}
			}
		}
		cx += 6 * px
	}
	return cx - x
}

func textWidth(s string, px int) int { return len([]rune(s)) * 6 * px }

func main() {
	out := flag.String("out", "docs/titlecard.png", "output PNG")
	flag.Parse()

	src, err := os.Open("/dev/urandom")
	if err != nil {
		fail(err)
	}
	defer src.Close()
	rng := melody.NewRand(src)

	// A short song evolved from /dev/urandom, as MUseRND makes them.
	st, acc, _, err := melody.BuildStyle(melody.Settings{Genre: "pop", Tonic: -1, Seconds: 30, BPM: 120,
		Parts: []string{"chords", "bass", "drums"}})
	if err != nil {
		fail(err)
	}
	notes := melody.NotesFor(st)
	genome, best, _ := melody.Evolve(rng, melody.Options{Notes: notes, Acc: acc, Style: st,
		Population: 48, Generations: 1500, Target: 93})
	arr := melody.Arrange(genome, notes, acc, st)

	cv := canvas{image.NewRGBA(image.Rect(0, 0, W, H))}
	cv.rect(0, 0, W, H, bg, 1)

	// Background: chaos on the left, notes crystallising to the right.
	cols, rows := W/cell, H/cell
	lo, hi := 127, 0
	for _, p := range arr.Parts {
		if p.Kind == melody.PartDrums {
			continue
		}
		for _, e := range p.Events {
			lo, hi = min(lo, e.Pitch), max(hi, e.Pitch)
		}
	}
	type key struct{ x, y int }
	noteCells := map[key]color.RGBA{}
	stepW := float64(cols) / float64(arr.Steps)
	for _, p := range arr.Parts {
		for _, e := range p.Events {
			y := rows - 4 - (e.Pitch-lo)*(rows-8)/max(1, hi-lo) // pitch rows
			if p.Kind == melody.PartDrums {
				y = rows - 2 - (e.Pitch % 3) // a thin drum lane at the bottom
			}
			x0 := int(float64(e.Start) * stepW)
			x1 := max(x0+1, int(float64(e.Start+e.Steps)*stepW))
			for x := x0; x < x1 && x < cols; x++ {
				noteCells[key{x, y}] = partCol[p.Kind]
			}
		}
	}
	var b [4]byte
	for x := 0; x < cols; x++ {
		t := float64(x) / float64(cols-1)
		for y := 0; y < rows; y++ {
			rng.Bytes(b[:])
			r := float64(b[3]) / 255
			if c, ok := noteCells[key{x, y}]; ok && r < math.Pow(t, 0.6)*1.15 {
				cv.rect(x*cell, y*cell, cell-1, cell-1, c, 0.35+0.55*t)
				continue
			}
			if r < math.Pow(1-t, 1.6)*0.9 { // raw random pixels
				cv.rect(x*cell, y*cell, cell-1, cell-1, rgb(b[0], b[1], b[2]), 0.25+0.45*(1-t))
			}
		}
	}

	// Title block, on a dark band wide enough for the longest line.
	title := "MUseRND"
	tx, ty, px := 84, 128, 14
	line1, line2 := "music evolved from /dev/urandom", "20 genres - 16 instruments - midi & musicxml"
	bandW := max(textWidth(title, px), textWidth(line1, 4), textWidth(line2, 3)) + 72
	cv.roundRect(48, 92, bandW, 250, 22, bg, 0.86)
	cv.text(tx+5, ty+6, title, px, rgb(0, 0, 0), 0.6) // shadow
	for i, ch := range title {
		// orange -> purple across the title, like the melody and chord colours
		f := float64(i) / float64(len(title)-1)
		c := color.RGBA{uint8(0xe8 + (0x8a-0xe8)*f), uint8(0x89 + (0x63-0x89)*f), uint8(0x0c + (0xd2-0x0c)*f), 255}
		cv.text(tx+i*6*px, ty, string(ch), px, c, 1)
	}
	cv.text(tx+2, ty+7*px+28, line1, 4, text, 1)
	cv.text(tx+2, ty+7*px+70, line2, 3, muted, 1)

	// Panel 1: genome -> notes.
	px1, py1, pw, ph := 812, 380, 196, 196
	panel(cv, px1, py1, pw, ph, "genome > notes")
	n := 10
	sw := (pw - 32) / n
	dec := melody.Decode(genome[:n*3], st)
	plo, phi := 127, 0
	for _, d := range dec {
		if d.Pitch != melody.Rest {
			plo, phi = min(plo, d.Pitch), max(phi, d.Pitch)
		}
	}
	for i := 0; i < n; i++ {
		g := genome[i*3 : i*3+3] // stored B,G,R
		x := px1 + 16 + i*sw
		cv.roundRect(x, py1+40, sw-4, sw-4, 3, rgb(g[2], g[1], g[0]), 1)
		cv.line(float64(x+(sw-4)/2), float64(py1+40+sw), float64(x+(sw-4)/2), float64(py1+88), 1.4, muted, 0.8)
		if d := dec[i]; d.Pitch != melody.Rest {
			y := py1 + ph - 24 - (d.Pitch-plo)*(ph-128)/max(1, phi-plo)
			cv.dot(float64(x+(sw-4)/2), float64(y), 5.5, partCol[melody.PartLead], 1)
			cv.line(float64(x+(sw-4)/2+5), float64(y), float64(x+(sw-4)/2+5), float64(y-24), 1.6, partCol[melody.PartLead], 1)
		}
	}

	// Panel 2: oscillator (the first melody notes as a waveform).
	px2 := px1 + pw + 24
	panel(cv, px2, py1, pw, ph, "oscillator")
	for gx := px2 + 12; gx < px2+pw-12; gx += 23 { // scope grid
		cv.line(float64(gx), float64(py1+36), float64(gx), float64(py1+ph-12), 1, line, 1)
	}
	for gy := py1 + 36; gy < py1+ph-12; gy += 23 {
		cv.line(float64(px2+12), float64(gy), float64(px2+pw-12), float64(gy), 1, line, 1)
	}
	mid := float64(py1) + float64(36+ph-12)/2
	var prevX, prevY float64
	for i := 0; i <= pw-24; i++ {
		x := float64(px2 + 12 + i)
		t := float64(i) / float64(pw-24)
		d := dec[min(len(dec)-1, int(t*4))] // four notes across the scope
		f := 3.0
		if d.Pitch != melody.Rest {
			f = 2 + float64(d.Pitch-plo)/2
		}
		v := math.Sin(2*math.Pi*f*t) + 0.35*math.Sin(4*math.Pi*f*t) + 0.15*math.Sin(6*math.Pi*f*t)
		y := mid - v*float64(ph-60)/3.2
		if i > 0 {
			cv.line(prevX, prevY, x, y, 5, partCol[melody.PartBass], 0.25) // glow
			cv.line(prevX, prevY, x, y, 2, rgb(0x6f, 0xf0, 0xc4), 1)
		}
		prevX, prevY = x, y
	}

	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		fail(err)
	}
	f, err := os.Create(*out)
	if err != nil {
		fail(err)
	}
	if err := png.Encode(f, cv.RGBA); err != nil {
		fail(err)
	}
	if err := f.Close(); err != nil {
		fail(err)
	}
	fmt.Printf("%s: %s song in %s, score %.1f\n", *out, st.Name, best.Melody.KeyName, best.Total)
}

func panel(cv canvas, x, y, w, h int, label string) {
	cv.roundRect(x-2, y-2, w+4, h+4, 14, line, 1)
	cv.roundRect(x, y, w, h, 12, panelBg, 0.96)
	cv.text(x+14, y+14, label, 2, muted, 1)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "titlecard:", err)
	os.Exit(1)
}

// font is a 5x7 pixel font for the characters the card uses.
var font = map[rune][7]string{
	' ': {".....", ".....", ".....", ".....", ".....", ".....", "....."},
	'M': {"#...#", "##.##", "#.#.#", "#.#.#", "#...#", "#...#", "#...#"},
	'U': {"#...#", "#...#", "#...#", "#...#", "#...#", "#...#", ".###."},
	'R': {"####.", "#...#", "#...#", "####.", "#.#..", "#..#.", "#...#"},
	'N': {"#...#", "##..#", "#.#.#", "#..##", "#...#", "#...#", "#...#"},
	'D': {"####.", "#...#", "#...#", "#...#", "#...#", "#...#", "####."},
	'a': {".....", ".....", ".###.", "....#", ".####", "#...#", ".####"},
	'b': {"#....", "#....", "####.", "#...#", "#...#", "#...#", "####."},
	'c': {".....", ".....", ".###.", "#....", "#....", "#...#", ".###."},
	'd': {"....#", "....#", ".####", "#...#", "#...#", "#...#", ".####"},
	'e': {".....", ".....", ".###.", "#...#", "#####", "#....", ".###."},
	'f': {"..##.", ".#..#", ".#...", "###..", ".#...", ".#...", ".#..."},
	'g': {".....", ".####", "#...#", "#...#", ".####", "....#", ".###."},
	'h': {"#....", "#....", "####.", "#...#", "#...#", "#...#", "#...#"},
	'i': {"..#..", ".....", ".##..", "..#..", "..#..", "..#..", ".###."},
	'j': {"...#.", ".....", "..##.", "...#.", "...#.", "#..#.", ".##.."},
	'k': {"#....", "#....", "#..#.", "#.#..", "##...", "#.#..", "#..#."},
	'l': {".##..", "..#..", "..#..", "..#..", "..#..", "..#..", ".###."},
	'm': {".....", ".....", "##.#.", "#.#.#", "#.#.#", "#...#", "#...#"},
	'n': {".....", ".....", "####.", "#...#", "#...#", "#...#", "#...#"},
	'o': {".....", ".....", ".###.", "#...#", "#...#", "#...#", ".###."},
	'p': {".....", ".....", "####.", "#...#", "####.", "#....", "#...."},
	'q': {".....", ".....", ".####", "#...#", ".####", "....#", "....#"},
	'r': {".....", ".....", "#.##.", "##..#", "#....", "#....", "#...."},
	's': {".....", ".....", ".####", "#....", ".###.", "....#", "####."},
	't': {".#...", ".#...", "###..", ".#...", ".#...", ".#..#", "..##."},
	'u': {".....", ".....", "#...#", "#...#", "#...#", "#..##", ".##.#"},
	'v': {".....", ".....", "#...#", "#...#", "#...#", ".#.#.", "..#.."},
	'w': {".....", ".....", "#...#", "#...#", "#.#.#", "#.#.#", ".#.#."},
	'x': {".....", ".....", "#...#", ".#.#.", "..#..", ".#.#.", "#...#"},
	'y': {".....", ".....", "#...#", "#...#", ".####", "....#", ".###."},
	'z': {".....", ".....", "#####", "...#.", "..#..", ".#...", "#####"},
	'0': {".###.", "#...#", "#..##", "#.#.#", "##..#", "#...#", ".###."},
	'1': {"..#..", ".##..", "..#..", "..#..", "..#..", "..#..", ".###."},
	'2': {".###.", "#...#", "....#", "...#.", "..#..", ".#...", "#####"},
	'3': {"####.", "....#", "....#", ".###.", "....#", "....#", "####."},
	'4': {"...#.", "..##.", ".#.#.", "#..#.", "#####", "...#.", "...#."},
	'5': {"#####", "#....", "####.", "....#", "....#", "#...#", ".###."},
	'6': {"..##.", ".#...", "#....", "####.", "#...#", "#...#", ".###."},
	'7': {"#####", "....#", "...#.", "..#..", ".#...", ".#...", ".#..."},
	'8': {".###.", "#...#", "#...#", ".###.", "#...#", "#...#", ".###."},
	'9': {".###.", "#...#", "#...#", ".####", "....#", "...#.", ".##.."},
	'/': {"....#", "....#", "...#.", "..#..", ".#...", "#....", "#...."},
	'-': {".....", ".....", ".....", "#####", ".....", ".....", "....."},
	'>': {".#...", "..#..", "...#.", "....#", "...#.", "..#..", ".#..."},
	'&': {".##..", "#..#.", "#.#..", ".#...", "#.#.#", "#..#.", ".##.#"},
	'.': {".....", ".....", ".....", ".....", ".....", ".##..", ".##.."},
	',': {".....", ".....", ".....", ".....", ".##..", "..#..", ".#..."},
	':': {".....", ".##..", ".##..", ".....", ".##..", ".##..", "....."},
	'#': {".#.#.", ".#.#.", "#####", ".#.#.", "#####", ".#.#.", ".#.#."},
	'?': {".###.", "#...#", "....#", "...#.", "..#..", ".....", "..#.."},
}
