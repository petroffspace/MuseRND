// Package melody turns random bytes into music. A genome is a byte string
// read 3 bytes per note (named after the RGB pixel layout it started as,
// stored B,G,R):
//
//	R  pitch     MIDI 57 + R*36/256  (A3..G#6, ~7 values per semitone)
//	G  duration  G>>5 indexes the style's Durations (in sixteenth notes)
//	   pan       G&31 places the note in the stereo field (0 left .. 31 right)
//	B  volume    B < RestBelow is a rest, otherwise velocity B
package melody

const (
	LowestPitch = 57 // A3: the melody sits above the chords
	PitchRange  = 36 // three octaves
	RestBelow   = 24 // blue values below this are rests
	Rest        = -1 // Note.Pitch value for a rest
)

// Note is one decoded 3-byte group.
type Note struct {
	Pitch    int // MIDI note number, or Rest
	Steps    int // length in sixteenth notes
	Velocity int // 0..255
	Pan      int // 0 (left) .. MaxPan (right)
}

const MaxPan = 31

// byte offsets within a note's 3 bytes
const (
	chB = 0
	chG = 1
	chR = 2
)

// Decode converts a genome's melody genes (3 bytes per note, B,G,R order)
// into notes, with note lengths from the style.
//
// With a fixed length (st.Bars), decoding stops when the bars are filled and
// the last note is shortened to fit.
func Decode(pix []byte, st *Style) []Note {
	limit := st.Bars * st.BarSteps()
	notes := make([]Note, len(pix)/3)
	total := 0
	for i := range notes {
		p := pix[i*3 : i*3+3]
		n := Note{
			Pitch:    LowestPitch + int(p[chR])*PitchRange/256,
			Steps:    st.Durations[p[chG]>>5],
			Velocity: int(p[chB]),
			Pan:      int(p[chG] & MaxPan),
		}
		if p[chB] < RestBelow {
			n.Pitch = Rest
		}
		notes[i] = n
		if limit > 0 {
			if total+n.Steps >= limit {
				notes[i].Steps = limit - total
				return notes[:i+1]
			}
			total += n.Steps
		}
	}
	return notes
}

// pitchToR returns an R value in the middle of the band for a MIDI pitch.
func pitchToR(pitch int) byte {
	semi := pitch - LowestPitch
	if semi < 0 {
		semi = 0
	}
	if semi >= PitchRange {
		semi = PitchRange - 1
	}
	// smallest R in band is ceil(semi*256/36); add half a band
	return byte((semi*256+PitchRange-1)/PitchRange + 3)
}

var noteNames = [12]string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}

// NoteName returns e.g. "C4" for MIDI 60.
func NoteName(pitch int) string {
	if pitch == Rest {
		return "rest"
	}
	return noteNames[pitch%12] + string(rune('0'+pitch/12-1))
}
