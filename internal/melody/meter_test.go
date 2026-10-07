package melody

import (
	"math"
	"math/rand"
	"os"
	"strings"
	"testing"
)

func TestMeters(t *testing.T) {
	for _, c := range []struct {
		genre          string
		bars, beat     int
		strong         []int
		barSeconds     float64
		timeSig        string
		midiTimeSig    string
		xmlTime, beat2 string
		quarterTempo   string
	}{
		{"pop", 16, 4, []int{0, 8}, 2, "4/4", "\x04\x02\x18\x08", "<beats>4</beats><beat-type>4</beat-type>", "<beat-unit>quarter</beat-unit><per-minute>", `tempo="120"`},
		{"waltz", 12, 4, []int{0}, 1.2, "3/4", "\x03\x02\x18\x08", "<beats>3</beats><beat-type>4</beat-type>", "<beat-unit>quarter</beat-unit><per-minute>", `tempo="150"`},
		{"jig", 12, 6, []int{0, 6}, 120.0 / 110, "6/8", "\x06\x03\x24\x08", "<beats>6</beats><beat-type>8</beat-type>", "<beat-unit-dot/>", `tempo="165"`},
	} {
		st, acc, bpm, err := BuildStyle(Settings{Genre: c.genre, Tonic: -1, Seconds: 30, Parts: []string{"chords", "bass", "drums"}})
		if err != nil {
			t.Fatal(err)
		}
		if st.BarSteps() != c.bars || st.BeatSteps() != c.beat || st.TimeSignature() != c.timeSig {
			t.Errorf("%s: bar %d beat %d %s", c.genre, st.BarSteps(), st.BeatSteps(), st.TimeSignature())
		}
		for s := 0; s < st.BarSteps(); s++ {
			want := 0
			for i, p := range c.strong {
				if s == p {
					want = 2 - min(i, 1)
				}
			}
			if st.Strong(s) != want {
				t.Errorf("%s: step %d strength %d, want %d", c.genre, s, st.Strong(s), want)
			}
		}
		if want := int(math.Round(30 / c.barSeconds)); st.Bars != want {
			t.Errorf("%s: %d bars for 30 s, want %d", c.genre, st.Bars, want)
		}
		tl := st.NewTimeline(st.Bars, bpm)
		if got := tl.At(st.BarSteps()); math.Abs(got-c.barSeconds) > 1e-9 {
			t.Errorf("%s: bar lasts %.4f s, want %.4f", c.genre, got, c.barSeconds)
		}

		n := NotesFor(st)
		g := make([]byte, GenomeSize(n, st))
		rand.New(rand.NewSource(13)).Read(g)
		a := Arrange(g, n, acc, st)
		if a.Steps != st.Bars*st.BarSteps() {
			t.Errorf("%s: %d steps", c.genre, a.Steps)
		}
		for _, p := range a.Parts {
			for _, e := range p.Events {
				if e.Start+e.Steps > a.Steps {
					t.Fatalf("%s: %s note runs past the end", c.genre, p.Name)
				}
			}
		}
		if d := a.Part(PartDrums); d != nil {
			for _, e := range d.Events {
				if e.Start/st.BarSteps() >= st.Bars {
					t.Fatalf("%s: drum hit outside the song", c.genre)
				}
			}
		}
		mid := string(MIDI(a, bpm, c.genre))
		if !strings.Contains(mid, "\xff\x58\x04"+c.midiTimeSig) {
			t.Errorf("%s: MIDI time signature missing", c.genre)
		}
		xml := string(MusicXML(a, bpm, c.genre))
		for _, want := range []string{c.xmlTime, c.beat2, c.quarterTempo} {
			if !strings.Contains(xml, want) {
				t.Errorf("%s: MusicXML lacks %s", c.genre, want)
			}
		}
		if path := os.Getenv("MUSICXML_DIR"); path != "" {
			os.WriteFile(path+"/"+c.genre+".musicxml", []byte(xml), 0o644)
		}
	}
}
