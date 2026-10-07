package melody

import (
	"encoding/binary"
	"encoding/xml"
	"math/rand"
	"testing"
)

func randomArrangement(t *testing.T, seed int64, acc Accompaniment) Arrangement {
	t.Helper()
	g := make([]byte, GenomeSize(64, Pop))
	rand.New(rand.NewSource(seed)).Read(g)
	return Arrange(g, 64, acc, Pop)
}

func TestMIDIStructure(t *testing.T) {
	a := randomArrangement(t, 3, AccompAll)
	b := MIDI(a, 120, "test")
	be := binary.BigEndian
	if string(b[0:4]) != "MThd" || be.Uint32(b[4:8]) != 6 || be.Uint16(b[8:10]) != 1 {
		t.Fatal("bad MIDI header")
	}
	ntracks := int(be.Uint16(b[10:12]))
	if ntracks != len(a.Parts)+1 || be.Uint16(b[12:14]) != TicksPerQuarter {
		t.Fatalf("header: %d tracks", ntracks)
	}
	pos := 14
	for tr := 0; tr < ntracks; tr++ {
		if string(b[pos:pos+4]) != "MTrk" {
			t.Fatalf("track %d: missing MTrk", tr)
		}
		end := pos + 8 + int(be.Uint32(b[pos+4:pos+8]))
		p := pos + 8
		readVar := func() int {
			v := 0
			for {
				c := b[p]
				p++
				v = v<<7 | int(c&0x7F)
				if c < 0x80 {
					return v
				}
			}
		}
		on, off, tick := 0, 0, 0
		sawEnd := false
		for p < end {
			tick += readVar()
			status := b[p]
			p++
			switch {
			case status == 0xFF:
				typ := b[p]
				p++
				n := readVar()
				p += n
				if typ == 0x2F {
					sawEnd = true
				}
			case status&0xF0 == 0x90:
				on++
				p += 2
			case status&0xF0 == 0x80:
				off++
				p += 2
			case status&0xF0 == 0xB0:
				p += 2
			case status&0xF0 == 0xC0:
				p++
			default:
				t.Fatalf("track %d: unexpected status %#x", tr, status)
			}
		}
		if p != end || !sawEnd {
			t.Fatalf("track %d: length mismatch or no end-of-track", tr)
		}
		if tr > 0 {
			if want := len(a.Parts[tr-1].Events); on != want || off != want {
				t.Fatalf("track %d: %d on / %d off, want %d", tr, on, off, want)
			}
		}
		pos = end
	}
	if pos != len(b) {
		t.Fatal("trailing bytes")
	}
}

// Minimal MusicXML model for checking measure arithmetic.
type xScore struct {
	Parts []struct {
		ID       string `xml:"id,attr"`
		Measures []struct {
			Number string `xml:"number,attr"`
			Items  []struct {
				XMLName  xml.Name
				Chord    *struct{} `xml:"chord"`
				Rest     *struct{} `xml:"rest"`
				Duration int       `xml:"duration"`
				Voice    int       `xml:"voice"`
				Ties     []struct {
					Type string `xml:"type,attr"`
				} `xml:"tie"`
				Pitch *struct {
					Step   string `xml:"step"`
					Alter  int    `xml:"alter"`
					Octave int    `xml:"octave"`
				} `xml:"pitch"`
			} `xml:",any"`
		} `xml:"measure"`
	} `xml:"part"`
}

func TestMusicXMLMeasuresAddUp(t *testing.T) {
	for _, acc := range []Accompaniment{AccompNone, AccompAll} {
		a := randomArrangement(t, 5, acc)
		var s xScore
		if err := xml.Unmarshal(MusicXML(a, 120, "test"), &s); err != nil {
			t.Fatal(err)
		}
		if len(s.Parts) != len(a.Parts) {
			t.Fatalf("%d parts, want %d", len(s.Parts), len(a.Parts))
		}
		for _, p := range s.Parts {
			ties := 0
			for _, m := range p.Measures {
				perVoice := map[int]int{}
				for _, it := range m.Items {
					if it.XMLName.Local == "note" && it.Chord == nil {
						perVoice[it.Voice] += it.Duration
					}
					for _, tie := range it.Ties {
						if tie.Type == "start" {
							ties++
						} else {
							ties--
						}
					}
				}
				for v, d := range perVoice {
					if d != StepsPerBar {
						t.Fatalf("%s measure %s voice %d: %d sixteenths, want %d", p.ID, m.Number, v, d, StepsPerBar)
					}
				}
			}
			if ties != 0 {
				t.Fatalf("%s: unbalanced ties (%d)", p.ID, ties)
			}
		}
	}
}

func TestSpelling(t *testing.T) {
	cases := []struct {
		tonic int
		minor bool
		pitch int
		want  string
	}{
		{4, true, 63, "D#4"},  // E minor: leading tone D#
		{2, true, 61, "C#4"},  // D minor: raised 7th is C#, not Db
		{5, false, 70, "Bb4"}, // F major
		{3, false, 68, "Ab4"}, // Eb major
		{8, true, 67, "F##4"}, // G# minor: leading tone F double-sharp
		{0, false, 66, "F#4"}, // C major chromatic note: sharp
		{0, true, 71, "B4"},   // C minor leading tone B natural
		{1, true, 60, "B#3"},  // C# minor leading tone, octave below C4
	}
	for _, c := range cases {
		step, alter, oct := newSpeller(c.tonic, scaleFor(c.minor)).spell(c.pitch)
		got := step
		for ; alter > 0; alter-- {
			got += "#"
		}
		for ; alter < 0; alter++ {
			got += "b"
		}
		got += string(rune('0' + oct))
		if got != c.want {
			t.Errorf("%s pitch %d: got %s, want %s", KeyName(c.tonic, scaleFor(c.minor)), c.pitch, got, c.want)
		}
	}
}

func TestGenomeFileRoundTrip(t *testing.T) {
	g := make([]byte, GenomeSize(64, Pop))
	rand.New(rand.NewSource(9)).Read(g)
	in := GenomeFile{Notes: 64, Acc: AccompBass | AccompDrums, Style: Pop, BPM: 96, Genome: g}
	b := MarshalGenome(in)
	out, err := UnmarshalGenome(b)
	if err != nil {
		t.Fatal(err)
	}
	if out.Notes != in.Notes || out.Acc != in.Acc || out.Style != in.Style || out.BPM != in.BPM || string(out.Genome) != string(g) {
		t.Fatal("genome file round trip changed the data")
	}
	b[len(b)-1] ^= 1
	if _, err := UnmarshalGenome(b); err == nil {
		t.Fatal("expected checksum error")
	}
	if _, err := UnmarshalGenome([]byte("MThd not a genome......")); err == nil {
		t.Fatal("expected magic error")
	}
}

func scaleFor(minor bool) *Scale {
	if minor {
		return ScaleMinor
	}
	return ScaleMajor
}
