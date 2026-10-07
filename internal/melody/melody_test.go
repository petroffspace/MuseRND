package melody

import (
	"math/rand"
	"testing"
)

// genomeFor encodes notes back into genome bytes.
func genomeFor(notes []Note) []byte {
	pix := make([]byte, 0, len(notes)*3)
	for _, n := range notes {
		g := byte(0)
		for i, s := range Pop.Durations {
			if s == n.Steps {
				g = byte(i<<5 | 16)
				break
			}
		}
		b, r := byte(200), pitchToR(n.Pitch)
		if n.Pitch == Rest {
			b, r = 0, 0
		}
		pix = append(pix, b, g, r)
	}
	return pix
}

func TestPitchToRRoundTrip(t *testing.T) {
	for p := LowestPitch; p < LowestPitch+PitchRange; p++ {
		if got := Decode([]byte{200, 0, pitchToR(p)}, Pop)[0].Pitch; got != p {
			t.Fatalf("pitch %d decoded as %d", p, got)
		}
	}
}

func TestDecodeChannels(t *testing.T) {
	n := Decode([]byte{200, 0xFF, 0}, Pop)[0]
	if n.Pitch != LowestPitch || n.Steps != 8 || n.Velocity != 200 {
		t.Fatalf("unexpected note %+v", n)
	}
	if Decode([]byte{RestBelow - 1, 0, 128}, Pop)[0].Pitch != Rest {
		t.Fatal("low blue should be a rest")
	}
}

func TestDetectorPrefersMusic(t *testing.T) {
	// "Twinkle Twinkle Little Star", twice.
	var tune []Note
	for range 2 {
		for _, p := range []int{60, 60, 67, 67, 69, 69, 67, 65, 65, 64, 64, 62, 62, 60} {
			tune = append(tune, Note{Pitch: p, Steps: 2, Velocity: 200})
		}
		tune[len(tune)-1].Steps = 4
		tune = append(tune, Note{Pitch: Rest, Steps: 2})
	}
	music := Evaluate(Decode(genomeFor(tune), Pop), Pop)
	if music.KeyName != "C major" {
		t.Errorf("key detected as %s, want C major", music.KeyName)
	}

	rng := rand.New(rand.NewSource(1))
	worst := 0.0
	for range 200 {
		pix := make([]byte, len(tune)*3)
		rng.Read(pix)
		worst = max(worst, Evaluate(Decode(pix, Pop), Pop).Total)
	}
	t.Logf("tune %.1f, best of 200 random images %.1f", music.Total, worst)
	if music.Total < 85 || music.Total <= worst+20 {
		t.Fatalf("detector does not separate music (%.1f) from noise (%.1f)", music.Total, worst)
	}
}

func TestEvolveReachesTarget(t *testing.T) {
	rng := NewRand(rand.New(rand.NewSource(42)))
	g, best, _ := Evolve(rng, Options{Notes: 64, Style: Pop, Population: 64, Generations: 5000, Target: 92})
	if best.Total < 92 {
		t.Fatalf("best score %.1f, want >= 92", best.Total)
	}
	if got, _ := EvaluateGenome(g, 64, AccompNone, Pop); got.Total != best.Total {
		t.Fatalf("returned genome scores %.1f, reported %.1f", got.Total, best.Total)
	}
}

func TestEvolveWholeArrangement(t *testing.T) {
	rng := NewRand(rand.New(rand.NewSource(7)))
	opt := Options{Notes: 64, Acc: AccompAll, Style: Pop, Population: 64, Generations: 3000, Target: 90}
	g, best, gens := Evolve(rng, opt)
	t.Logf("after %d generations:\n%s", gens, best)
	if best.Total < 80 {
		t.Fatalf("best total %.1f, want >= 80", best.Total)
	}
	if len(g) != GenomeSize(64, Pop) {
		t.Fatalf("genome %d bytes, want %d", len(g), GenomeSize(64, Pop))
	}
}

// twinkle returns "Twinkle Twinkle" (one phrase per bar) as genome bytes
// followed by bar genes from bar(b).
func twinkle(bar func(b int) []byte) []byte {
	var tune []Note
	for _, p := range []int{60, 60, 67, 67, 69, 69, 67, 67, 65, 65, 64, 64, 62, 62, 60, 60} {
		tune = append(tune, Note{Pitch: p, Steps: 4, Velocity: 200})
	}
	g := genomeFor(tune)
	for b := 0; b < MaxBars(len(tune), Pop); b++ {
		g = append(g, bar(b)...)
	}
	return g
}

func TestDetectorPrefersGoodAccompaniment(t *testing.T) {
	degrees := []byte{0, 3, 3, 0} // I IV IV I
	good := twinkle(func(b int) []byte {
		crash := byte(255)
		if b == 0 {
			crash = 0
		}
		return []byte{
			degrees[b%4], 0, // chord, held for the bar
			0x80, 0x40, 0x40, 0x40, 0x82, 0x40, 0x83, 0x40, // bass: root (half), fifth, octave
			0x01, 0x01, 0x10, 0x10, 0x55, 0x55, // kick 1+3, snare 2+4, eighth hats
			crash,
		}
	})
	gc, ga := EvaluateGenome(good, 16, AccompAll, Pop)
	t.Logf("good:\n%s%s", gc, FormatChords(ga.Chords))
	if gc.Harmony < 0.7 || gc.Bass < 0.9 || gc.Drums < 0.9 {
		t.Fatalf("hand-written accompaniment scored low: harmony %.2f bass %.2f drums %.2f", gc.Harmony, gc.Bass, gc.Drums)
	}
	if got := FormatChords(ga.Chords); got != "C | F | F | C" {
		t.Fatalf("chords %q", got)
	}

	rng := rand.New(rand.NewSource(1))
	worst := 0.0
	for range 200 {
		bad := twinkle(func(int) []byte {
			b := make([]byte, BarGenes)
			rng.Read(b)
			return b
		})
		c, _ := EvaluateGenome(bad, 16, AccompAll, Pop)
		worst = max(worst, c.Total)
	}
	t.Logf("good %.1f, best of 200 random accompaniments %.1f", gc.Total, worst)
	if gc.Total <= worst+5 {
		t.Fatalf("detector does not separate good (%.1f) from random (%.1f) accompaniment", gc.Total, worst)
	}
}

func TestDiatonicChords(t *testing.T) {
	var major, minor []Chord
	for d := 0; d < 7; d++ {
		major = append(major, diatonicChord(d, 0, ScaleMajor, false))
		minor = append(minor, diatonicChord(d, 9, ScaleMinor, false))
	}
	if got := FormatChords(major); got != "C | Dm | Em | F | G | Am | Bdim" {
		t.Errorf("C major: %s", got)
	}
	if got := FormatChords(minor); got != "Am | Bdim | C | Dm | E | F | G#dim" {
		t.Errorf("A minor: %s", got)
	}
}

func TestParseAccompaniment(t *testing.T) {
	for spec, want := range map[string]Accompaniment{
		"all": AccompAll, "none": AccompNone, "both": AccompBass | AccompChords,
		"drums": AccompDrums, "bass,drums": AccompBass | AccompDrums,
		"chords, drums": AccompChords | AccompDrums,
	} {
		if got, err := ParseAccompaniment(spec); err != nil || got != want {
			t.Errorf("%q: got %v, %v; want %v", spec, got, err, want)
		}
	}
	if _, err := ParseAccompaniment("bongos"); err == nil {
		t.Error("expected error for unknown part")
	}
}

func BenchmarkEvaluateGenome(b *testing.B) {
	g := make([]byte, GenomeSize(256, Pop))
	rand.New(rand.NewSource(1)).Read(g)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		EvaluateGenome(g, 256, AccompAll, Pop)
	}
}
