package melody

import (
	"fmt"
	"math/bits"
	"strings"
)

// An Arrangement is the full composition as timed note events, decoded
// entirely from a genome of random bytes: the melody section gives the lead
// line, and per-bar genes give the chords, bass line and drum pattern. It is
// the single source for the MIDI and MusicXML outputs, so both always agree.
//
// Genome layout:
//
//	[melody: 3 bytes x notes][bar 0: BarGenes bytes][bar 1] ... [bar MaxBars-1]
//
// Per-bar genes:
//
//	0       chord: b % len(style.ChordDegrees) picks a degree of the melody's key
//	1       chord rhythm: b >> 6 picks one of the style's four ChordRhythms
//	2..9    bass, one byte per eighth note: b >> 6 is 0 rest, 1 hold, 2-3 note;
//	        (b & 63) % 6 picks root, third, fifth, octave, step above, step below
//	10..11  kick: 16-bit mask, bit i = hit on sixteenth i
//	12..13  snare mask
//	14..15  hi-hat (or ride) mask
//	16      crash on beat 1 when < CrashBelow
const (
	BarGenes   = 17
	CrashBelow = 32
)

// MaxBars is how many bars of genes a genome carries: enough for the longest
// melody of n notes in the style (every note its longest length).
func MaxBars(notes int, st *Style) int {
	if st.Bars > 0 {
		return st.Bars
	}
	return (notes*st.maxDuration() + st.BarSteps() - 1) / st.BarSteps()
}

// GenomeSize is the genome length in bytes for a melody of n notes.
func GenomeSize(notes int, st *Style) int {
	return notes*3 + MaxBars(notes, st)*(BarGenes+len(st.Extras)*ExtraGenes)
}

// barGenes returns the genes of bar b.
func barGenes(genome []byte, notes, b int) []byte {
	off := notes*3 + b*BarGenes
	return genome[off : off+BarGenes]
}

// Accompaniment selects the parts played under the melody.
type Accompaniment int

const (
	AccompNone   Accompaniment = 0
	AccompBass   Accompaniment = 1 << 0
	AccompChords Accompaniment = 1 << 1
	AccompDrums  Accompaniment = 1 << 2
	AccompAll                  = AccompBass | AccompChords | AccompDrums
)

// ParseAccompaniment parses "all", "none", or a comma-separated list of
// "bass", "chords" and "drums" ("both" means bass,chords).
func ParseAccompaniment(spec string) (Accompaniment, error) {
	switch spec {
	case "all":
		return AccompAll, nil
	case "none", "":
		return AccompNone, nil
	case "both":
		return AccompBass | AccompChords, nil
	}
	var acc Accompaniment
	for _, part := range strings.Split(spec, ",") {
		switch strings.TrimSpace(part) {
		case "bass":
			acc |= AccompBass
		case "chords":
			acc |= AccompChords
		case "drums":
			acc |= AccompDrums
		default:
			return 0, fmt.Errorf("unknown accompaniment %q (want all, none, or a list of bass,chords,drums)", part)
		}
	}
	return acc, nil
}

// PartKind identifies a part of the arrangement.
type PartKind int

const (
	PartLead PartKind = iota
	PartChords
	PartBass
	PartDrums
)

// Event is one note. Times are in sixteenth notes from the start.
type Event struct {
	Start, Steps int
	Pitch        int // MIDI note; for drums a GM percussion key
	Velocity     int // 0..255
	Pan          int // 0..MaxPan
}

// Part is one instrument's line. Events are sorted by Start.
type Part struct {
	Kind   PartKind
	Name   string
	Events []Event
}

// Arrangement is the whole piece.
type Arrangement struct {
	Parts []Part
	Steps int // length in sixteenths (whole bars when accompanied)
	Bars  int
	Tonic int
	Scale *Scale
	// BarTonic, when set, is the key note of every bar (after a key change
	// made by a partly locked transposition).
	BarTonic []int
	Style    *Style
	Notes    []Note  // the decoded melody
	Chords   []Chord // one per bar; nil without accompaniment
}

// Part returns the part of the given kind, or nil.
func (a *Arrangement) Part(k PartKind) *Part {
	for i := range a.Parts {
		if a.Parts[i].Kind == k {
			return &a.Parts[i]
		}
	}
	return nil
}

// Output is the arrangement as it is written out (MIDI, MusicXML,
// playback): transposed as the style says, with its transpose lock.
func (a Arrangement) Output() Arrangement {
	st := a.Style
	return a.TransposedLocked(st.Transpose, st.TransposeLock, st.TransposeFrom, st.TransposeTo)
}

// Transposed shifts every pitched part by semis semitones.
func (a Arrangement) Transposed(semis int) Arrangement { return a.TransposedLocked(semis, 0, 0, 0) }

// TransposedLocked returns a copy shifted by semis semitones, except for the
// locked parts in bars from..to (0-based, to exclusive; to 0 = whole song).
// The key follows the harmony (chords, else bass, else melody) bar by bar,
// so a locked first section and a transposed second one is a key change
// (BarTonic). Drums never move. The scored melody (Notes) is untouched, so
// transposing never changes a score.
func (a Arrangement) TransposedLocked(semis int, lock Lock, from, to int) Arrangement {
	if semis == 0 {
		return a
	}
	if to <= 0 || to > a.Bars {
		to = a.Bars
	}
	shift := func(k PartKind, bar int) int {
		if a.Style.IsPerc(k) || lock&LockOf(k) != 0 && bar >= from && bar < to {
			return 0
		}
		return semis
	}
	ref := PartLead // the part that carries the harmony decides the key
	if a.Part(PartChords) != nil {
		ref = PartChords
	} else if a.Part(PartBass) != nil {
		ref = PartBass
	}
	bar := make([]int, a.Bars)
	changes := false
	for b := range bar {
		bar[b] = shift(ref, b)
		changes = changes || bar[b] != bar[0]
	}
	pc := func(p, d int) int { return ((p+d)%12 + 12) % 12 }

	t := a
	t.Tonic = pc(a.Tonic, bar[0])
	t.BarTonic = nil
	if changes {
		t.BarTonic = make([]int, a.Bars)
		for b, d := range bar {
			t.BarTonic[b] = pc(a.Tonic, d)
		}
	}
	t.Parts = make([]Part, len(a.Parts))
	for i, p := range a.Parts {
		np := Part{Kind: p.Kind, Name: p.Name, Events: append([]Event(nil), p.Events...)}
		for k := range np.Events {
			e := &np.Events[k]
			e.Pitch = max(0, min(127, e.Pitch+shift(p.Kind, e.Start/a.Style.BarSteps())))
		}
		t.Parts[i] = np
	}
	if a.Chords != nil {
		t.Chords = make([]Chord, len(a.Chords))
		for b, c := range a.Chords {
			nc := Chord{Degree: c.Degree, Tones: make([]int, len(c.Tones))}
			for k, tone := range c.Tones {
				nc.Tones[k] = pc(tone, bar[min(b, len(bar)-1)])
			}
			nc.Root, nc.Name = nc.Tones[0], chordName(nc.Tones)
			t.Chords[b] = nc
		}
	}
	return t
}

// TonicAt is the key note in bar b (it differs from Tonic after a key change).
func (a *Arrangement) TonicAt(b int) int {
	if b < len(a.BarTonic) {
		return a.BarTonic[b]
	}
	return a.Tonic
}

// KeyName names the arrangement's key.
func (a *Arrangement) KeyName() string { return KeyName(a.Tonic, a.Scale) }

// Arrange decodes a genome for a melody of n notes into an arrangement in
// the given style, with the parts selected by acc (limited to the parts the
// style uses).
func Arrange(genome []byte, n int, acc Accompaniment, st *Style) Arrangement {
	acc &= st.Acc
	a := Arrangement{Style: st}
	a.Notes = Decode(genome[:n*3], st)
	a.Tonic, a.Scale = DetectKey(a.Notes, st)

	lead := Part{Kind: PartLead, Name: "Melody", Events: make([]Event, 0, n)}
	for _, nt := range a.Notes {
		if nt.Pitch != Rest {
			lead.Events = append(lead.Events, Event{a.Steps, nt.Steps, nt.Pitch, nt.Velocity, nt.Pan})
		}
		a.Steps += nt.Steps
	}
	a.Bars = max(1, (a.Steps+a.Style.BarSteps()-1)/a.Style.BarSteps())
	if st.Bars > 0 {
		a.Bars = st.Bars
		a.Steps = a.Bars * a.Style.BarSteps()
	}
	a.Parts = append(a.Parts, lead)
	if acc == AccompNone && len(st.Extras) == 0 {
		return a
	}
	a.Steps = a.Bars * a.Style.BarSteps() // finish the last bar

	a.Chords = make([]Chord, a.Bars)
	for b := range a.Chords {
		a.Chords[b] = styleChord(barGenes(genome, n, b)[0], a.Tonic, a.Scale, st)
	}
	if acc&AccompChords != 0 {
		a.Parts = append(a.Parts, a.chordPart(genome, n))
	}
	if acc&AccompBass != 0 {
		a.Parts = append(a.Parts, a.bassPart(genome, n))
	}
	if acc&AccompDrums != 0 {
		a.Parts = append(a.Parts, a.drumPart(genome, n))
	}
	for i := range st.Extras {
		a.Parts = append(a.Parts, a.extraPart(genome, n, i))
	}
	return a
}

// Chord voicing limits: the chord's top note stays at least a minor third
// under the lowest melody note of the bar, between chordTopMin and
// chordTopMax (so a low melody squeezes the chord down only so far).
const (
	chordTopMax = 67 // G4
	chordTopMin = 52 // E3
)

// chordPart plays each bar's chord in one of the style's chord rhythms,
// voiced closely below the melody. A chord tone that the melody is sounding
// at the attack is left out, so melody and chords never double each other.
func (a *Arrangement) chordPart(genome []byte, n int) Part {
	p := Part{Kind: PartChords, Name: "Chords", Events: make([]Event, 0, a.Bars*6)}
	mel, _ := melodyTimeline(a)
	prevLow := chordTopMax + 3
	for b, c := range a.Chords {
		low := 1000
		for _, m := range mel[b*a.Style.BarSteps() : (b+1)*a.Style.BarSteps()] {
			if m != Rest {
				low = min(low, m)
			}
		}
		if low == 1000 {
			low = prevLow // a silent bar keeps the previous voicing
		}
		prevLow = low
		top := max(chordTopMin, min(chordTopMax, low-3))

		rhythm := a.Style.ChordRhythms[barGenes(genome, n, b)[1]>>6]
		for k, r := range rhythm {
			vel := a.Style.ChordVelocity
			if k > 0 {
				vel = vel * 7 / 8 // later hits in the bar a little softer
			}
			at := b*a.Style.BarSteps() + r[0]
			doubled := -1
			if m := mel[at]; m != Rest && inChord(m%12, c) {
				doubled = m % 12
			}
			for _, pc := range c.Tones {
				if pc == doubled {
					continue
				}
				pitch := top - (top-pc+120)%12 // highest pitch of this class <= top
				p.Events = append(p.Events, Event{at, r[1], pitch, vel, MaxPan / 2})
			}
		}
	}
	return p
}

// bassPitch returns the bass note for a tone choice over chord c:
// root, third, fifth, octave, scale step above or below the root.
func (a *Arrangement) bassPitch(c Chord, tone int) int {
	root := 36 + c.Root // C2..B2
	up := func(pc int) int { return root + (pc-c.Root+12)%12 }
	st := a.Scale.chordSteps(c.Degree)
	switch tone {
	case 1:
		return up(c.Tones[1])
	case 2:
		return up(c.Tones[2])
	case 3:
		return root + 12
	case 4:
		return up((a.Tonic + st[(c.Degree+1)%7]) % 12)
	case 5:
		return up((a.Tonic+st[(c.Degree+6)%7])%12) - 12
	}
	return root
}

// bassPart decodes eight eighth-note slots per bar; "hold" slots extend the
// previous note when it is still sounding.
func (a *Arrangement) bassPart(genome []byte, n int) Part {
	p := Part{Kind: PartBass, Name: "Bass", Events: make([]Event, 0, a.Bars*8)}
	for b, c := range a.Chords {
		g := barGenes(genome, n, b)[2 : 2+a.Style.BarSteps()/2] // one slot per eighth
		for slot, v := range g {
			start := b*a.Style.BarSteps() + slot*2
			switch v >> 6 {
			case 0: // rest
			case 1: // hold
				if k := len(p.Events) - 1; k >= 0 && p.Events[k].Start+p.Events[k].Steps == start {
					p.Events[k].Steps += 2
				}
			default:
				vel := 170
				if slot%2 == 0 {
					vel = 200 // on the beat
				}
				p.Events = append(p.Events, Event{start, 2, a.bassPitch(c, int(v&63)%6), vel, MaxPan / 2})
			}
		}
	}
	return p
}

// drumMasks returns the kick, snare and hat hit masks and crash flag of bar b.
// Only the first bs bits of each mask are used (bs sixteenths per bar).
func drumMasks(genome []byte, n, b, bs int) (kick, snare, hat uint16, crash bool) {
	g := barGenes(genome, n, b)
	m := uint16(1<<bs - 1)
	kick = (uint16(g[10]) | uint16(g[11])<<8) & m
	snare = (uint16(g[12]) | uint16(g[13])<<8) & m
	hat = (uint16(g[14]) | uint16(g[15])<<8) & m
	return kick, snare, hat, g[16] < CrashBelow
}

// drumPart decodes the per-bar hit masks. Velocities follow the metre:
// strongest on beat 1, then beats, then off-beats.
func (a *Arrangement) drumPart(genome []byte, n int) Part {
	hits := 0
	for b := 0; b < a.Bars; b++ {
		k, s, h, _ := drumMasks(genome, n, b, a.Style.BarSteps())
		hits += bits.OnesCount16(k) + bits.OnesCount16(s) + bits.OnesCount16(h) + 1
	}
	p := Part{Kind: PartDrums, Name: "Drums", Events: make([]Event, 0, hits)}
	vol := a.Style.DrumVolume
	if vol == 0 {
		vol = 100
	}
	accent := func(step, strong, beat, off int) int {
		v := off
		switch {
		case step == 0:
			v = strong
		case step%a.Style.BeatSteps() == 0:
			v = beat
		}
		return v * vol / 100
	}
	k := a.Style.DrumKeys
	for b := 0; b < a.Bars; b++ {
		kick, snare, hat, crash := drumMasks(genome, n, b, a.Style.BarSteps())
		for step := 0; step < a.Style.BarSteps(); step++ {
			at := b*a.Style.BarSteps() + step
			bit := uint16(1) << step
			if crash && step == 0 {
				p.Events = append(p.Events, Event{at, 1, k.Crash, 190 * vol / 100, MaxPan / 2})
			} else if hat&bit != 0 {
				p.Events = append(p.Events, Event{at, 1, k.Hat, accent(step, 150, 140, 105), MaxPan / 2})
			}
			if snare&bit != 0 {
				p.Events = append(p.Events, Event{at, 1, k.Snare, accent(step, 200, 200, 140), MaxPan / 2})
			}
			if kick&bit != 0 {
				p.Events = append(p.Events, Event{at, 1, k.Kick, accent(step, 230, 210, 170), MaxPan / 2})
			}
		}
	}
	return p
}
