package melody

import (
	"bytes"
	"encoding/binary"
	"math/rand"
	"os"
	"strings"
	"testing"
)

func sixteenParts() Settings {
	return Settings{Genre: "pop", Tonic: -1, Seconds: 16, BPM: 120, Parts: []string{"chords", "bass", "drums"},
		Extras: []ExtraPart{
			{"counter", 73}, {"arpeggio", 46}, {"pad", 89}, {"stabs", 61},
			{"percussion", 62}, {"percussion", 54}, {"counter", 41}, {"arpeggio", 4},
			{"pad", 50}, {"stabs", 56}, {"counter", 11}, {"percussion", 70},
		}}
}

// midiChannels returns, per track, the channels its note-ons use and whether
// it sends a program change on channel 10.
func midiChannels(t *testing.T, b []byte) (chans []map[byte]bool, drumProgram []bool) {
	t.Helper()
	be := binary.BigEndian
	pos := 14
	for pos < len(b) {
		end := pos + 8 + int(be.Uint32(b[pos+4:pos+8]))
		ch, prog := map[byte]bool{}, false
		p := pos + 8
		varlen := func() {
			for b[p]&0x80 != 0 {
				p++
			}
			p++
		}
		for p < end {
			varlen()
			st := b[p]
			p++
			switch {
			case st == 0xFF:
				p++
				n := int(b[p])
				p += 1 + n
			case st&0xF0 == 0x90:
				ch[st&0x0F] = true
				p += 2
			case st&0xF0 == 0xC0:
				prog = prog || st&0x0F == 9
				p++
			default:
				p += 2
			}
		}
		chans, drumProgram = append(chans, ch), append(drumProgram, prog)
		pos = end
	}
	return chans, drumProgram
}

func TestSixteenParts(t *testing.T) {
	s := sixteenParts()
	st, acc, bpm, err := BuildStyle(s)
	if err != nil {
		t.Fatal(err)
	}
	n := NotesFor(st)
	rng := NewRand(rand.New(rand.NewSource(3)))
	first := -1.0
	g, best, _ := Evolve(rng, Options{Notes: n, Acc: acc, Style: st, Population: 24, Generations: 400, Target: 101,
		Progress: func(gen int, c Composition) {
			if first < 0 {
				first = c.Total
			}
		}})
	t.Logf("%.1f -> %.1f\n%s", first, best.Total, best)
	if len(best.Extras) != 12 || best.Total <= first {
		t.Fatalf("extras scored %d, total %.1f -> %.1f", len(best.Extras), first, best.Total)
	}
	_, a := EvaluateGenome(g, n, acc, st)
	if len(a.Parts) != 16 {
		t.Fatalf("%d parts", len(a.Parts))
	}
	chans, drumProg := midiChannels(t, MIDI(a, bpm, "16"))
	used := map[byte]int{}
	for i, p := range a.Parts {
		for ch := range chans[i+1] { // track 0 is the conductor
			if st.IsPerc(p.Kind) != (ch == 9) {
				t.Errorf("%s plays on channel %d", p.Name, ch+1)
			}
			if ch != 9 {
				used[ch]++
			}
		}
		if p.Kind.IsExtra() && st.IsPerc(p.Kind) && drumProg[i+1] {
			t.Errorf("%s resets the drum kit", p.Name)
		}
	}
	for ch, k := range used {
		if k > 1 {
			t.Errorf("channel %d shared by %d pitched parts", ch+1, k)
		}
	}
	if len(used) != 12 { // melody, chords, bass + 9 pitched extras (3 extras are percussion)
		t.Errorf("%d pitched channels in use, want 12", len(used))
	}
	xml := string(MusicXML(a, bpm, "16"))
	if strings.Count(xml, "<score-part ") != 16 || !strings.Contains(xml, "Mute Hi Conga") || !strings.Contains(xml, "Counter-melody 3") {
		t.Error("MusicXML lacks the extra parts")
	}
	if path := os.Getenv("MUSICXML_OUT"); path != "" {
		os.WriteFile(path, []byte(xml), 0o644)
	}

	// a variation locking extra 2 (arpeggio) keeps it exactly
	child, _, _ := Evolve(NewRand(rand.New(rand.NewSource(9))), Options{Notes: n, Acc: acc, Style: st, Population: 12,
		Generations: 100, Target: 101, Seed: g, Lock: LockOf(ExtraKind(1)), Amount: -1})
	for b := 0; b < st.Bars; b++ {
		if !bytes.Equal(extraGenes(g, n, st, 1, b), extraGenes(child, n, st, 1, b)) {
			t.Fatalf("locked extra changed in bar %d", b+1)
		}
	}
	if bytes.Equal(extraGenes(g, n, st, 0, 0), extraGenes(child, n, st, 0, 0)) && bytes.Equal(extraGenes(g, n, st, 0, 1), extraGenes(child, n, st, 0, 1)) {
		t.Error("unlocked extra did not change")
	}

	s.Extras = append(s.Extras, ExtraPart{"pad", 1})
	if _, _, _, err := BuildStyle(s); err == nil {
		t.Error("13 extras accepted")
	}
	for _, bad := range []ExtraPart{{"kazoo", 1}, {"percussion", 20}, {"counter", 200}} {
		s.Extras = []ExtraPart{bad}
		if _, _, _, err := BuildStyle(s); err == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
}
