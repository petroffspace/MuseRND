package melody

import (
	"fmt"
	"math"
)

// Settings are the user's choices for one composition (web UI and genome
// files). They are applied to a genre preset by BuildStyle.
type Settings struct {
	Genre string `json:"genre"` // pop, blues, house, ambient
	Mood  string `json:"mood"`  // see Moods (a preset, or "custom")
	// Sliders shape the mood; when set they are what counts (Mood is only
	// the preset's name). Settings saved before sliders existed have none
	// and keep their original legacy mood behaviour.
	Sliders     *MoodSliders   `json:"sliders,omitempty"`
	Scale       string         `json:"scale"`             // a Scales ID, or "auto"
	Tonic       int            `json:"tonic"`             // pitch class 0..11, or -1 for auto
	Seconds     int            `json:"seconds"`           // target length
	BPM         int            `json:"bpm"`               // tempo; 0 = genre default adjusted by mood
	Parts       []string       `json:"parts"`             // any of chords, bass, drums
	Instruments map[string]int `json:"instruments"`       // part -> GM program (drums: kit program)
	Quality     string         `json:"quality,omitempty"` // quick, normal, best (search budget)
	// Bars, when set, fixes the length in bars (overrides Seconds); used
	// when the tempo of an existing song changes, so its genome still fits.
	Bars int `json:"bars,omitempty"`
	// Transpose shifts every pitched part by semitones when the song is
	// written out (MIDI, MusicXML, playback); scoring is unaffected.
	Transpose int `json:"transpose,omitempty"`
	// TransposeLock keeps some parts at their original pitch, optionally
	// only in a range of bars (e.g. a key change for the last section).
	TransposeLock *TransposeLock `json:"transposeLock,omitempty"`
	// TempoSection gives a range of bars a tempo of its own.
	TempoSection *TempoSection `json:"tempoSection,omitempty"`
	// InstrumentLock keeps some parts on other instruments in a range of
	// bars (a mid-song instrument change).
	InstrumentLock *InstrumentLock `json:"instrumentLock,omitempty"`
	// Extras are additional parts (at most MaxExtras), named extra1,
	// extra2, ... in Instruments and locks.
	Extras []ExtraPart `json:"extras,omitempty"`
}

// ExtraPart is an additional instrument: its role and its GM program (for
// the percussion role, the GM percussion key it plays).
type ExtraPart struct {
	Role    string `json:"role"`
	Program int    `json:"program"`
}

// InstrumentLock: in bars From..To (1-based, inclusive) the named parts play
// the given instruments instead of Settings.Instruments.
type InstrumentLock struct {
	Parts       []string       `json:"parts"` // melody, chords, bass, drums
	From        int            `json:"from"`
	To          int            `json:"to"`
	Instruments map[string]int `json:"instruments"` // part -> GM program / drum kit
}

// instrumentFor checks a program for a part and names it.
func instrumentFor(kind PartKind, prog int, base gmInstrument) (gmInstrument, error) {
	if kind == PartDrums {
		name, ok := DrumKits[prog]
		if !ok {
			return base, fmt.Errorf("unknown drum kit %d", prog)
		}
		base.program, base.name = byte(prog), name
		return base, nil
	}
	if prog < 0 || prog > 127 {
		return base, fmt.Errorf("GM program %d out of range", prog)
	}
	base.program, base.name = byte(prog), GMNames[prog]
	return base, nil
}

// TempoSection plays bars From..To (1-based, inclusive) at their own tempo;
// the rest of the song uses Settings.BPM. It is how a tempo change can leave
// some bars ("locked") at the tempo they had.
type TempoSection struct {
	From int `json:"from"`
	To   int `json:"to"`
	BPM  int `json:"bpm"`
}

// TransposeLock names parts that are not transposed, and where.
type TransposeLock struct {
	Parts []string `json:"parts"`          // melody, chords, bass
	From  int      `json:"from,omitempty"` // bars, 1-based inclusive; 0 = whole song
	To    int      `json:"to,omitempty"`
}

// MaxTranspose limits Settings.Transpose.
const MaxTranspose = 12

// Tempo limits for user-chosen tempos.
const (
	MinBPM = 40
	MaxBPM = 220
)

// MoodSliders shape a genre's character. Each runs -1..+1; 0 leaves the
// genre as it is.
type MoodSliders struct {
	Energy     float64 `json:"energy"`     // tempo, note length, drum volume
	Darkness   float64 `json:"darkness"`   // minor (+) or major (-) when the scale is auto
	Density    float64 `json:"density"`    // more notes and fewer rests (+)
	Swing      float64 `json:"swing"`      // added to the genre's swing
	Complexity float64 `json:"complexity"` // less repetition, wider range, 7th chords (+)
}

// Mood is a named preset of sliders. The five original moods also keep
// their legacy parameters for songs saved before sliders existed.
type Mood struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Sliders     MoodSliders `json:"sliders"`
	TempoFactor float64     `json:"-"` // legacy
	LengthShift int         `json:"-"` // legacy: +1 longer notes, -1 shorter
	RestDelta   float64     `json:"-"` // legacy: added to the ideal share of rests
	ModeBias    int         `json:"-"` // legacy: -1 minor, +1 major (auto scale)
	DrumVolume  int         `json:"-"` // legacy: percent of the genre's drum volume
}

var Moods = []Mood{
	{ID: "balanced", Name: "balanced", TempoFactor: 1, DrumVolume: 100},
	{ID: "calm", Name: "calm", Sliders: MoodSliders{Energy: -0.8, Density: -0.4, Complexity: -0.2},
		TempoFactor: 0.85, LengthShift: 1, RestDelta: 0.08, DrumVolume: 75},
	{ID: "energetic", Name: "energetic", Sliders: MoodSliders{Energy: 0.8, Density: 0.4, Complexity: 0.2},
		TempoFactor: 1.12, LengthShift: -1, RestDelta: -0.05, DrumVolume: 110},
	{ID: "dark", Name: "dark", Sliders: MoodSliders{Energy: -0.3, Darkness: 0.8},
		TempoFactor: 0.95, ModeBias: -1, DrumVolume: 100},
	{ID: "bright", Name: "bright", Sliders: MoodSliders{Energy: 0.3, Darkness: -0.8},
		TempoFactor: 1.05, ModeBias: 1, DrumVolume: 100},
	{ID: "melancholic", Name: "melancholic", Sliders: MoodSliders{Energy: -0.5, Darkness: 0.7, Density: -0.3, Complexity: 0.3}},
	{ID: "dreamy", Name: "dreamy", Sliders: MoodSliders{Energy: -0.6, Density: -0.7, Complexity: 0.5}},
	{ID: "aggressive", Name: "aggressive", Sliders: MoodSliders{Energy: 1, Darkness: 0.6, Density: 0.8, Complexity: -0.2}},
	{ID: "epic", Name: "epic", Sliders: MoodSliders{Energy: 0.4, Darkness: 0.4, Density: 0.2, Complexity: 0.6}},
	{ID: "romantic", Name: "romantic", Sliders: MoodSliders{Energy: -0.3, Darkness: -0.2, Density: -0.2, Swing: 0.3, Complexity: 0.4}},
	{ID: "playful", Name: "playful", Sliders: MoodSliders{Energy: 0.5, Darkness: -0.6, Density: 0.5, Swing: 0.5}},
}

// legacyMoods are the moods that existed before sliders; settings without
// sliders that name one of them use its legacy parameters.
var legacyMoods = map[string]bool{"balanced": true, "calm": true, "energetic": true, "dark": true, "bright": true}

// SliderTempo is the tempo sliders suggest for a genre.
func SliderTempo(genre *Style, sl MoodSliders) int {
	return int(math.Round(float64(genre.BPM) * (1 + 0.15*clampUnit(sl.Energy))))
}

func clampUnit(v float64) float64 { return math.Max(-1, math.Min(1, v)) }

// MoodByID finds a mood ("" = balanced).
func MoodByID(id string) (Mood, error) {
	if id == "" || id == "custom" {
		id = "balanced"
	}
	for _, m := range Moods {
		if m.ID == id {
			return m, nil
		}
	}
	return Mood{}, fmt.Errorf("unknown mood %q", id)
}

// DefaultBPM is the tempo a genre and mood suggest.
func DefaultBPM(genre *Style, mood Mood) int {
	return int(math.Round(float64(genre.BPM) * mood.TempoFactor))
}

// PartNames are the accompaniment parts a user can switch on.
var PartNames = map[string]Accompaniment{"chords": AccompChords, "bass": AccompBass, "drums": AccompDrums}

// InstrumentParts are the parts whose instrument can be chosen.
var InstrumentParts = map[string]PartKind{"lead": PartLead, "chords": PartChords, "bass": PartBass, "drums": PartDrums}

// BuildStyle applies settings to a genre preset and returns the resulting
// style, the accompaniment and the tempo. It is deterministic, so a genome
// file's settings always rebuild the same style.
func BuildStyle(s Settings) (*Style, Accompaniment, int, error) {
	base, err := StyleByName(s.Genre)
	if err != nil {
		return nil, 0, 0, err
	}
	mood, err := MoodByID(s.Mood)
	if err != nil {
		return nil, 0, 0, err
	}
	st := *base // copy; maps and slices are replaced below, never mutated
	st.Instruments = map[PartKind]gmInstrument{}
	for k, v := range base.Instruments {
		st.Instruments[k] = v
	}
	if len(s.Extras) > MaxExtras {
		return nil, 0, 0, fmt.Errorf("at most %d extra parts", MaxExtras)
	}
	st.Extras = nil
	nextChannel := 0
	for i, x := range s.Extras {
		role, err := RoleByID(x.Role)
		if err != nil {
			return nil, 0, 0, err
		}
		spec := extraSpec{Role: role}
		ins := gmInstrument{channel: 9}
		if role == RolePercussion {
			name, ok := GMPercussion[x.Program]
			if !ok {
				return nil, 0, 0, fmt.Errorf("extra%d: %d is not a GM percussion key (35-81)", i+1, x.Program)
			}
			spec.Key, ins.name = x.Program, name
		} else {
			if x.Program < 0 || x.Program > 127 {
				return nil, 0, 0, fmt.Errorf("extra%d: GM program %d out of range", i+1, x.Program)
			}
			ins = gmInstrument{channel: melodicChannels[nextChannel], program: byte(x.Program), name: GMNames[x.Program]}
			nextChannel++
		}
		st.Extras = append(st.Extras, spec)
		st.Instruments[ExtraKind(i)] = ins
	}

	if s.Scale != "" && s.Scale != "auto" {
		sc := ScaleByID(s.Scale)
		if sc == nil {
			return nil, 0, 0, fmt.Errorf("unknown scale %q", s.Scale)
		}
		st.Scale = sc
	}
	switch {
	case s.Tonic >= 12:
		return nil, 0, 0, fmt.Errorf("key %d out of range", s.Tonic)
	case s.Tonic >= 0:
		st.Tonic = s.Tonic + 1
	}
	sliders := s.Sliders
	if sliders == nil && !legacyMoods[mood.ID] {
		sliders = &mood.Sliders // a newer preset named without sliders
	}
	var defaultBPM int
	if sliders == nil {
		st.ModeBias = mood.ModeBias

		// mood: note lengths, rests, drums
		if mood.LengthShift != 0 {
			var d [8]int
			for i := range d {
				d[i] = base.Durations[max(0, min(7, i+mood.LengthShift))]
			}
			st.Durations = d
		}
		st.RestTarget = math.Max(0.03, base.RestTarget+mood.RestDelta)
		vol := base.DrumVolume
		if vol == 0 {
			vol = 100
		}
		st.DrumVolume = vol * mood.DrumVolume / 100
		defaultBPM = DefaultBPM(base, mood)
	} else {
		applySliders(&st, base, *sliders)
		defaultBPM = SliderTempo(base, *sliders)
	}

	bpm := s.BPM
	if bpm == 0 {
		bpm = defaultBPM
	}
	if bpm < MinBPM || bpm > MaxBPM {
		return nil, 0, 0, fmt.Errorf("tempo %d out of range %d-%d", bpm, MinBPM, MaxBPM)
	}
	st.BPM = bpm

	switch {
	case s.Bars > 0:
		if s.Bars > 2000 {
			return nil, 0, 0, fmt.Errorf("%d bars is too long", s.Bars)
		}
		st.Bars = s.Bars
	case s.Seconds > 0:
		if s.Seconds < 10 || s.Seconds > 900 {
			return nil, 0, 0, fmt.Errorf("duration %ds out of range 10-900", s.Seconds)
		}
		// whole 4/4 bars closest to the requested length at this tempo
		barSeconds := float64(st.BarSteps()) * 60 / (float64(bpm) * float64(st.BeatSteps()))
		st.Bars = max(2, int(math.Round(float64(s.Seconds)/barSeconds)))
	}
	if s.Transpose < -MaxTranspose || s.Transpose > MaxTranspose {
		return nil, 0, 0, fmt.Errorf("transpose %d out of range ±%d", s.Transpose, MaxTranspose)
	}
	st.Transpose = s.Transpose
	if ts := s.TempoSection; ts != nil {
		if st.Bars == 0 || ts.From < 1 || ts.To < ts.From || ts.To > st.Bars {
			return nil, 0, 0, fmt.Errorf("tempo section bars %d-%d out of range", ts.From, ts.To)
		}
		if ts.BPM < MinBPM || ts.BPM > MaxBPM {
			return nil, 0, 0, fmt.Errorf("section tempo %d out of range %d-%d", ts.BPM, MinBPM, MaxBPM)
		}
		st.SectionBPM, st.SectionFrom, st.SectionTo = ts.BPM, ts.From-1, ts.To
	}
	if tl := s.TransposeLock; tl != nil {
		for _, p := range tl.Parts {
			kind, ok := KindOf(p)
			if !ok || st.IsPerc(kind) {
				return nil, 0, 0, fmt.Errorf("cannot lock %q when transposing (only pitched parts)", p)
			}
			st.TransposeLock |= LockOf(kind)
		}
		if tl.From != 0 || tl.To != 0 {
			if tl.From < 1 || tl.To < tl.From || st.Bars > 0 && tl.To > st.Bars {
				return nil, 0, 0, fmt.Errorf("transpose lock bars %d-%d out of range", tl.From, tl.To)
			}
			st.TransposeFrom, st.TransposeTo = tl.From-1, tl.To
		}
	}

	var acc Accompaniment
	for _, p := range s.Parts {
		a, ok := PartNames[p]
		if !ok {
			return nil, 0, 0, fmt.Errorf("unknown part %q", p)
		}
		acc |= a
	}
	acc &= st.Acc

	for part, prog := range s.Instruments {
		kind, ok := KindOf(part)
		if !ok || kind.IsExtra() && int(kind-PartExtra) >= len(st.Extras) {
			return nil, 0, 0, fmt.Errorf("unknown instrument part %q", part)
		}
		if kind.IsExtra() && st.Extras[kind-PartExtra].Role == RolePercussion {
			name, ok := GMPercussion[prog]
			if !ok {
				return nil, 0, 0, fmt.Errorf("%s: %d is not a GM percussion key (35-81)", part, prog)
			}
			st.Extras[kind-PartExtra].Key = prog
			ins := st.Instruments[kind]
			ins.name = name
			st.Instruments[kind] = ins
			continue
		}
		ins, err := instrumentFor(kind, prog, st.Instruments[kind])
		if err != nil {
			return nil, 0, 0, err
		}
		st.Instruments[kind] = ins
	}
	if il := s.InstrumentLock; il != nil {
		if st.Bars == 0 || il.From < 1 || il.To < il.From || il.To > st.Bars {
			return nil, 0, 0, fmt.Errorf("instrument lock bars %d-%d out of range", il.From, il.To)
		}
		st.AltInstruments = map[PartKind]gmInstrument{}
		st.AltFrom, st.AltTo = il.From-1, il.To
		for _, p := range il.Parts {
			kind, ok := KindOf(p)
			if !ok || kind.IsExtra() && (int(kind-PartExtra) >= len(st.Extras) || st.IsPerc(kind)) {
				return nil, 0, 0, fmt.Errorf("part %q cannot switch instrument", p)
			}
			prog, ok := il.Instruments[p]
			if !ok {
				return nil, 0, 0, fmt.Errorf("no instrument kept for %s", p)
			}
			ins, err := instrumentFor(kind, prog, st.Instruments[kind])
			if err != nil {
				return nil, 0, 0, err
			}
			st.AltInstruments[kind] = ins
		}
	}
	return &st, acc, bpm, nil
}

// NotesFor is how many melody genes a style with a fixed length needs: enough
// to fill every bar even if every note is the shortest length.
func NotesFor(st *Style) int {
	shortest := st.Durations[0]
	for _, d := range st.Durations {
		shortest = min(shortest, d)
	}
	return (st.Bars*st.BarSteps() + shortest - 1) / shortest
}

// Instrument returns the GM program and name a style uses for a part.
func (s *Style) Instrument(k PartKind) (int, string) {
	ins := s.Instruments[k]
	return int(ins.program), ins.name
}

// GMNames are the 128 General MIDI melodic programs (0-based).
var GMNames = [128]string{
	"Acoustic Grand Piano", "Bright Acoustic Piano", "Electric Grand Piano", "Honky-tonk Piano", "Electric Piano 1", "Electric Piano 2", "Harpsichord", "Clavinet",
	"Celesta", "Glockenspiel", "Music Box", "Vibraphone", "Marimba", "Xylophone", "Tubular Bells", "Dulcimer",
	"Drawbar Organ", "Percussive Organ", "Rock Organ", "Church Organ", "Reed Organ", "Accordion", "Harmonica", "Tango Accordion",
	"Acoustic Guitar (nylon)", "Acoustic Guitar (steel)", "Electric Guitar (jazz)", "Electric Guitar (clean)", "Electric Guitar (muted)", "Overdriven Guitar", "Distortion Guitar", "Guitar Harmonics",
	"Acoustic Bass", "Electric Bass (finger)", "Electric Bass (pick)", "Fretless Bass", "Slap Bass 1", "Slap Bass 2", "Synth Bass 1", "Synth Bass 2",
	"Violin", "Viola", "Cello", "Contrabass", "Tremolo Strings", "Pizzicato Strings", "Orchestral Harp", "Timpani",
	"String Ensemble 1", "String Ensemble 2", "Synth Strings 1", "Synth Strings 2", "Choir Aahs", "Voice Oohs", "Synth Voice", "Orchestra Hit",
	"Trumpet", "Trombone", "Tuba", "Muted Trumpet", "French Horn", "Brass Section", "Synth Brass 1", "Synth Brass 2",
	"Soprano Sax", "Alto Sax", "Tenor Sax", "Baritone Sax", "Oboe", "English Horn", "Bassoon", "Clarinet",
	"Piccolo", "Flute", "Recorder", "Pan Flute", "Blown Bottle", "Shakuhachi", "Whistle", "Ocarina",
	"Lead 1 (square)", "Lead 2 (sawtooth)", "Lead 3 (calliope)", "Lead 4 (chiff)", "Lead 5 (charang)", "Lead 6 (voice)", "Lead 7 (fifths)", "Lead 8 (bass + lead)",
	"Pad 1 (new age)", "Pad 2 (warm)", "Pad 3 (polysynth)", "Pad 4 (choir)", "Pad 5 (bowed)", "Pad 6 (metallic)", "Pad 7 (halo)", "Pad 8 (sweep)",
	"FX 1 (rain)", "FX 2 (soundtrack)", "FX 3 (crystal)", "FX 4 (atmosphere)", "FX 5 (brightness)", "FX 6 (goblins)", "FX 7 (echoes)", "FX 8 (sci-fi)",
	"Sitar", "Banjo", "Shamisen", "Koto", "Kalimba", "Bagpipe", "Fiddle", "Shanai",
	"Tinkle Bell", "Agogo", "Steel Drums", "Woodblock", "Taiko Drum", "Melodic Tom", "Synth Drum", "Reverse Cymbal",
	"Guitar Fret Noise", "Breath Noise", "Seashore", "Bird Tweet", "Telephone Ring", "Helicopter", "Applause", "Gunshot",
}

// DrumKits are the GM2/GS drum kit programs (channel 10).
var DrumKits = map[int]string{
	0: "Standard Drum Kit", 8: "Room Kit", 16: "Power Kit", 24: "Electronic Kit",
	25: "TR-808 Kit", 32: "Jazz Kit", 40: "Brush Kit", 48: "Orchestra Kit",
}

// Suggested lists programs that suit each genre's parts, for "randomise
// from suggestions".
var Suggested = map[string]map[string][]int{
	"pop": {
		"lead":   {80, 81, 0, 4, 11, 27, 73, 85},
		"chords": {48, 49, 0, 4, 50, 89, 90},
		"bass":   {33, 34, 38, 35},
		"drums":  {0, 8, 16},
	},
	"blues": {
		"lead":   {65, 66, 26, 27, 29, 22, 56},
		"chords": {17, 16, 18, 0, 4, 5},
		"bass":   {32, 33, 35},
		"drums":  {0, 8, 32, 40},
	},
	"house": {
		"lead":   {81, 80, 87, 62, 4, 11},
		"chords": {0, 4, 5, 90, 62, 50},
		"bass":   {38, 39, 87, 33},
		"drums":  {25, 24, 0},
	},
	"ambient": {
		"lead":   {85, 88, 98, 102, 91, 11, 75, 73},
		"chords": {94, 89, 88, 95, 92, 50, 99},
		"bass":   {39, 38, 89, 42},
		"drums":  {25, 24, 40},
	},
}

// applySliders shapes a copy of a genre with mood sliders.
func applySliders(st *Style, base *Style, sl MoodSliders) {
	e, d, n, sw, c := clampUnit(sl.Energy), clampUnit(sl.Darkness), clampUnit(sl.Density), clampUnit(sl.Swing), clampUnit(sl.Complexity)
	switch {
	case d >= 0.34:
		st.ModeBias = -1
	case d <= -0.34:
		st.ModeBias = 1
	default:
		st.ModeBias = base.ModeBias
	}
	// energy and density shorten notes, calm and sparse lengthen them
	if shift := int(math.Round(-(0.7*e + 0.8*n))); shift != 0 {
		shift = max(-2, min(2, shift))
		var dur [8]int
		for i := range dur {
			dur[i] = base.Durations[max(0, min(7, i+shift))]
		}
		st.Durations = dur
	}
	st.RestTarget = math.Max(0.03, base.RestTarget-0.08*n-0.04*e)
	vol := base.DrumVolume
	if vol == 0 {
		vol = 100
	}
	st.DrumVolume = int(math.Round(float64(vol) * (1 + 0.25*e)))
	st.Swing = math.Max(0, math.Min(1, base.Swing+sw))
	st.MotifIdeal = math.Max(0.3, math.Min(0.95, base.MotifIdeal-0.25*c))
	st.RangeFull = base.RangeFull + int(math.Round(4*c))
	if c >= 0.5 && !base.Dominant {
		st.Sevenths = true
	}
}
