package melody

// A Scale is the tonal frame of a composition: which notes the melody is
// rewarded for using, which 7-note scale the chords and note spelling come
// from, and how the key is written down.
type Scale struct {
	ID   string
	Name string
	// Steps is the 7-note scale chords are built on and notes are spelled
	// in (for 5- and 6-note scales, their 7-note parent).
	Steps [7]int
	// Weights says how much each semitone above the tonic counts as "in
	// scale" for the melody (1 = scale note, partial for colour notes).
	Weights [12]float64
	// RaiseLeading raises the 7th in V and vii (common-practice minor).
	RaiseLeading bool
	// Minor marks a minor-third tonic (MIDI key signature mode).
	Minor bool
	// ParentOffset is how far the tonic sits above the major key whose
	// signature is used (dorian 2, aeolian 9, ...).
	ParentOffset int
	// XMLMode is the MusicXML <mode> value.
	XMLMode string
	// BlueNotes spells b3, b5, b7 as lowered degrees.
	BlueNotes bool
}

func weightsOf(notes ...int) (w [12]float64) {
	for _, n := range notes {
		w[n] = 1
	}
	return w
}

var (
	ionian     = [7]int{0, 2, 4, 5, 7, 9, 11}
	aeolian    = [7]int{0, 2, 3, 5, 7, 8, 10}
	ScaleMajor = &Scale{ID: "major", Name: "major", Steps: ionian, Weights: weightsOf(ionian[:]...), XMLMode: "major"}
	// ScaleMinor is the common-practice minor: natural minor melody with the
	// raised leading tone allowed, and major V / diminished vii chords.
	ScaleMinor = &Scale{ID: "minor", Name: "minor", Steps: aeolian, Weights: weightsOf(0, 2, 3, 5, 7, 8, 10, 11),
		RaiseLeading: true, Minor: true, ParentOffset: 9, XMLMode: "minor"}
	ScaleBlues = &Scale{ID: "blues", Name: "blues", Steps: ionian, Weights: bluesScale, ParentOffset: 0,
		XMLMode: "major", BlueNotes: true}

	// Scales lists every selectable scale.
	Scales = []*Scale{
		ScaleMajor,
		ScaleMinor,
		{ID: "natural-minor", Name: "natural minor", Steps: aeolian, Weights: weightsOf(aeolian[:]...), Minor: true, ParentOffset: 9, XMLMode: "minor"},
		{ID: "harmonic-minor", Name: "harmonic minor", Steps: [7]int{0, 2, 3, 5, 7, 8, 11}, Weights: weightsOf(0, 2, 3, 5, 7, 8, 11), Minor: true, ParentOffset: 9, XMLMode: "minor"},
		{ID: "dorian", Name: "dorian", Steps: [7]int{0, 2, 3, 5, 7, 9, 10}, Weights: weightsOf(0, 2, 3, 5, 7, 9, 10), Minor: true, ParentOffset: 2, XMLMode: "dorian"},
		{ID: "phrygian", Name: "phrygian", Steps: [7]int{0, 1, 3, 5, 7, 8, 10}, Weights: weightsOf(0, 1, 3, 5, 7, 8, 10), Minor: true, ParentOffset: 4, XMLMode: "phrygian"},
		{ID: "lydian", Name: "lydian", Steps: [7]int{0, 2, 4, 6, 7, 9, 11}, Weights: weightsOf(0, 2, 4, 6, 7, 9, 11), ParentOffset: 5, XMLMode: "lydian"},
		{ID: "mixolydian", Name: "mixolydian", Steps: [7]int{0, 2, 4, 5, 7, 9, 10}, Weights: weightsOf(0, 2, 4, 5, 7, 9, 10), ParentOffset: 7, XMLMode: "mixolydian"},
		{ID: "locrian", Name: "locrian", Steps: [7]int{0, 1, 3, 5, 6, 8, 10}, Weights: weightsOf(0, 1, 3, 5, 6, 8, 10), Minor: true, ParentOffset: 11, XMLMode: "locrian"},
		{ID: "major-pentatonic", Name: "major pentatonic", Steps: ionian, Weights: weightsOf(0, 2, 4, 7, 9), XMLMode: "major"},
		{ID: "minor-pentatonic", Name: "minor pentatonic", Steps: aeolian, Weights: weightsOf(0, 3, 5, 7, 10), Minor: true, ParentOffset: 9, XMLMode: "minor"},
		ScaleBlues,
	}
)

// ScaleByID finds a scale ("" or "auto" returns nil: detect from the melody).
func ScaleByID(id string) *Scale {
	for _, s := range Scales {
		if s.ID == id {
			return s
		}
	}
	return nil
}

// chordSteps is the scale a chord on the given degree is built from.
func (s *Scale) chordSteps(degree int) [7]int {
	st := s.Steps
	if s.RaiseLeading && (degree == 4 || degree == 6) {
		st[6] = 11
	}
	return st
}

// keySignature returns the number of sharps (>0) or flats (<0) for a key.
func keySignature(tonic int, s *Scale) int {
	// major keys by tonic pitch class, preferring the conventional spelling
	fifths := [12]int{0, -5, 2, -3, 4, -1, 6, 1, -4, 3, -2, 5}
	return fifths[(tonic-s.ParentOffset+12)%12]
}
