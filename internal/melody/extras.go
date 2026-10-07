package melody

import (
	"fmt"
	"math/bits"
	"strconv"
	"strings"
)

// Extra parts: up to MaxExtras additional instruments on top of melody,
// chords, bass and drums. Each has a role that decides how its genes are
// read and what the detector rewards. Like everything else, their notes
// come from /dev/urandom; the role only shapes the vocabulary (chord tones,
// scale notes, hit masks) and the scoring.
//
// Genome layout: after the bar genes, every extra part has its own block of
// ExtraGenes bytes per bar:
//
//	[melody][bars x BarGenes][extra 1: bars x ExtraGenes][extra 2: ...]

const (
	MaxExtras  = 12
	ExtraGenes = 16
	// PartExtra is the PartKind of the first extra part; extra k is
	// PartExtra+k.
	PartExtra PartKind = 4
)

// Role is what an extra part plays.
type Role int

const (
	RoleCounter    Role = iota // a second line under the melody, scale notes
	RoleArpeggio               // broken chords in sixteenths
	RolePad                    // sustained chord tones
	RoleStabs                  // short accented chords
	RolePercussion             // one extra drum sound
)

var roleIDs = []string{"counter", "arpeggio", "pad", "stabs", "percussion"}
var roleNames = []string{"Counter-melody", "Arpeggio", "Pad", "Stabs", "Percussion"}

// RoleByID finds a role.
func RoleByID(id string) (Role, error) {
	for i, r := range roleIDs {
		if r == id {
			return Role(i), nil
		}
	}
	return 0, fmt.Errorf("unknown role %q (want %s)", id, strings.Join(roleIDs, ", "))
}

func (r Role) String() string { return roleIDs[r] }

// Name is the role's display name.
func (r Role) Name() string { return roleNames[r] }

// RoleInfo lists roles for the UI.
func RoleInfo() []map[string]string {
	var out []map[string]string
	for i := range roleIDs {
		out = append(out, map[string]string{"id": roleIDs[i], "name": roleNames[i]})
	}
	return out
}

// extraSpec is an extra part in a Style.
type extraSpec struct {
	Role Role
	Key  int // GM percussion key (percussion role)
}

// extraVelocity is each role's base loudness (0..255): accompaniment stays
// under the melody.
var extraVelocity = []int{150, 110, 80, 120, 150}

// melodicChannels are the MIDI channels extra pitched parts use, in order
// (0-2 are melody, chords and bass; 9 is drums).
var melodicChannels = []byte{3, 4, 5, 6, 7, 8, 10, 11, 12, 13, 14, 15}

// IsExtra reports whether a part kind is an extra part.
func (k PartKind) IsExtra() bool { return k >= PartExtra }

// ExtraKind is the PartKind of extra part i (0-based).
func ExtraKind(i int) PartKind { return PartExtra + PartKind(i) }

// PartKey names a part kind: lead, chords, bass, drums, extra1..extra12.
func PartKey(k PartKind) string {
	switch k {
	case PartLead:
		return "lead"
	case PartChords:
		return "chords"
	case PartBass:
		return "bass"
	case PartDrums:
		return "drums"
	}
	return "extra" + strconv.Itoa(int(k-PartExtra)+1)
}

// KindOf parses a part key; "melody" is accepted for the lead.
func KindOf(key string) (PartKind, bool) {
	switch key {
	case "lead", "melody":
		return PartLead, true
	case "chords":
		return PartChords, true
	case "bass":
		return PartBass, true
	case "drums":
		return PartDrums, true
	}
	if n, err := strconv.Atoi(strings.TrimPrefix(key, "extra")); err == nil && strings.HasPrefix(key, "extra") && n >= 1 && n <= MaxExtras {
		return ExtraKind(n - 1), true
	}
	return 0, false
}

// LockOf is the lock bit protecting a part.
func LockOf(k PartKind) Lock {
	switch k {
	case PartLead:
		return LockMelody
	case PartChords:
		return LockChords
	case PartBass:
		return LockBass
	case PartDrums:
		return LockDrums
	}
	return 1 << (4 + uint(k-PartExtra))
}

// IsPerc reports whether a part is percussion (drums or a percussion extra).
func (s *Style) IsPerc(k PartKind) bool {
	if k == PartDrums {
		return true
	}
	if i := int(k - PartExtra); k.IsExtra() && i < len(s.Extras) {
		return s.Extras[i].Role == RolePercussion
	}
	return false
}

// extraGenes returns the genes of extra part i in bar b.
func extraGenes(genome []byte, notes int, st *Style, i, b int) []byte {
	bars := MaxBars(notes, st)
	off := notes*3 + bars*BarGenes + (i*bars+b)*ExtraGenes
	return genome[off : off+ExtraGenes]
}

// extraName is the display name of extra part i.
func (s *Style) extraName(i int) string {
	e := s.Extras[i]
	n := 0
	for j := 0; j <= i; j++ {
		if s.Extras[j].Role == e.Role {
			n++
		}
	}
	return fmt.Sprintf("%s %d", e.Role.Name(), n)
}

// scalePitches lists the notes of the key's scale (for the chord's degree)
// from lo up to hi.
func (a *Arrangement) scalePitches(c Chord, lo, hi int) []int {
	st := a.Scale.chordSteps(c.Degree)
	var ps []int
	for p := lo; p <= hi; p++ {
		for _, s := range st {
			if (p-a.Tonic-s)%12 == 0 {
				ps = append(ps, p)
				break
			}
		}
	}
	return ps
}

// chordPitches lists the chord's tones from lo up to hi.
func chordPitches(c Chord, lo, hi int) []int {
	var ps []int
	for p := lo; p <= hi; p++ {
		if inChord(p%12, c) {
			ps = append(ps, p)
		}
	}
	return ps
}

// extraPart decodes extra part i.
func (a *Arrangement) extraPart(genome []byte, n, i int) Part {
	st := a.Style
	spec := st.Extras[i]
	p := Part{Kind: ExtraKind(i), Name: st.extraName(i), Events: make([]Event, 0, a.Bars*8)}
	vel := extraVelocity[spec.Role]
	for b := 0; b < a.Bars; b++ {
		g := extraGenes(genome, n, st, i, b)
		c := a.Chords[b]
		at := b * a.Style.BarSteps()
		switch spec.Role {
		case RoleCounter: // eighth slots: rest, hold or a scale note G3..G5
			ps := a.scalePitches(c, 55, 79)
			for s, v := range g[:a.Style.BarSteps()/2] {
				start := at + s*2
				switch v >> 6 {
				case 0:
				case 1:
					if k := len(p.Events) - 1; k >= 0 && p.Events[k].Start+p.Events[k].Steps == start {
						p.Events[k].Steps += 2
					}
				default:
					p.Events = append(p.Events, Event{start, 2, ps[int(v&63)%len(ps)], vel, MaxPan/2 + 6})
				}
			}
		case RoleArpeggio: // sixteenth slots: rest or a chord tone E3..E5
			ps := chordPitches(c, 52, 76)
			for s, v := range g[:a.Style.BarSteps()] {
				if v >= 96 {
					p.Events = append(p.Events, Event{at + s, 1, ps[int(v)%len(ps)], vel, MaxPan/2 - 6})
				}
			}
		case RolePad: // held chord tones in an open voicing
			rhythm := [][2]int{{0, 16}} // whole bar, or two halves for one gene value in four
			if g[0]>>6 == 2 {
				rhythm = [][2]int{{0, 8}, {8, 8}}
			}
			lo := 48 + int(g[1]%6) // voicing position
			ps := chordPitches(c, lo, lo+18)
			for _, r := range rhythm {
				for k, pitch := range ps {
					if k%2 == 0 || len(ps) <= 4 { // open: every other tone of a dense voicing
						p.Events = append(p.Events, Event{at + r[0], r[1], pitch, vel, MaxPan / 2})
					}
				}
			}
		case RoleStabs: // 16-step hit mask, a three-note chord C4..C5
			mask := uint16(g[0]) | uint16(g[1])<<8
			ps := chordPitches(c, 60, 72)
			if len(ps) > 3 {
				ps = ps[len(ps)-3:]
			}
			for s := 0; s < a.Style.BarSteps(); s++ {
				if mask&(1<<s) != 0 {
					v := vel
					if g[2]&(1<<(s%8)) != 0 {
						v = vel * 5 / 4 // accent
					}
					for _, pitch := range ps {
						p.Events = append(p.Events, Event{at + s, 1, pitch, v, MaxPan / 2})
					}
				}
			}
		case RolePercussion: // 16-step hit mask of one drum sound
			mask := uint16(g[0]) | uint16(g[1])<<8
			for s := 0; s < a.Style.BarSteps(); s++ {
				if mask&(1<<s) != 0 {
					v := vel * 3 / 4
					if g[2]&(1<<(s%8)) != 0 {
						v = vel
					}
					p.Events = append(p.Events, Event{at + s, 1, spec.Key, v * drumVolume(st) / 100, MaxPan / 2})
				}
			}
		}
	}
	return p
}

func drumVolume(st *Style) int {
	if st.DrumVolume == 0 {
		return 100
	}
	return st.DrumVolume
}

// scoreExtras rates every extra part (0..1 each) for its role.
func (c *Composition) scoreExtras(a *Arrangement, genome []byte, n int, kicks []uint16) {
	if len(a.Style.Extras) == 0 {
		return
	}
	mel, onset := melodyTimeline(a)
	barLow := make([]int, a.Bars) // lowest melody note per bar
	for b := range barLow {
		barLow[b] = 1000
		for _, m := range mel[b*a.Style.BarSteps() : (b+1)*a.Style.BarSteps()] {
			if m != Rest {
				barLow[b] = min(barLow[b], m)
			}
		}
	}
	below := func(e Event) float64 { // under the melody (or no melody there)
		if l := barLow[e.Start/a.Style.BarSteps()]; l == 1000 || e.Pitch < l {
			return 1
		}
		return 0
	}
	repeats := func(i int) float64 { // bars repeating the pattern 1 or 2 bars before
		if a.Bars < 3 {
			return 1
		}
		r := 0.0
		for b := 2; b < a.Bars; b++ {
			g, g1, g2 := extraGenes(genome, n, a.Style, i, b), extraGenes(genome, n, a.Style, i, b-1), extraGenes(genome, n, a.Style, i, b-2)
			if string(g) == string(g1) || string(g) == string(g2) {
				r++
			}
		}
		return clamp01(r / float64(a.Bars-2) / 0.7)
	}
	perBar := func(p *Part, distinctStarts bool) []int {
		cnt := make([]int, a.Bars)
		last := -1
		for _, e := range p.Events {
			if distinctStarts && e.Start == last {
				continue
			}
			last = e.Start
			cnt[e.Start/a.Style.BarSteps()]++
		}
		return cnt
	}
	density := func(cnt []int, lo, hi int) float64 {
		t := 0.0
		for _, k := range cnt {
			t += densityScore(k, lo, hi)
		}
		return t / float64(len(cnt))
	}

	c.Extras = make([]float64, len(a.Style.Extras))
	c.xSub = make([][]sub, len(a.Style.Extras))
	for i, spec := range a.Style.Extras {
		p := a.Part(ExtraKind(i))
		var subs []sub
		switch spec.Role {
		case RoleCounter:
			var cons, under, fills, motion float64
			for k, e := range p.Events {
				m := mel[e.Start]
				switch {
				case m == Rest:
					cons++
				default:
					switch abs(m-e.Pitch) % 12 {
					case 3, 4, 8, 9:
						cons++
					case 5, 7:
						cons += 0.6
					case 0:
						cons += 0.3
					default:
						cons += 0.1
					}
				}
				if m == Rest || e.Pitch < m {
					under++
				}
				if !onset[e.Start] {
					fills++
				}
				if k > 0 {
					switch iv := abs(e.Pitch - p.Events[k-1].Pitch); {
					case iv <= 2:
						motion++
					case iv <= 5:
						motion += 0.8
					default:
						motion += 0.3
					}
				}
			}
			nE := float64(max(1, len(p.Events)))
			subs = []sub{{"with melody", cons / nE}, {"below it", under / nE}, {"fills gaps", fills / nE},
				{"stepwise", motion / float64(max(1, len(p.Events)-1))}, {"density", density(perBar(p, false), 2, 6)}}
			c.Extras[i] = weigh(subs, 0.35, 0.2, 0.15, 0.15, 0.15)
		case RoleArpeggio:
			var under float64
			for _, e := range p.Events {
				under += below(e)
			}
			subs = []sub{{"pattern", repeats(i)}, {"density", density(perBar(p, false), 4, 12)},
				{"below melody", under / float64(max(1, len(p.Events)))}}
			c.Extras[i] = weigh(subs, 0.4, 0.3, 0.3)
		case RolePad:
			var under, long float64
			for _, e := range p.Events {
				under += below(e)
				if e.Steps >= 8 {
					long++
				}
			}
			nE := float64(max(1, len(p.Events)))
			subs = []sub{{"below melody", under / nE}, {"sustained", long / nE}, {"steady", repeats(i)}}
			c.Extras[i] = weigh(subs, 0.5, 0.3, 0.2)
		case RoleStabs:
			var clash float64
			starts := perBar(p, true)
			last := -1
			hits := 0.0
			for _, e := range p.Events {
				if e.Start == last {
					continue
				}
				last = e.Start
				hits++
				if onset[e.Start] {
					clash++
				}
			}
			subs = []sub{{"sparse", density(starts, 1, 4)}, {"pattern", repeats(i)}, {"between melody notes", 1 - clash/max(1, hits)}}
			c.Extras[i] = weigh(subs, 0.3, 0.4, 0.3)
		case RolePercussion:
			var doubled, hits float64
			for b := 0; b < a.Bars; b++ {
				g := extraGenes(genome, n, a.Style, i, b)
				mask := uint16(g[0]) | uint16(g[1])<<8
				hits += float64(bits.OnesCount16(mask))
				if kicks != nil {
					doubled += float64(bits.OnesCount16(mask & kicks[b]))
				}
			}
			subs = []sub{{"groove", repeats(i)}, {"density", density(perBar(p, false), 1, 8)},
				{"own space", 1 - doubled/max(1, hits)}}
			c.Extras[i] = weigh(subs, 0.4, 0.4, 0.2)
		}
		c.xSub[i] = subs
	}
}

// GMPercussion names the General MIDI percussion keys 35..81.
var GMPercussion = map[int]string{
	35: "Acoustic Bass Drum", 36: "Bass Drum 1", 37: "Side Stick", 38: "Acoustic Snare", 39: "Hand Clap",
	40: "Electric Snare", 41: "Low Floor Tom", 42: "Closed Hi-Hat", 43: "High Floor Tom", 44: "Pedal Hi-Hat",
	45: "Low Tom", 46: "Open Hi-Hat", 47: "Low-Mid Tom", 48: "Hi-Mid Tom", 49: "Crash Cymbal 1",
	50: "High Tom", 51: "Ride Cymbal 1", 52: "Chinese Cymbal", 53: "Ride Bell", 54: "Tambourine",
	55: "Splash Cymbal", 56: "Cowbell", 57: "Crash Cymbal 2", 58: "Vibraslap", 59: "Ride Cymbal 2",
	60: "Hi Bongo", 61: "Low Bongo", 62: "Mute Hi Conga", 63: "Open Hi Conga", 64: "Low Conga",
	65: "High Timbale", 66: "Low Timbale", 67: "High Agogo", 68: "Low Agogo", 69: "Cabasa",
	70: "Maracas", 71: "Short Whistle", 72: "Long Whistle", 73: "Short Guiro", 74: "Long Guiro",
	75: "Claves", 76: "Hi Wood Block", 77: "Low Wood Block", 78: "Mute Cuica", 79: "Open Cuica",
	80: "Mute Triangle", 81: "Open Triangle",
}

// PartProgram is what a part's instrument setting holds: the GM program (or
// drum kit), or for a percussion extra the GM percussion key it plays.
func (s *Style) PartProgram(k PartKind) int {
	if k.IsExtra() && s.IsPerc(k) {
		return s.Extras[k-PartExtra].Key
	}
	return int(s.Instruments[k].program)
}

// PartKinds lists every part a style can have: the four main parts and its
// extra parts.
func (s *Style) PartKinds() []PartKind {
	ks := []PartKind{PartLead, PartChords, PartBass, PartDrums}
	for i := range s.Extras {
		ks = append(ks, ExtraKind(i))
	}
	return ks
}

// ExtraRole is the role of extra part k (as its ID), or "" for main parts.
func (s *Style) ExtraRole(k PartKind) string {
	if !k.IsExtra() || int(k-PartExtra) >= len(s.Extras) {
		return ""
	}
	return s.Extras[k-PartExtra].Role.String()
}

// Channel is the MIDI channel (0-based) a part plays on.
func (s *Style) Channel(k PartKind) int { return int(s.Instruments[k].channel) }
