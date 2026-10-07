package melody

import (
	"bytes"
	"math/rand"
	"testing"
)

func TestVariationKeepsLockedParts(t *testing.T) {
	st, acc, _, err := BuildStyle(Settings{Genre: "pop", Seconds: 20, Tonic: -1, Parts: []string{"chords", "bass", "drums"}})
	if err != nil {
		t.Fatal(err)
	}
	n := NotesFor(st)
	parent := make([]byte, GenomeSize(n, st))
	rand.New(rand.NewSource(5)).Read(parent)

	for _, amount := range []int{3, -1} {
		rng := NewRand(rand.New(rand.NewSource(int64(9 + amount))))
		child, _, _ := Evolve(rng, Options{Notes: n, Acc: acc, Style: st, Population: 16, Generations: 200, Target: 101,
			Seed: parent, Lock: LockMelody | LockDrums, Amount: amount})
		if !bytes.Equal(child[:n*3], parent[:n*3]) {
			t.Fatalf("amount %d: locked melody changed", amount)
		}
		changedChords, changedBass := false, false
		for b := 0; b < MaxBars(n, st); b++ {
			pg, cg := barGenes(parent, n, b), barGenes(child, n, b)
			if !bytes.Equal(pg[10:17], cg[10:17]) {
				t.Fatalf("amount %d: locked drums changed in bar %d", amount, b)
			}
			changedChords = changedChords || !bytes.Equal(pg[0:2], cg[0:2])
			changedBass = changedBass || !bytes.Equal(pg[2:10], cg[2:10])
		}
		if !changedChords || !changedBass {
			t.Errorf("amount %d: unlocked parts did not change (chords %v, bass %v)", amount, changedChords, changedBass)
		}
	}
}

func TestTasteLearnsPreference(t *testing.T) {
	// liked songs have long notes and much repetition, disliked the opposite
	mk := func(length, repeat, noise float64) []float64 {
		f := make([]float64, len(FeatureNames))
		for i := range f {
			f[i] = 0.5 + noise
		}
		f[8], f[6] = length, repeat // note length, repetition (both character)
		return f
	}
	liked := [][]float64{mk(0.6, 0.8, 0.01), mk(0.7, 0.9, -0.02), mk(0.65, 0.85, 0)}
	disliked := [][]float64{mk(0.2, 0.3, 0.02), mk(0.25, 0.2, -0.01)}
	tm, err := LearnTaste(liked, disliked)
	if err != nil {
		t.Fatal(err)
	}
	if a, b := tm.Score(mk(0.7, 0.85, 0)), tm.Score(mk(0.2, 0.25, 0)); a < 0.8 || b > 0.2 {
		t.Fatalf("taste scores liked-like %.2f, disliked-like %.2f", a, b)
	}
	prefs := tm.Prefs(3)
	if len(prefs) < 2 || prefs[0].Weight <= 0 {
		t.Fatalf("prefs %+v", prefs)
	}
	names := map[string]bool{prefs[0].Feature: true, prefs[1].Feature: true}
	if !names["note length"] || !names["repetition"] {
		t.Errorf("expected note length and repetition, got %+v", prefs)
	}
	if _, err := LearnTaste(liked[:1], nil); err == nil {
		t.Error("one rating should not be enough")
	}
	only, _ := LearnTaste(liked, nil)
	if only.Score(liked[0]) < only.Score(disliked[0]) {
		t.Error("likes-only taste should prefer liked songs")
	}
}

func TestTasteSteersSearch(t *testing.T) {
	st, acc, _, _ := BuildStyle(Settings{Genre: "pop", Seconds: 20, Tonic: -1, Parts: []string{"chords", "bass", "drums"}})
	n := NotesFor(st)
	// a taste for much silence, learned from fake ratings
	f := func(silence float64) []float64 {
		v := make([]float64, len(FeatureNames))
		for i := range v {
			v[i] = 0.5
		}
		v[4] = silence
		return v
	}
	tm, _ := LearnTaste([][]float64{f(0.5), f(0.45)}, [][]float64{f(0.05), f(0.1)})
	run := func(taste *Taste) float64 {
		g, _, _ := Evolve(NewRand(rand.New(rand.NewSource(1))), Options{Notes: n, Acc: acc, Style: st, Population: 32,
			Generations: 600, Target: 101, Taste: taste, TasteWeight: 0.5})
		c, _ := EvaluateGenome(g, n, acc, st)
		return c.Melody.RestShare
	}
	plain, steered := run(nil), run(tm)
	t.Logf("silence share: plain %.2f, with taste for silence %.2f", plain, steered)
	if steered <= plain {
		t.Fatalf("taste did not steer the search (%.2f vs %.2f)", steered, plain)
	}
}

func TestSectionLock(t *testing.T) {
	st, acc, _, err := BuildStyle(Settings{Genre: "pop", Seconds: 32, Tonic: 0, Scale: "major", Parts: []string{"chords", "bass", "drums"}})
	if err != nil {
		t.Fatal(err)
	}
	n := NotesFor(st)
	parent := make([]byte, GenomeSize(n, st))
	rand.New(rand.NewSource(11)).Read(parent)
	const from, to = 2, 5 // bars 3-5

	for _, amount := range []int{20, -1} {
		rng := NewRand(rand.New(rand.NewSource(int64(30 + amount))))
		child, _, _ := Evolve(rng, Options{Notes: n, Acc: acc, Style: st, Population: 16, Generations: 300, Target: 101,
			Seed: parent, Lock: LockMelody | LockChords, Amount: amount, LockFrom: from, LockTo: to})

		pa := Arrange(parent, n, acc, st)
		ca := Arrange(child, n, acc, st)
		lo, hi := from*StepsPerBar, to*StepsPerBar
		in := func(a Arrangement, k PartKind) []Event {
			var out []Event
			for _, e := range a.Part(k).Events {
				if e.Start < hi && e.Start+e.Steps > lo {
					out = append(out, e)
				}
			}
			return out
		}
		pm, cm := in(pa, PartLead), in(ca, PartLead)
		if len(pm) != len(cm) {
			t.Fatalf("amount %d: locked melody has %d notes, want %d", amount, len(cm), len(pm))
		}
		for i := range pm {
			if pm[i] != cm[i] {
				t.Fatalf("amount %d: locked melody note %d changed: %+v -> %+v", amount, i, pm[i], cm[i])
			}
		}
		for b := from; b < to; b++ {
			if pa.Chords[b].Name != ca.Chords[b].Name || barGenes(parent, n, b)[1] != barGenes(child, n, b)[1] {
				t.Fatalf("amount %d: locked chord in bar %d changed", amount, b+1)
			}
		}
		// outside the section things move
		changedOutside := false
		for b := 0; b < MaxBars(n, st); b++ {
			if b >= from && b < to {
				continue
			}
			if string(barGenes(parent, n, b)) != string(barGenes(child, n, b)) {
				changedOutside = true
			}
		}
		pAll, cAll := pa.Part(PartLead).Events, ca.Part(PartLead).Events
		melodyChanged := len(pAll) != len(cAll)
		for i := 0; !melodyChanged && i < len(pAll); i++ {
			melodyChanged = pAll[i] != cAll[i]
		}
		if !changedOutside || !melodyChanged {
			t.Errorf("amount %d: nothing changed outside the section (bars %v, melody %v)", amount, changedOutside, melodyChanged)
		}
	}
}
