package melody

import (
	"encoding/xml"
	"math/rand"
	"testing"
)

func TestSeventhChords(t *testing.T) {
	var major, minor, blues []Chord
	for d := 0; d < 7; d++ {
		major = append(major, diatonicChord(d, 0, ScaleMajor, true))
		minor = append(minor, diatonicChord(d, 9, ScaleMinor, true))
	}
	for _, d := range Blues.ChordDegrees {
		blues = append(blues, dominantChord(d, 9))
	}
	if got := FormatChords(major); got != "Cmaj7 | Dm7 | Em7 | Fmaj7 | G7 | Am7 | Bm7b5" {
		t.Errorf("C major sevenths: %s", got)
	}
	if got := FormatChords(minor); got != "Am7 | Bm7b5 | Cmaj7 | Dm7 | E7 | Fmaj7 | G#dim7" {
		t.Errorf("A minor sevenths: %s", got)
	}
	if got := FormatChords(blues); got != "A7 | D7 | E7" {
		t.Errorf("A blues: %s", got)
	}
}

func TestBluesKeyAndSpelling(t *testing.T) {
	// A minor-pentatonic blues lick: A C D Eb E G A
	var lick []Note
	for _, p := range []int{57, 60, 62, 63, 64, 67, 69, 67, 64, 60, 57, 57} {
		lick = append(lick, Note{Pitch: p, Steps: 2, Velocity: 200})
	}
	if tonic, sc := DetectKey(lick, Blues); KeyName(tonic, sc) != "A blues" {
		t.Fatalf("detected %s", KeyName(tonic, sc))
	}
	sp := newSpeller(9, ScaleBlues)
	for pitch, want := range map[int]string{60: "C", 63: "Eb", 67: "G", 61: "C#", 66: "F#"} {
		step, alter, _ := sp.spell(pitch)
		got := step + map[int]string{-1: "b", 0: "", 1: "#"}[alter]
		if got != want {
			t.Errorf("A blues: pitch %d spelled %s, want %s", pitch, got, want)
		}
	}
}

func TestSwingTicks(t *testing.T) {
	if StepTick(2, 0) != 240 || StepTick(2, 1) != 320 || StepTick(6, 1) != 800 || StepTick(4, 1) != 480 || StepTick(2, 0.5) != 280 {
		t.Fatal("swing timing wrong")
	}
	if swingXML(1) != "<swing><first>2</first><second>1</second><swing-type>eighth</swing-type></swing>" || swingXML(0) != "" {
		t.Fatalf("swing XML %q", swingXML(1))
	}
}

func TestHouseDrumsPreferFourOnTheFloor(t *testing.T) {
	g := make([]byte, GenomeSize(64, House))
	rand.New(rand.NewSource(4)).Read(g)
	for b := 0; b < MaxBars(64, House); b++ {
		bg := barGenes(g, 64, b)
		bg[10], bg[11] = 0x11, 0x11 // kick on every beat
		bg[12], bg[13] = 0x10, 0x10 // clap on 2 and 4
		bg[14], bg[15] = 0x44, 0x44 // offbeat hats
		bg[16] = 255
	}
	house, _ := EvaluateGenome(g, 64, AccompAll, House)
	pop, _ := EvaluateGenome(g, 64, AccompAll, Pop)
	t.Logf("four-on-the-floor drums: house %.2f, pop %.2f", house.Drums, pop.Drums)
	if house.Drums < 0.75 || house.Drums <= pop.Drums {
		t.Fatalf("house drums %.2f should beat pop drums %.2f", house.Drums, pop.Drums)
	}
}

func TestStylesEvolveAndExport(t *testing.T) {
	for _, st := range Styles {
		t.Run(st.Name, func(t *testing.T) {
			rng := NewRand(rand.New(rand.NewSource(int64(st.ID) + 11)))
			first := -1.0
			g, best, _ := Evolve(rng, Options{Notes: 48, Acc: AccompAll, Style: st, Population: 32, Generations: 800, Target: 101,
				Progress: func(gen int, c Composition) {
					if first < 0 {
						first = c.Total
					}
				}})
			t.Logf("%.1f -> %.1f\n%s", first, best.Total, best)
			if best.Total < first+5 {
				t.Errorf("evolution did not improve (%.1f -> %.1f)", first, best.Total)
			}
			_, a := EvaluateGenome(g, 48, AccompAll, st)
			if st == Ambient {
				d := a.Part(PartDrums)
				if d == nil {
					t.Fatal("ambient should have its soft drum kit")
				}
				for _, e := range d.Events {
					if e.Velocity > 230*55/100 {
						t.Fatalf("ambient drum hit too loud: %d", e.Velocity)
					}
				}
			}
			var s xScore
			if err := xml.Unmarshal(MusicXML(a, st.BPM, st.Name), &s); err != nil {
				t.Fatal(err)
			}
			for _, p := range s.Parts {
				for _, m := range p.Measures {
					perVoice := map[int]int{}
					for _, it := range m.Items {
						if it.XMLName.Local == "note" && it.Chord == nil {
							perVoice[it.Voice] += it.Duration
						}
					}
					for v, d := range perVoice {
						if d != st.BarSteps() {
							t.Fatalf("%s measure %s voice %d: %d sixteenths", p.ID, m.Number, v, d)
						}
					}
				}
			}
			if b := MIDI(a, st.BPM, st.Name); string(b[:4]) != "MThd" {
				t.Fatal("bad MIDI")
			}
		})
	}
}

func TestChordsBelowMelodyWithoutDoubling(t *testing.T) {
	rng := rand.New(rand.NewSource(21))
	for _, st := range Styles {
		for trial := 0; trial < 20; trial++ {
			g := make([]byte, GenomeSize(64, st))
			rng.Read(g)
			a := Arrange(g, 64, AccompAll, st)
			mel, _ := melodyTimeline(&a)
			for _, e := range a.Part(PartChords).Events {
				if m := mel[e.Start]; m != Rest && m%12 == e.Pitch%12 {
					t.Fatalf("%s: chord note %d doubles melody %d at step %d", st.Name, e.Pitch, m, e.Start)
				}
				bs := st.BarSteps()
				b := e.Start / bs
				low := 1000
				for _, m := range mel[b*bs : (b+1)*bs] {
					if m != Rest {
						low = min(low, m)
					}
				}
				if low != 1000 && low-3 >= chordTopMin && e.Pitch > low-3 {
					t.Fatalf("%s: chord note %d not below melody low %d in bar %d", st.Name, e.Pitch, low, b)
				}
			}
			c, _ := EvaluateGenome(g, 64, AccompAll, st)
			found := false
			for _, s := range c.hSub {
				found = found || s.name == "fills gaps"
			}
			if !found {
				t.Fatalf("%s: separation not scored", st.Name)
			}
		}
	}
}
