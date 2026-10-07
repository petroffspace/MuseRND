package melody

import (
	"math"
	"math/rand"
	"os"
	"strings"
	"testing"
)

func TestBuildStyleAndFixedLength(t *testing.T) {
	s := Settings{Genre: "house", Mood: "calm", Scale: "dorian", Tonic: 2, Seconds: 60, BPM: 0,
		Parts: []string{"bass", "drums"}, Instruments: map[string]int{"lead": 11, "drums": 24}}
	st, acc, bpm, err := BuildStyle(s)
	if err != nil {
		t.Fatal(err)
	}
	if bpm != 105 { // 124 * 0.85
		t.Errorf("bpm %d, want 105", bpm)
	}
	if st.Bars != 26 { // 60 s * 105 bpm / 240
		t.Errorf("bars %d, want 26", st.Bars)
	}
	if acc != AccompBass|AccompDrums {
		t.Errorf("acc %v", acc)
	}
	if House.Instruments[PartLead].program != 81 {
		t.Fatal("BuildStyle modified the genre preset")
	}

	notes := NotesFor(st)
	g := make([]byte, GenomeSize(notes, st))
	rand.New(rand.NewSource(3)).Read(g)
	a := Arrange(g, notes, acc, st)
	if a.Bars != 26 || a.Steps != 26*StepsPerBar {
		t.Fatalf("arrangement %d bars / %d steps", a.Bars, a.Steps)
	}
	total := 0
	for _, n := range a.Notes {
		total += n.Steps
	}
	if total != 26*StepsPerBar {
		t.Fatalf("melody fills %d sixteenths, want %d", total, 26*StepsPerBar)
	}
	if a.KeyName() != "D dorian" {
		t.Errorf("key %s, want D dorian", a.KeyName())
	}
	if a.Part(PartChords) != nil {
		t.Error("chords were not requested")
	}
	if p, name := st.Instrument(PartLead); p != 11 || name != "Vibraphone" {
		t.Errorf("lead %d %s", p, name)
	}
	mx := string(MusicXML(a, bpm, "x"))
	for _, want := range []string{"<mode>dorian</mode>", "<fifths>0</fifths>", "Vibraphone", "<midi-program>25</midi-program>"} {
		if !strings.Contains(mx, want) {
			t.Errorf("MusicXML lacks %s", want)
		}
	}

	// genome file v3 rebuilds the same style
	b := MarshalGenome(GenomeFile{Notes: notes, Acc: acc, Style: st, BPM: bpm, Genome: g, Settings: &s})
	f, err := UnmarshalGenome(b)
	if err != nil {
		t.Fatal(err)
	}
	if f.BPM != bpm || f.Style.Bars != st.Bars || f.Acc != acc || f.Notes != notes {
		t.Fatal("v3 round trip changed settings")
	}
	if c1, _ := EvaluateGenome(g, notes, acc, st); func() float64 { c2, _ := EvaluateGenome(f.Genome, f.Notes, f.Acc, f.Style); return c2.Total }() != c1.Total {
		t.Fatal("v3 round trip scores differently")
	}
}

func TestBuildStyleRejectsBadInput(t *testing.T) {
	for _, s := range []Settings{
		{Genre: "polka"}, {Genre: "pop", Mood: "angry"}, {Genre: "pop", Scale: "x"},
		{Genre: "pop", BPM: 500}, {Genre: "pop", Seconds: 5}, {Genre: "pop", Tonic: 12},
		{Genre: "pop", Parts: []string{"kazoo"}}, {Genre: "pop", Instruments: map[string]int{"drums": 3}},
	} {
		if _, _, _, err := BuildStyle(s); err == nil {
			t.Errorf("%+v: expected error", s)
		}
	}
}

func TestForcedScales(t *testing.T) {
	rng := rand.New(rand.NewSource(8))
	for _, sc := range Scales {
		st, _, _, err := BuildStyle(Settings{Genre: "pop", Scale: sc.ID, Tonic: -1, Seconds: 20})
		if err != nil {
			t.Fatal(err)
		}
		n := NotesFor(st)
		g := make([]byte, GenomeSize(n, st))
		rng.Read(g)
		a := Arrange(g, n, AccompAll, st)
		if a.Scale != sc {
			t.Errorf("forced %s, got %s", sc.ID, a.Scale.ID)
		}
		for _, c := range a.Chords {
			if strings.HasSuffix(c.Name, "?") {
				t.Errorf("%s: unnamed chord %v", sc.ID, c.Tones)
			}
		}
	}
}

func TestTransposeAndTempoEdits(t *testing.T) {
	s := Settings{Genre: "pop", Tonic: 1, Scale: "minor", Seconds: 30, Parts: []string{"chords", "bass", "drums"}}
	st, acc, bpm, _ := BuildStyle(s)
	n := NotesFor(st)
	g := make([]byte, GenomeSize(n, st))
	rand.New(rand.NewSource(2)).Read(g)
	c0, a := EvaluateGenome(g, n, acc, st)

	// tempo change with bars pinned: same genome fits, same notes
	s2 := s
	s2.BPM, s2.Bars = 90, st.Bars
	st2, acc2, bpm2, err := BuildStyle(s2)
	if err != nil || bpm2 != 90 || st2.Bars != st.Bars || GenomeSize(NotesFor(st2), st2) != len(g) {
		t.Fatalf("tempo edit: %v bpm %d bars %d (was %d at %d bpm)", err, bpm2, st2.Bars, st.Bars, bpm)
	}
	if c, _ := EvaluateGenome(g, n, acc2, st2); c.Total != c0.Total {
		t.Fatal("tempo change altered the score")
	}

	// transpose +2: C# minor -> D# minor, every pitched note +2, drums unchanged
	tr := a.Transposed(2)
	if tr.KeyName() != "D# minor" || a.KeyName() != "C# minor" {
		t.Fatalf("keys %s -> %s", a.KeyName(), tr.KeyName())
	}
	for i, p := range a.Parts {
		for k, e := range p.Events {
			want := e.Pitch + 2
			if p.Kind == PartDrums {
				want = e.Pitch
			}
			if tr.Parts[i].Events[k].Pitch != want {
				t.Fatalf("%s note %d: %d -> %d", p.Name, k, e.Pitch, tr.Parts[i].Events[k].Pitch)
			}
		}
	}
	if a.Chords[0].Name == tr.Chords[0].Name {
		t.Error("chord names not transposed")
	}
	if xml := string(MusicXML(tr, bpm, "t")); !strings.Contains(xml, "<fifths>6</fifths>") { // D# minor = 6 sharps
		t.Error("MusicXML key signature not transposed")
	}
	if _, _, _, err := BuildStyle(Settings{Genre: "pop", Transpose: 13}); err == nil {
		t.Error("transpose 13 accepted")
	}
}

func TestTransposeLock(t *testing.T) {
	s := Settings{Genre: "pop", Tonic: 0, Scale: "major", Seconds: 16, BPM: 120, Parts: []string{"chords", "bass", "drums"}}
	st, acc, bpm, _ := BuildStyle(s) // 8 bars
	n := NotesFor(st)
	g := make([]byte, GenomeSize(n, st))
	rand.New(rand.NewSource(6)).Read(g)
	c0, a := EvaluateGenome(g, n, acc, st)

	// key change: everything locked in bars 1-4, bars 5-8 up a whole tone
	s.Transpose, s.TransposeLock = 2, &TransposeLock{Parts: []string{"melody", "chords", "bass"}, From: 1, To: 4}
	st2, acc2, _, err := BuildStyle(s)
	if err != nil {
		t.Fatal(err)
	}
	c2, scored := EvaluateGenome(g, n, acc2, st2)
	if c2.Total != c0.Total {
		t.Fatal("transpose lock changed the score")
	}
	out := scored.Output()
	if out.KeyName() != "C major" || out.TonicAt(4) != 2 || out.TonicAt(3) != 0 {
		t.Fatalf("keys: start %s, bar 4 %d, bar 5 %d", out.KeyName(), out.TonicAt(3), out.TonicAt(4))
	}
	for i, p := range a.Parts {
		for k, e := range p.Events {
			want := e.Pitch
			if p.Kind != PartDrums && e.Start/StepsPerBar >= 4 {
				want += 2
			}
			if got := out.Parts[i].Events[k].Pitch; got != want {
				t.Fatalf("%s at step %d: %d, want %d", p.Name, e.Start, got, want)
			}
		}
	}
	if out.Chords[3].Name != a.Chords[3].Name || out.Chords[4].Name == a.Chords[4].Name {
		t.Error("chord names do not follow the key change")
	}
	xml := string(MusicXML(out, bpm, "k"))
	if strings.Count(xml, "<key>") != 2*3 { // C at the start, D from bar 5, in 3 pitched parts
		t.Errorf("MusicXML has %d key signatures", strings.Count(xml, "<key>"))
	}
	if !strings.Contains(xml, "<fifths>2</fifths>") {
		t.Error("no D major key signature after the change")
	}
	if mid := MIDI(out, bpm, "k"); strings.Count(string(mid), "\xff\x59\x02") != 2 {
		t.Error("MIDI lacks the key change event")
	}

	// melody kept, accompaniment transposed: key follows the chords, no change
	s.TransposeLock = &TransposeLock{Parts: []string{"melody"}}
	st3, acc3, _, _ := BuildStyle(s)
	_, sc3 := EvaluateGenome(g, n, acc3, st3)
	o3 := sc3.Output()
	if o3.BarTonic != nil || o3.KeyName() != "D major" || o3.Parts[0].Events[0].Pitch != a.Parts[0].Events[0].Pitch {
		t.Fatalf("melody-locked transpose: key %s, BarTonic %v", o3.KeyName(), o3.BarTonic)
	}
	for _, bad := range []TransposeLock{{Parts: []string{"drums"}}, {Parts: []string{"melody"}, From: 3, To: 2}, {Parts: []string{"bass"}, From: 1, To: 99}} {
		s.TransposeLock = &bad
		if _, _, _, err := BuildStyle(s); err == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
}

func TestTempoSection(t *testing.T) {
	s := Settings{Genre: "pop", Tonic: -1, Seconds: 16, BPM: 120, Parts: []string{"chords", "bass", "drums"},
		TempoSection: &TempoSection{From: 5, To: 8, BPM: 90}}
	st, acc, bpm, err := BuildStyle(s)
	if err != nil || st.Bars != 8 {
		t.Fatalf("%v, %d bars", err, st.Bars)
	}
	n := NotesFor(st)
	g := make([]byte, GenomeSize(n, st))
	rand.New(rand.NewSource(4)).Read(g)
	_, a := EvaluateGenome(g, n, acc, st)
	tl := st.NewTimeline(a.Bars, bpm)
	want := 4*2.0 + 4*240/90.0
	if got := tl.At(a.Steps); math.Abs(got-want) > 1e-9 {
		t.Fatalf("length %.3f s, want %.3f", got, want)
	}
	if got := tl.At(4*StepsPerBar + 4); math.Abs(got-(8+60.0/90)) > 1e-9 { // beat 2 of bar 5
		t.Fatalf("bar 5 beat 2 at %.4f", got)
	}
	mid := string(MIDI(a, bpm, "t"))
	if strings.Count(mid, "\xff\x51\x03") != 2 {
		t.Errorf("MIDI has %d tempo events, want 2", strings.Count(mid, "\xff\x51\x03"))
	}
	xml := string(MusicXML(a, bpm, "t"))
	if strings.Count(xml, "<per-minute>") != 2 || !strings.Contains(xml, `<sound tempo="90"/>`) {
		t.Error("MusicXML lacks the tempo change")
	}
	for _, bad := range []TempoSection{{From: 0, To: 3, BPM: 90}, {From: 5, To: 9, BPM: 90}, {From: 2, To: 3, BPM: 300}} {
		s.TempoSection = &bad
		if _, _, _, err := BuildStyle(s); err == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
}

func TestInstrumentLock(t *testing.T) {
	s := Settings{Genre: "pop", Tonic: -1, Seconds: 16, BPM: 120, Parts: []string{"chords", "bass", "drums"},
		Instruments:    map[string]int{"lead": 11, "drums": 0},
		InstrumentLock: &InstrumentLock{Parts: []string{"melody", "drums"}, From: 1, To: 4, Instruments: map[string]int{"melody": 73, "drums": 25}}}
	st, acc, bpm, err := BuildStyle(s)
	if err != nil {
		t.Fatal(err)
	}
	if p, name, alt := st.InstrumentAtBar(PartLead, 3); p != 73 || name != "Flute" || !alt {
		t.Fatalf("bar 4 melody: %d %s %v", p, name, alt)
	}
	if p, _, alt := st.InstrumentAtBar(PartLead, 4); p != 11 || alt {
		t.Fatalf("bar 5 melody: %d %v", p, alt)
	}
	if p, _, _ := st.InstrumentAtBar(PartChords, 0); p != int(Pop.Instruments[PartChords].program) {
		t.Fatal("unlocked part changed")
	}
	n := NotesFor(st)
	g := make([]byte, GenomeSize(n, st))
	rand.New(rand.NewSource(12)).Read(g)
	_, a := EvaluateGenome(g, n, acc, st)
	mid := string(MIDI(a, bpm, "i"))
	if !strings.Contains(mid, "\xc0\x49") || !strings.Contains(mid, "\xc0\x0b") || !strings.Contains(mid, "\xc9\x19") || !strings.Contains(mid, "\xc9\x00") {
		t.Error("MIDI lacks the program changes (flute->vibraphone, TR-808->standard)")
	}
	xml := string(MusicXML(a, bpm, "i"))
	for _, want := range []string{`<score-instrument id="P1-I2">`, `<instrument id="P1-I2"/>`, `<instrument id="P1-I1"/>`, "-I36b", "Flute", "Vibraphone"} {
		if !strings.Contains(xml, want) {
			t.Errorf("MusicXML lacks %s", want)
		}
	}
	if path := os.Getenv("MUSICXML_OUT"); path != "" { // for an external schema check
		os.WriteFile(path, []byte(xml), 0o644)
	}
	for _, bad := range []InstrumentLock{{Parts: []string{"melody"}, From: 1, To: 99, Instruments: map[string]int{"melody": 1}},
		{Parts: []string{"melody"}, From: 1, To: 2}, {Parts: []string{"drums"}, From: 1, To: 2, Instruments: map[string]int{"drums": 3}}} {
		s.InstrumentLock = &bad
		if _, _, _, err := BuildStyle(s); err == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
}

func TestMoodSliders(t *testing.T) {
	sl := &MoodSliders{Energy: 1, Darkness: 0.8, Density: 0.5, Swing: 0.5, Complexity: 0.6}
	st, _, bpm, err := BuildStyle(Settings{Genre: "pop", Tonic: -1, Seconds: 20, Sliders: sl})
	if err != nil {
		t.Fatal(err)
	}
	if bpm != 138 || st.ModeBias != -1 || st.Swing != 0.5 || !st.Sevenths || st.Durations[0] >= Pop.Durations[2] && st.Durations == Pop.Durations {
		t.Fatalf("bpm %d bias %d swing %.2f sevenths %v durations %v", bpm, st.ModeBias, st.Swing, st.Sevenths, st.Durations)
	}
	if st.RestTarget >= Pop.RestTarget || st.MotifIdeal >= Pop.MotifIdeal || st.RangeFull <= Pop.RangeFull {
		t.Fatalf("rest %.2f motif %.2f range %d", st.RestTarget, st.MotifIdeal, st.RangeFull)
	}
	// blues keeps its shuffle unless the slider removes it
	bl, _, _, _ := BuildStyle(Settings{Genre: "blues", Tonic: -1, Sliders: &MoodSliders{}})
	st2, _, _, _ := BuildStyle(Settings{Genre: "blues", Tonic: -1, Sliders: &MoodSliders{Swing: -1}})
	if bl.Swing != 1 || st2.Swing != 0 {
		t.Fatalf("blues swing %.1f / %.1f", bl.Swing, st2.Swing)
	}
	// a new preset named without sliders uses its preset sliders
	m, _ := MoodByID("dreamy")
	st3, _, bpm3, _ := BuildStyle(Settings{Genre: "pop", Mood: "dreamy", Tonic: -1})
	if bpm3 != SliderTempo(Pop, m.Sliders) || st3.Durations == Pop.Durations {
		t.Fatalf("dreamy: bpm %d durations %v", bpm3, st3.Durations)
	}
	// legacy moods without sliders behave exactly as before (saved songs)
	st4, _, bpm4, _ := BuildStyle(Settings{Genre: "house", Mood: "calm", Tonic: -1})
	if bpm4 != 105 || st4.DrumVolume != 75 {
		t.Fatalf("legacy calm: bpm %d drums %d", bpm4, st4.DrumVolume)
	}
	// sliders survive a genome file round trip
	s := Settings{Genre: "pop", Tonic: -1, Seconds: 12, Sliders: sl}
	st5, acc, bpm5, _ := BuildStyle(s)
	n := NotesFor(st5)
	g := make([]byte, GenomeSize(n, st5))
	f, err := UnmarshalGenome(MarshalGenome(GenomeFile{Notes: n, Acc: acc, Style: st5, BPM: bpm5, Genome: g, Settings: &s}))
	if err != nil || f.Settings.Sliders == nil || *f.Settings.Sliders != *sl || f.Style.Swing != 0.5 {
		t.Fatalf("round trip: %v %+v", err, f.Settings)
	}
}
