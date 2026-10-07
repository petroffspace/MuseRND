package melody

// More genres. Each is a Style preset: how genes are read (note lengths,
// chord vocabulary and rhythms, drum sounds, instruments) and what the
// detectors reward (harmony model, bass role, reference drum grooves). The
// notes still come from /dev/urandom.

// jazzTransitions rewards the ii-V-I family (vi-ii-V-I turnarounds,
// iii-vi), the backbone of jazz and bossa nova harmony.
var jazzTransitions = [7][7]int{
	//        I  ii iii IV  V  vi vii
	/* I   */ {-2, 2, 1, 2, 2, 3, 1},
	/* ii  */ {0, -2, 0, 0, 5, 0, 2},
	/* iii */ {0, 0, -2, 1, 0, 4, 0},
	/* IV  */ {2, 2, 0, -2, 3, 0, 2},
	/* V   */ {5, 0, 0, 0, -2, 3, 0},
	/* vi  */ {0, 5, 0, 2, 1, -2, 0},
	/* vii */ {4, 0, 1, 0, 1, 0, -2},
}

const sixteenths = 0xFFFF

var (
	Rock = &Style{
		ID: 4, Name: "rock", BPM: 120, Notes: 256, Acc: AccompAll,
		Durations: [8]int{2, 2, 2, 2, 4, 4, 6, 8}, RestTarget: 0.12, RangeFull: 12, MotifIdeal: 0.75, Grid: 2,
		Harmony: Functional, ChordDegrees: []int{0, 3, 4, 5, 1},
		ChordRhythms: [4][][2]int{
			{{0, 2}, {2, 2}, {4, 2}, {6, 2}, {8, 2}, {10, 2}, {12, 2}, {14, 2}}, // driving eighths
			{{0, 8}, {8, 8}},
			{{0, 16}},
			{{0, 6}, {6, 6}, {12, 4}},
		},
		ChordVelocity: 110, Bass: BassWithKick,
		Grooves:  []Groove{{0x0501, backbeat, eighths}, {0x0101, backbeat, eighths}, {0x0111, backbeat, eighths}},
		DrumKeys: DrumKeys{36, 38, 42, 49}, Fills: true,
		Instruments: map[PartKind]gmInstrument{
			PartLead:   {0, 30, "Distortion Guitar"},
			PartChords: {1, 29, "Overdriven Guitar"},
			PartBass:   {2, 34, "Electric Bass (pick)"},
			PartDrums:  {9, 16, "Power Kit"},
		},
		Weights: [4]float64{0.5, 0.15, 0.15, 0.2},
	}

	Funk = &Style{
		ID: 5, Name: "funk", BPM: 104, Notes: 320, Acc: AccompAll,
		Durations: [8]int{1, 1, 2, 2, 2, 3, 4, 6}, RestTarget: 0.25, RangeFull: 10, MotifIdeal: 0.85, Grid: 1,
		Harmony: Loop, ChordDegrees: []int{0, 3, 1, 4}, Sevenths: true,
		ChordRhythms: [4][][2]int{
			{{0, 1}, {3, 1}, {6, 1}, {10, 1}, {12, 1}},
			{{2, 1}, {6, 1}, {10, 1}, {14, 1}},
			{{0, 1}, {1, 1}, {4, 1}, {7, 1}, {10, 1}, {12, 1}, {15, 1}},
			{{0, 2}, {6, 2}, {10, 2}},
		},
		ChordVelocity: 100, Bass: BassSyncopated,
		Grooves:  []Groove{{0x0281, backbeat, sixteenths}, {0x0421, backbeat, eighths}, {0x0481, 0x1090, sixteenths}},
		DrumKeys: DrumKeys{36, 38, 42, 49},
		Instruments: map[PartKind]gmInstrument{
			PartLead:   {0, 61, "Brass Section"},
			PartChords: {1, 28, "Electric Guitar (muted)"},
			PartBass:   {2, 36, "Slap Bass 1"},
			PartDrums:  {9, 0, "Standard Drum Kit"},
		},
		Weights: [4]float64{0.45, 0.15, 0.2, 0.2},
	}

	Jazz = &Style{
		ID: 6, Name: "jazz", BPM: 140, Swing: 1, Notes: 224, Acc: AccompAll,
		Durations: [8]int{2, 2, 2, 2, 4, 4, 6, 8}, RestTarget: 0.15, RangeFull: 14, MotifIdeal: 0.55, Grid: 2,
		Harmony: Functional, ChordDegrees: []int{0, 1, 2, 3, 4, 5, 6}, Sevenths: true, Transitions: &jazzTransitions,
		ChordRhythms: [4][][2]int{
			{{0, 3}, {6, 4}},  // Charleston
			{{4, 2}, {12, 2}}, // on 2 and 4
			{{0, 16}},
			{{2, 2}, {10, 3}}, // anticipations
		},
		ChordVelocity: 80, Bass: BassWalking,
		// swung ride: beats plus the "and" of 2 and 4; hi-hat foot on 2 and 4
		Grooves:  []Groove{{0x0001, backbeat, 0x5151}, {0x0101, 0, 0x5151}, {0, backbeat, 0x5151}},
		DrumKeys: DrumKeys{36, 44, 51, 49},
		Instruments: map[PartKind]gmInstrument{
			PartLead:   {0, 66, "Tenor Sax"},
			PartChords: {1, 0, "Acoustic Grand Piano"},
			PartBass:   {2, 32, "Acoustic Bass"},
			PartDrums:  {9, 40, "Brush Kit"},
		},
		Weights: [4]float64{0.5, 0.2, 0.2, 0.1},
	}

	Soul = &Style{
		ID: 7, Name: "soul", BPM: 90, Swing: 0.3, Notes: 224, Acc: AccompAll,
		Durations: [8]int{1, 2, 2, 2, 4, 4, 6, 8}, RestTarget: 0.15, RangeFull: 12, MotifIdeal: 0.7, Grid: 2,
		Harmony: Functional, ChordDegrees: []int{0, 1, 2, 3, 4, 5}, Sevenths: true,
		ChordRhythms: [4][][2]int{
			{{0, 16}},
			{{0, 4}, {4, 4}, {8, 4}, {12, 4}},
			{{0, 8}, {8, 8}},
			{{0, 6}, {6, 10}},
		},
		ChordVelocity: 90, Bass: BassWithKick,
		Grooves:  []Groove{{0x0101, backbeat, eighths}, {0x0501, backbeat, eighths}},
		DrumKeys: DrumKeys{36, 38, 42, 49}, Fills: true,
		Instruments: map[PartKind]gmInstrument{
			PartLead:   {0, 53, "Voice Oohs"},
			PartChords: {1, 4, "Electric Piano 1"},
			PartBass:   {2, 33, "Electric Bass (finger)"},
			PartDrums:  {9, 0, "Standard Drum Kit"},
		},
		Weights: [4]float64{0.5, 0.2, 0.15, 0.15},
	}

	Techno = &Style{
		ID: 8, Name: "techno", BPM: 130, Notes: 384, Acc: AccompAll, ModeBias: -1,
		Durations: [8]int{1, 1, 2, 2, 2, 4, 4, 8}, RestTarget: 0.3, RangeFull: 8, MotifIdeal: 0.95, Grid: 2,
		Harmony: Loop, ChordDegrees: []int{0, 5, 3},
		ChordRhythms: [4][][2]int{
			{{2, 2}, {6, 2}, {10, 2}, {14, 2}},
			{{0, 16}},
			{{0, 1}, {3, 1}, {6, 1}},
			{{2, 1}, {10, 1}},
		},
		ChordVelocity: 80, Bass: BassOffbeat,
		Grooves:  []Groove{{beats, 0, offbeats}, {beats, backbeat, offbeats}, {beats, 0, 0xEEEE}},
		DrumKeys: DrumKeys{36, 39, 42, 49},
		Instruments: map[PartKind]gmInstrument{
			PartLead:   {0, 81, "Lead 2 (sawtooth)"},
			PartChords: {1, 90, "Pad 3 (polysynth)"},
			PartBass:   {2, 38, "Synth Bass 1"},
			PartDrums:  {9, 24, "Electronic Kit"},
		},
		Weights: [4]float64{0.35, 0.2, 0.2, 0.25},
	}

	Trap = &Style{
		ID: 9, Name: "trap", BPM: 140, Notes: 256, Acc: AccompAll, ModeBias: -1,
		Durations: [8]int{1, 2, 2, 2, 4, 4, 6, 8}, RestTarget: 0.25, RangeFull: 10, MotifIdeal: 0.85, Grid: 1,
		Harmony: Loop, ChordDegrees: []int{0, 5, 3, 4},
		ChordRhythms:  [4][][2]int{{{0, 16}}, {{0, 16}}, {{0, 8}, {8, 8}}, {{0, 12}, {12, 4}}},
		ChordVelocity: 70, Bass: BassWithKick,
		// half-time: clap on beat 3, rolling hats
		Grooves:  []Groove{{0x0401, 0x0100, sixteenths}, {0x0021, 0x0100, eighths}, {0x0201, 0x0100, sixteenths}},
		DrumKeys: DrumKeys{36, 39, 42, 49},
		Instruments: map[PartKind]gmInstrument{
			PartLead:   {0, 88, "Pad 1 (new age)"},
			PartChords: {1, 89, "Pad 2 (warm)"},
			PartBass:   {2, 38, "Synth Bass 1"},
			PartDrums:  {9, 25, "TR-808 Kit"},
		},
		Weights: [4]float64{0.45, 0.15, 0.2, 0.2},
	}

	DnB = &Style{
		ID: 10, Name: "dnb", BPM: 174, Notes: 256, Acc: AccompAll, ModeBias: -1,
		Durations: [8]int{2, 2, 4, 4, 4, 8, 8, 16}, RestTarget: 0.2, RangeFull: 12, MotifIdeal: 0.8, Grid: 2,
		Harmony: Loop, ChordDegrees: []int{0, 5, 3, 6}, Sevenths: true,
		ChordRhythms:  [4][][2]int{{{0, 16}}, {{0, 16}}, {{0, 8}, {8, 8}}, {{0, 16}}},
		ChordVelocity: 70, Bass: BassSyncopated,
		// two-step and an amen-like break
		Grooves:  []Groove{{0x0401, backbeat, eighths}, {0x0C01, 0x1210, eighths}},
		DrumKeys: DrumKeys{36, 38, 42, 49}, Fills: true,
		Instruments: map[PartKind]gmInstrument{
			PartLead:   {0, 85, "Lead 6 (voice)"},
			PartChords: {1, 89, "Pad 2 (warm)"},
			PartBass:   {2, 39, "Synth Bass 2"},
			PartDrums:  {9, 24, "Electronic Kit"},
		},
		Weights: [4]float64{0.4, 0.15, 0.2, 0.25},
	}

	Synthwave = &Style{
		ID: 11, Name: "synthwave", BPM: 100, Notes: 256, Acc: AccompAll, ModeBias: -1,
		Durations: [8]int{1, 2, 2, 2, 4, 4, 6, 8}, RestTarget: 0.12, RangeFull: 12, MotifIdeal: 0.75, Grid: 2,
		Harmony: Functional, ChordDegrees: []int{0, 5, 2, 6, 3, 4},
		ChordRhythms: [4][][2]int{
			{{0, 2}, {2, 2}, {4, 2}, {6, 2}, {8, 2}, {10, 2}, {12, 2}, {14, 2}}, // eighth pulse
			{{0, 16}},
			{{0, 8}, {8, 8}},
			{{0, 16}},
		},
		ChordVelocity: 85, Bass: BassWithKick,
		Grooves:  []Groove{{0x0101, backbeat, eighths}, {0x0101, backbeat, sixteenths}},
		DrumKeys: DrumKeys{36, 40, 42, 49}, Fills: true,
		Instruments: map[PartKind]gmInstrument{
			PartLead:   {0, 81, "Lead 2 (sawtooth)"},
			PartChords: {1, 90, "Pad 3 (polysynth)"},
			PartBass:   {2, 38, "Synth Bass 1"},
			PartDrums:  {9, 24, "Electronic Kit"},
		},
		Weights: [4]float64{0.5, 0.15, 0.15, 0.2},
	}

	Reggae = &Style{
		ID: 12, Name: "reggae", BPM: 76, Notes: 224, Acc: AccompAll,
		Durations: [8]int{2, 2, 2, 4, 4, 4, 6, 8}, RestTarget: 0.25, RangeFull: 10, MotifIdeal: 0.8, Grid: 2,
		Harmony: Functional, ChordDegrees: []int{0, 3, 4, 5, 1},
		ChordRhythms: [4][][2]int{
			{{2, 2}, {6, 2}, {10, 2}, {14, 2}}, // skank on the off-beats
			{{2, 1}, {6, 1}, {10, 1}, {14, 1}},
			{{4, 2}, {12, 2}},
			{{2, 2}, {6, 2}, {10, 2}, {14, 2}},
		},
		ChordVelocity: 95, Bass: BassSyncopated,
		// one drop: kick and rim click together on beat 3
		Grooves:  []Groove{{0x0100, 0x0100, eighths}, {0x0100, 0x0100, offbeats}, {0x0101, 0x0100, eighths}},
		DrumKeys: DrumKeys{36, 37, 42, 49},
		Instruments: map[PartKind]gmInstrument{
			PartLead:   {0, 27, "Electric Guitar (clean)"},
			PartChords: {1, 17, "Percussive Organ"},
			PartBass:   {2, 33, "Electric Bass (finger)"},
			PartDrums:  {9, 0, "Standard Drum Kit"},
		},
		Weights: [4]float64{0.45, 0.2, 0.2, 0.15},
	}

	Bossa = &Style{
		ID: 13, Name: "bossa", BPM: 130, Notes: 224, Acc: AccompAll,
		Durations: [8]int{2, 2, 2, 4, 4, 4, 6, 8}, RestTarget: 0.15, RangeFull: 12, MotifIdeal: 0.6, Grid: 2,
		Harmony: Functional, ChordDegrees: []int{0, 1, 2, 3, 4, 5, 6}, Sevenths: true, Transitions: &jazzTransitions,
		ChordRhythms: [4][][2]int{
			{{0, 2}, {3, 2}, {6, 2}, {10, 2}, {12, 2}}, // bossa guitar
			{{0, 3}, {6, 4}, {12, 4}},
			{{2, 2}, {6, 2}, {10, 2}, {14, 2}},
			{{0, 8}, {8, 8}},
		},
		ChordVelocity: 80, Bass: BassWithKick,
		// rim clicks on the bossa clave, kick on 1 and 3 with pick-ups
		Grooves:  []Groove{{0x0909, 0x1449, eighths}, {0x0101, 0x1449, eighths}},
		DrumKeys: DrumKeys{36, 37, 70, 49},
		Instruments: map[PartKind]gmInstrument{
			PartLead:   {0, 73, "Flute"},
			PartChords: {1, 24, "Acoustic Guitar (nylon)"},
			PartBass:   {2, 32, "Acoustic Bass"},
			PartDrums:  {9, 40, "Brush Kit"},
		},
		Weights: [4]float64{0.5, 0.2, 0.15, 0.15},
	}

	Latin = &Style{
		ID: 14, Name: "latin", BPM: 115, Notes: 256, Acc: AccompAll, ModeBias: -1,
		Durations: [8]int{1, 2, 2, 2, 4, 4, 6, 8}, RestTarget: 0.12, RangeFull: 12, MotifIdeal: 0.75, Grid: 2,
		Harmony: Loop, ChordDegrees: []int{0, 3, 4},
		ChordRhythms: [4][][2]int{
			{{0, 1}, {3, 1}, {6, 1}, {8, 1}, {11, 1}, {14, 1}}, // piano montuno
			{{2, 2}, {6, 2}, {10, 2}, {14, 2}},
			{{0, 3}, {6, 2}, {10, 2}, {12, 2}},
			{{0, 8}, {8, 8}},
		},
		ChordVelocity: 95, Bass: BassSyncopated,
		// son clave (either side, bar by bar) on claves, cowbell on the beats
		Grooves:  []Groove{{0x0100, 0x1041, beats}, {0x1001, 0x0110, beats}},
		DrumKeys: DrumKeys{36, 75, 56, 57},
		Instruments: map[PartKind]gmInstrument{
			PartLead:   {0, 56, "Trumpet"},
			PartChords: {1, 0, "Acoustic Grand Piano"},
			PartBass:   {2, 32, "Acoustic Bass"},
			PartDrums:  {9, 0, "Standard Drum Kit"},
		},
		Weights: [4]float64{0.45, 0.2, 0.2, 0.15},
	}

	Afrobeat = &Style{
		ID: 15, Name: "afrobeat", BPM: 110, Notes: 320, Acc: AccompAll,
		Durations: [8]int{1, 1, 2, 2, 2, 4, 4, 6}, RestTarget: 0.2, RangeFull: 10, MotifIdeal: 0.85, Grid: 2,
		AutoScale: nil, // set in init (dorian)
		Harmony:   Loop, ChordDegrees: []int{0, 3}, Sevenths: true,
		ChordRhythms: [4][][2]int{
			{{0, 1}, {2, 1}, {3, 1}, {6, 1}, {8, 1}, {10, 1}, {11, 1}, {14, 1}}, // interlocking guitar
			{{1, 1}, {4, 1}, {7, 1}, {9, 1}, {12, 1}, {15, 1}},
			{{2, 1}, {6, 1}, {10, 1}, {14, 1}},
			{{0, 2}, {6, 2}, {12, 2}},
		},
		ChordVelocity: 85, Bass: BassSyncopated,
		Grooves:  []Groove{{0x0481, backbeat, sixteenths}, {0x0101, backbeat, eighths}},
		DrumKeys: DrumKeys{36, 38, 42, 49},
		Instruments: map[PartKind]gmInstrument{
			PartLead:   {0, 61, "Brass Section"},
			PartChords: {1, 27, "Electric Guitar (clean)"},
			PartBass:   {2, 33, "Electric Bass (finger)"},
			PartDrums:  {9, 0, "Standard Drum Kit"},
		},
		Weights: [4]float64{0.45, 0.15, 0.2, 0.2},
	}
)

// Genres in 3/4 and 6/8. Masks and chord rhythms use 12 sixteenths per bar.
var (
	Waltz = &Style{
		ID: 16, Name: "waltz", Meter: [2]int{3, 4}, BPM: 150, Notes: 192, Acc: AccompAll,
		Durations: [8]int{2, 2, 4, 4, 4, 4, 8, 12}, RestTarget: 0.1, RangeFull: 12, MotifIdeal: 0.7, Grid: 2,
		Harmony: Functional, ChordDegrees: []int{0, 3, 4, 5, 1},
		ChordRhythms: [4][][2]int{
			{{4, 4}, {8, 4}}, // "pah-pah" on beats 2 and 3
			{{4, 3}, {8, 3}},
			{{0, 12}},
			{{4, 8}},
		},
		ChordVelocity: 85, Bass: BassDownbeat,
		Grooves:  []Groove{{0x0001, 0x0110, 0x0111}, {0x0001, 0, 0x0111}},
		DrumKeys: DrumKeys{36, 37, 42, 49}, DrumVolume: 60,
		Instruments: map[PartKind]gmInstrument{
			PartLead:   {0, 40, "Violin"},
			PartChords: {1, 0, "Acoustic Grand Piano"},
			PartBass:   {2, 43, "Contrabass"},
			PartDrums:  {9, 40, "Brush Kit"},
		},
		Weights: [4]float64{0.55, 0.2, 0.15, 0.1},
	}

	Minuet = &Style{
		ID: 17, Name: "minuet", Meter: [2]int{3, 4}, BPM: 110, Notes: 192, Acc: AccompBass | AccompChords,
		Durations: [8]int{2, 2, 2, 4, 4, 4, 8, 12}, RestTarget: 0.08, RangeFull: 12, MotifIdeal: 0.65, Grid: 2,
		Harmony: Functional, ChordDegrees: []int{0, 3, 4, 1, 5, 6},
		ChordRhythms: [4][][2]int{
			{{0, 12}},
			{{0, 4}, {4, 4}, {8, 4}},
			{{0, 8}, {8, 4}},
			{{4, 4}, {8, 4}},
		},
		ChordVelocity: 75, Bass: BassWalking,
		DrumKeys: DrumKeys{36, 38, 42, 49},
		Instruments: map[PartKind]gmInstrument{
			PartLead:   {0, 40, "Violin"},
			PartChords: {1, 6, "Harpsichord"},
			PartBass:   {2, 42, "Cello"},
			PartDrums:  {9, 48, "Orchestra Kit"},
		},
		Weights: [4]float64{0.6, 0.25, 0.15, 0},
	}

	Ballad68 = &Style{
		ID: 18, Name: "ballad68", Meter: [2]int{6, 8}, BPM: 60, Notes: 160, Acc: AccompAll,
		Durations: [8]int{2, 2, 2, 4, 4, 6, 6, 12}, RestTarget: 0.15, RangeFull: 12, MotifIdeal: 0.7, Grid: 2,
		Harmony: Functional, ChordDegrees: []int{0, 3, 4, 5, 1},
		ChordRhythms: [4][][2]int{
			{{0, 2}, {2, 2}, {4, 2}, {6, 2}, {8, 2}, {10, 2}}, // flowing eighths
			{{0, 6}, {6, 6}},
			{{0, 12}},
			{{0, 6}, {6, 3}, {9, 3}},
		},
		ChordVelocity: 80, Bass: BassDownbeat,
		// 6/8 ballad: kick on 1 (and 4), snare on 4, eighth hats
		Grooves:  []Groove{{0x0001, 0x0040, 0x0555}, {0x0041, 0x0040, 0x0555}},
		DrumKeys: DrumKeys{36, 38, 42, 49}, DrumVolume: 80,
		Instruments: map[PartKind]gmInstrument{
			PartLead:   {0, 73, "Flute"},
			PartChords: {1, 0, "Acoustic Grand Piano"},
			PartBass:   {2, 33, "Electric Bass (finger)"},
			PartDrums:  {9, 0, "Standard Drum Kit"},
		},
		Weights: [4]float64{0.55, 0.2, 0.15, 0.1},
	}

	Jig = &Style{
		ID: 19, Name: "jig", Meter: [2]int{6, 8}, BPM: 110, Notes: 288, Acc: AccompAll, ModeBias: 1,
		Durations: [8]int{2, 2, 2, 2, 4, 4, 6, 6}, RestTarget: 0.05, RangeFull: 12, MotifIdeal: 0.7, Grid: 2,
		Harmony: Functional, ChordDegrees: []int{0, 3, 4, 5},
		ChordRhythms: [4][][2]int{
			{{0, 6}, {6, 6}},
			{{0, 3}, {6, 3}},
			{{0, 2}, {4, 2}, {6, 2}, {10, 2}}, // down-up guitar
			{{0, 12}},
		},
		ChordVelocity: 85, Bass: BassDownbeat,
		// bodhran-like: low drum on 1 and 4, rim on the "3" and "6"
		Grooves:  []Groove{{0x0041, 0x0410, 0x0555}, {0x0041, 0, 0x0555}},
		DrumKeys: DrumKeys{36, 37, 70, 49},
		Instruments: map[PartKind]gmInstrument{
			PartLead:   {0, 110, "Fiddle"},
			PartChords: {1, 25, "Acoustic Guitar (steel)"},
			PartBass:   {2, 32, "Acoustic Bass"},
			PartDrums:  {9, 0, "Standard Drum Kit"},
		},
		Weights: [4]float64{0.55, 0.2, 0.1, 0.15},
	}
)

func init() {
	Afrobeat.AutoScale = ScaleByID("dorian")
	for _, g := range []struct {
		st           *Style
		title, group string
	}{
		{Pop, "pop", "Original"}, {Blues, "blues", "Original"}, {House, "house", "Original"}, {Ambient, "ambient", "Original"},
		{Rock, "rock", "Band"}, {Funk, "funk", "Band"}, {Jazz, "jazz", "Band"}, {Soul, "soul", "Band"},
		{Techno, "techno", "Electronic"}, {Trap, "trap", "Electronic"}, {DnB, "drum & bass", "Electronic"}, {Synthwave, "synthwave", "Electronic"},
		{Reggae, "reggae", "World"}, {Bossa, "bossa nova", "World"}, {Latin, "latin (son clave)", "World"}, {Afrobeat, "afrobeat", "World"},
		{Waltz, "waltz (3/4)", "3/4 & 6/8"}, {Minuet, "minuet (3/4)", "3/4 & 6/8"}, {Ballad68, "ballad (6/8)", "3/4 & 6/8"}, {Jig, "jig (6/8)", "3/4 & 6/8"},
	} {
		g.st.Title, g.st.Group = g.title, g.group
	}
	Styles = append(Styles, Rock, Funk, Jazz, Soul, Techno, Trap, DnB, Synthwave, Reggae, Bossa, Latin, Afrobeat,
		Waltz, Minuet, Ballad68, Jig)
	for i, s := range Styles {
		if int(s.ID) != i {
			panic("style IDs must match their position in Styles")
		}
	}
	for k, v := range map[string]map[string][]int{
		"rock":      {"lead": {30, 29, 81, 27, 56}, "chords": {29, 30, 27, 18}, "bass": {34, 33, 35}, "drums": {16, 0, 8}},
		"funk":      {"lead": {61, 56, 62, 27, 81}, "chords": {28, 27, 4, 7}, "bass": {36, 37, 33, 38}, "drums": {0, 8, 16}},
		"jazz":      {"lead": {66, 65, 56, 59, 71, 26}, "chords": {0, 4, 26, 11}, "bass": {32, 35}, "drums": {40, 32, 0}},
		"soul":      {"lead": {53, 52, 66, 56, 4}, "chords": {4, 5, 16, 0, 48}, "bass": {33, 34, 35}, "drums": {0, 8}},
		"techno":    {"lead": {81, 80, 87, 98}, "chords": {90, 89, 95, 50}, "bass": {38, 39, 87}, "drums": {24, 25}},
		"trap":      {"lead": {88, 10, 98, 81, 11}, "chords": {89, 90, 88, 48}, "bass": {38, 39}, "drums": {25, 24}},
		"dnb":       {"lead": {85, 81, 88, 54}, "chords": {89, 94, 95, 50}, "bass": {39, 38, 87}, "drums": {24, 25, 0}},
		"synthwave": {"lead": {81, 80, 62, 87}, "chords": {90, 50, 89, 91}, "bass": {38, 39}, "drums": {24, 25}},
		"reggae":    {"lead": {27, 22, 56, 16, 4}, "chords": {17, 16, 27, 4}, "bass": {33, 34, 35}, "drums": {0, 8}},
		"bossa":     {"lead": {73, 65, 66, 75, 24}, "chords": {24, 0, 4, 26}, "bass": {32, 33, 35}, "drums": {40, 32}},
		"latin":     {"lead": {56, 61, 73, 57}, "chords": {0, 4, 24}, "bass": {32, 33}, "drums": {0, 8}},
		"afrobeat":  {"lead": {61, 56, 66, 62}, "chords": {27, 28, 16, 4}, "bass": {33, 34, 36}, "drums": {0, 8}},
		"waltz":     {"lead": {40, 73, 21, 71, 0}, "chords": {0, 48, 21, 46}, "bass": {43, 32, 42}, "drums": {40, 0, 48}},
		"minuet":    {"lead": {40, 73, 68, 71}, "chords": {6, 0, 48, 46}, "bass": {42, 43, 70}, "drums": {48, 0}},
		"ballad68":  {"lead": {73, 52, 0, 40, 85}, "chords": {0, 4, 48, 24}, "bass": {33, 32, 42}, "drums": {0, 40}},
		"jig":       {"lead": {110, 74, 109, 105, 21}, "chords": {25, 105, 24, 21}, "bass": {32, 33}, "drums": {0, 40}},
	} {
		Suggested[k] = v
	}
}
