package melody

import (
	"fmt"
	"strings"
)

// A Style steers composition towards a genre without adding any
// non-random material: it sets how genes are decoded (note lengths, chord
// vocabulary and voicing, chord rhythms, drum sounds) and what the detectors
// reward (scale, harmony model, bass role, reference drum grooves, weights).
// Every note still comes from /dev/urandom.
type Style struct {
	ID    byte // stored in genome files
	Name  string
	Title string // display name ("" = Name)
	Group string // genre family for menus
	BPM   int    // default tempo
	// Meter is the time signature {beats, unit}: {4,4}, {3,4} or {6,8};
	// zero means 4/4.
	Meter [2]int
	// Swing is how far off-beat eighths are delayed: 0 straight, 1 a 2:1
	// triplet shuffle (values between give lighter swing).
	Swing float64
	Notes int // default melody length (when Bars is 0)
	// Bars fixes the piece's length (set from a duration in seconds); the
	// melody is cut at that length. 0: the melody's own length decides.
	Bars int
	// Transpose (semitones) applies when the piece is written out, except
	// to TransposeLock parts in bars TransposeFrom..TransposeTo (0-based,
	// To exclusive; To 0 = whole song). See Arrangement.Output.
	Transpose                  int
	TransposeLock              Lock
	TransposeFrom, TransposeTo int
	// SectionBPM, when set, is the tempo of bars SectionFrom..SectionTo
	// (0-based, To exclusive); the rest play at the song's tempo.
	SectionBPM, SectionFrom, SectionTo int
	// Extras are the extra parts (beyond melody, chords, bass and drums);
	// their instruments are in Instruments under ExtraKind(i).
	Extras []extraSpec
	// AltInstruments are played instead of Instruments in bars
	// AltFrom..AltTo (0-based, To exclusive).
	AltInstruments map[PartKind]gmInstrument
	AltFrom, AltTo int
	Acc            Accompaniment

	// melody
	Durations [8]int // G>>5 -> length in sixteenths
	Blues     bool   // blues scale and blues key detection instead of major/minor
	// AutoScale is the genre's own scale when the user leaves scale on auto
	// (e.g. dorian for afrobeat); nil: detect major/minor.
	AutoScale *Scale
	// Transitions rates chord-to-chord moves for this genre (nil: the
	// common-practice table).
	Transitions *[7][7]int
	// Scale forces a scale (nil: detect major/minor from the melody, or
	// blues for blues styles); Tonic forces the key note as pitch class + 1
	// (0: detect); ModeBias limits auto detection (-1 minor, +1 major).
	Scale      *Scale
	Tonic      int
	ModeBias   int
	RestTarget float64 // ideal share of silence
	RangeFull  int     // melodic span (semitones) that still gets full credit
	MotifIdeal float64 // ideal share of repeated 4-note figures
	Grid       int     // onsets should fall on multiples of this many sixteenths

	// harmony
	Harmony       HarmonyKind
	ChordDegrees  []int // chord gene picks one of these scale degrees
	Sevenths      bool  // diatonic seventh chords instead of triads
	Dominant      bool  // every chord a dominant seventh (blues I7 IV7 V7)
	ChordRhythms  [4][][2]int
	ChordVelocity int // 0..255; chords stay in the background

	// bass and drums
	Bass     BassKind
	Grooves  []Groove // reference patterns the drums are compared with
	DrumKeys DrumKeys
	Fills    bool // reward snare fills at the end of 8-bar phrases
	// DrumVolume scales drum velocities in percent (0 means 100).
	DrumVolume int

	Instruments map[PartKind]gmInstrument
	Weights     [4]float64 // melody, harmony, bass, drums
}

// HarmonyKind selects the harmony detector.
type HarmonyKind int

const (
	Functional HarmonyKind = iota // common-practice progressions and cadences
	TwelveBar                     // the 12-bar blues form
	Loop                          // a short progression repeated (dance music)
	Drift                         // slow, smooth changes with common tones
)

// BassKind selects the bass detector.
type BassKind int

const (
	BassWithKick   BassKind = iota // roots on the downbeat, locked to the kick
	BassWalking                    // a note on every beat, stepwise
	BassOffbeat                    // on the "and" of each beat, away from the kick
	BassDrone                      // one or two long notes per bar
	BassSyncopated                 // off-beat, funky lines (funk, reggae, latin, afrobeat)
	BassDownbeat                   // a root on the downbeat, little else (waltz "oom", 6/8 ballads)
)

// Groove is a one-bar drum pattern as 16-step masks (bit i = sixteenth i).
type Groove struct{ Kick, Snare, Hat uint16 }

// DrumKeys are the General MIDI percussion keys a style plays.
type DrumKeys struct{ Kick, Snare, Hat, Crash int }

const (
	beats    = 0x1111 // every quarter
	backbeat = 0x1010 // beats 2 and 4
	eighths  = 0x5555
	offbeats = 0x4444 // the "and" of every beat
)

// Pop chords are mostly held: a sustained pad under the melody.
var chordRhythmsPop = [4][][2]int{
	{{0, 16}},
	{{0, 16}},
	{{0, 8}, {8, 8}},
	{{0, 6}, {6, 10}},
}

var (
	Pop = &Style{
		ID: 0, Name: "pop", BPM: 120, Notes: 256, Acc: AccompAll,
		Durations: [8]int{1, 2, 2, 2, 4, 4, 6, 8}, RestTarget: 0.12, RangeFull: 12, MotifIdeal: 0.7, Grid: 2,
		Harmony: Functional, ChordDegrees: []int{0, 1, 2, 3, 4, 5, 6},
		ChordRhythms: chordRhythmsPop, ChordVelocity: 80,
		Bass: BassWithKick,
		Grooves: []Groove{
			{0x0101, backbeat, eighths},
			{0x0501, backbeat, eighths}, // extra kick on the "and" of 3
			{0x0141, backbeat, eighths}, // syncopated kick before 3
		},
		DrumKeys: DrumKeys{36, 38, 42, 49}, Fills: true,
		Instruments: map[PartKind]gmInstrument{
			PartLead:   {0, 80, "Lead 1 (square)"},
			PartChords: {1, 48, "String Ensemble 1"},
			PartBass:   {2, 33, "Electric Bass (finger)"},
			PartDrums:  {9, 0, "Standard Drum Kit"},
		},
		Weights: [4]float64{0.55, 0.15, 0.15, 0.15},
	}

	Blues = &Style{
		ID: 1, Name: "blues", BPM: 90, Swing: 1, Notes: 192, Acc: AccompAll,
		Durations: [8]int{2, 2, 2, 2, 4, 4, 6, 8}, Blues: true, RestTarget: 0.15, RangeFull: 12, MotifIdeal: 0.7, Grid: 2,
		Harmony: TwelveBar, ChordDegrees: []int{0, 3, 4}, Dominant: true,
		ChordRhythms: [4][][2]int{
			{{0, 16}},
			{{0, 8}, {8, 8}},
			{{0, 3}, {4, 3}, {8, 3}, {12, 3}}, // shuffle comping on the beats
			{{0, 6}, {6, 10}},                 // Charleston
		},
		ChordVelocity: 80,
		Bass:          BassWalking,
		Grooves:       []Groove{{0x0101, backbeat, eighths}, {beats, backbeat, eighths}},
		DrumKeys:      DrumKeys{36, 38, 51, 49}, Fills: true, // shuffle on the ride
		Instruments: map[PartKind]gmInstrument{
			PartLead:   {0, 65, "Alto Sax"},
			PartChords: {1, 17, "Percussive Organ"},
			PartBass:   {2, 32, "Acoustic Bass"},
			PartDrums:  {9, 0, "Standard Drum Kit"},
		},
		Weights: [4]float64{0.5, 0.2, 0.15, 0.15},
	}

	House = &Style{
		ID: 2, Name: "house", BPM: 124, Notes: 384, Acc: AccompAll,
		Durations: [8]int{1, 1, 2, 2, 2, 3, 4, 4}, RestTarget: 0.25, RangeFull: 10, MotifIdeal: 0.9, Grid: 2,
		Harmony: Loop, ChordDegrees: []int{0, 1, 2, 3, 4, 5, 6}, Sevenths: true,
		ChordRhythms: [4][][2]int{
			{{2, 2}, {6, 2}, {10, 2}, {14, 2}}, // offbeat stabs
			{{2, 1}, {6, 1}, {10, 1}, {14, 1}}, // short offbeat stabs
			{{0, 2}, {3, 2}, {6, 2}, {10, 2}},  // syncopated stabs
			{{0, 16}},                          // pad
		},
		ChordVelocity: 100, // stabs are part of the house sound
		Bass:          BassOffbeat,
		Grooves:       []Groove{{beats, backbeat, offbeats}, {beats, 0, offbeats}},
		DrumKeys:      DrumKeys{36, 39, 46, 49}, // clap, open hat
		Instruments: map[PartKind]gmInstrument{
			PartLead:   {0, 81, "Lead 2 (sawtooth)"},
			PartChords: {1, 0, "Acoustic Grand Piano"},
			PartBass:   {2, 38, "Synth Bass 1"},
			PartDrums:  {9, 0, "Standard Drum Kit"},
		},
		Weights: [4]float64{0.4, 0.2, 0.2, 0.2},
	}

	Ambient = &Style{
		ID: 3, Name: "ambient", BPM: 70, Notes: 96, Acc: AccompAll,
		Durations: [8]int{4, 6, 8, 8, 12, 16, 16, 16}, RestTarget: 0.25, RangeFull: 12, MotifIdeal: 0.5, Grid: 4,
		Harmony: Drift, ChordDegrees: []int{0, 1, 2, 3, 4, 5, 6}, Sevenths: true,
		ChordRhythms: [4][][2]int{{{0, 16}}, {{0, 16}}, {{0, 16}}, {{0, 8}, {8, 8}}}, ChordVelocity: 80,
		Bass: BassDrone,
		// sparse half-time electronic kit: kick, rim click on 3, offbeat shaker
		Grooves: []Groove{
			{0x0001, 0x0100, offbeats},
			{0x0401, 0x0100, offbeats}, // kick pickup before 3
			{0x0001, 0, eighths},       // just kick and shaker
		},
		DrumKeys:   DrumKeys{36, 37, 82, 59}, // kick, side stick, shaker, ride wash
		DrumVolume: 55,
		Instruments: map[PartKind]gmInstrument{
			PartLead:   {0, 85, "Lead 6 (voice)"},
			PartChords: {1, 94, "Pad 7 (halo)"},
			PartBass:   {2, 39, "Synth Bass 2"},
			PartDrums:  {9, 25, "TR-808 Kit"}, // GM2/GS kit select on channel 10
		},
		Weights: [4]float64{0.5, 0.15, 0.15, 0.2},
	}

	// Styles lists all styles, indexed by ID.
	Styles = []*Style{Pop, Blues, House, Ambient}
)

// StyleByName finds a style.
func StyleByName(name string) (*Style, error) {
	for _, s := range Styles {
		if s.Name == name {
			return s, nil
		}
	}
	var names []string
	for _, s := range Styles {
		names = append(names, s.Name)
	}
	return nil, fmt.Errorf("unknown style %q (want %s)", name, strings.Join(names, ", "))
}

// StyleByID finds a style by its genome-file ID.
func StyleByID(id byte) (*Style, error) {
	if int(id) < len(Styles) {
		return Styles[id], nil
	}
	return nil, fmt.Errorf("unknown style id %d", id)
}

// maxDuration is the longest note a style can decode.
func (s *Style) maxDuration() int {
	m := 0
	for _, d := range s.Durations {
		m = max(m, d)
	}
	return m
}

// BarBPM is the tempo of bar b in a song whose tempo is base.
func (s *Style) BarBPM(b, base int) int {
	if s.SectionBPM > 0 && b >= s.SectionFrom && b < s.SectionTo {
		return s.SectionBPM
	}
	return base
}

// Timeline converts sixteenth-note positions to seconds, following the
// per-bar tempo and swing.
type Timeline struct {
	starts              []float64 // start of every bar in seconds, plus the end
	bpm                 []int
	swing               float64
	barSteps, beatSteps int
}

// NewTimeline builds the timeline of a song of the given bars at base tempo.
func (s *Style) NewTimeline(bars, base int) Timeline {
	t := Timeline{starts: make([]float64, bars+1), bpm: make([]int, bars), swing: s.Swing, barSteps: s.BarSteps(), beatSteps: s.BeatSteps()}
	for b := 0; b < bars; b++ {
		t.bpm[b] = s.BarBPM(b, base)
		t.starts[b+1] = t.starts[b] + float64(t.barSteps)*60/(float64(t.bpm[b])*float64(s.BeatSteps()))
	}
	return t
}

// At is the time in seconds of a position in sixteenths.
func (t Timeline) At(step int) float64 {
	b := step / t.barSteps
	if b >= len(t.bpm) {
		return t.starts[len(t.starts)-1]
	}
	within := StepTick(step, t.swing) - StepTick(b*t.barSteps, t.swing)
	quarterBPM := float64(t.bpm[b]) * float64(t.beatSteps) / 4
	return t.starts[b] + float64(within)*60/quarterBPM/TicksPerQuarter
}

// BarStarts are the bar start times in seconds, plus the end of the song.
func (t Timeline) BarStarts() []float64 { return t.starts }

// InstrumentAt is the instrument a part plays in bar b, and whether it is
// the alternative one of an instrument lock.
func (s *Style) InstrumentAt(k PartKind, b int) (gmInstrument, bool) {
	if alt, ok := s.AltInstruments[k]; ok && b >= s.AltFrom && b < s.AltTo {
		return alt, true
	}
	return s.Instruments[k], false
}

// HasAlt reports whether a part switches instrument somewhere.
func (s *Style) HasAlt(k PartKind) bool {
	_, ok := s.AltInstruments[k]
	return ok
}

// InstrumentAtBar is InstrumentAt as program and name.
func (s *Style) InstrumentAtBar(k PartKind, b int) (program int, name string, alt bool) {
	ins, alt := s.InstrumentAt(k, b)
	return int(ins.program), ins.name, alt
}

// meter returns the time signature (4/4 when unset).
func (s *Style) meter() (beats, unit int) {
	if s.Meter[0] == 0 {
		return 4, 4
	}
	return s.Meter[0], s.Meter[1]
}

// BarSteps is the bar length in sixteenths: 16 in 4/4, 12 in 3/4 and 6/8.
func (s *Style) BarSteps() int {
	b, u := s.meter()
	return b * 16 / u
}

// BeatSteps is the beat length in sixteenths: a quarter (4), or a dotted
// quarter (6) in compound meters like 6/8. Tempos count these beats.
func (s *Style) BeatSteps() int {
	b, u := s.meter()
	if u == 8 && b%3 == 0 {
		return 6
	}
	return 16 / u
}

// Strong tells how strong a position in the bar is: 2 for the downbeat, 1 for
// the bar's other strong beat (3 in 4/4, 4 in 6/8), 0 otherwise.
func (s *Style) Strong(step int) int {
	bs, beat := s.BarSteps(), s.BeatSteps()
	switch pos := step % bs; {
	case pos == 0:
		return 2
	case bs/beat%2 == 0 && pos == bs/2:
		return 1
	}
	return 0
}

// TimeSignature is the meter as text, e.g. "6/8".
func (s *Style) TimeSignature() string {
	b, u := s.meter()
	return fmt.Sprintf("%d/%d", b, u)
}

// QuarterBPM converts a tempo in beats to quarter notes per minute (MIDI and
// MusicXML count quarters; 6/8 counts dotted quarters).
func (s *Style) QuarterBPM(bpm float64) float64 { return bpm * float64(s.BeatSteps()) / 4 }
