package melody

import "strings"

// StepsPerBar is the bar length in sixteenth notes (4/4 time).
const StepsPerBar = 16

// Chord is a chord on a degree of the detected key, chosen by the genome:
// a triad or a seventh chord depending on the style.
type Chord struct {
	Degree int    // 0..6 (I..vii)
	Root   int    // pitch class 0..11
	Tones  []int  // pitch classes: root, third, fifth[, seventh]
	Name   string // e.g. "Am", "G7", "Bm7b5"
}

// transition[a][b] rates moving from degree a to degree b (common-practice
// tendencies; -2 discourages, 5 is the strongest V -> I pull).
var transition = [7][7]int{
	//        I  ii iii IV  V  vi vii
	/* I   */ {-2, 2, 1, 3, 3, 2, 1},
	/* ii  */ {0, -2, 0, 1, 4, 0, 2},
	/* iii */ {0, 0, -2, 2, 0, 3, 0},
	/* IV  */ {2, 1, 0, -2, 4, 0, 2},
	/* V   */ {5, 0, 0, 0, -2, 3, 0},
	/* vi  */ {1, 3, 0, 3, 1, -2, 0},
	/* vii */ {4, 0, 1, 0, 1, 0, -2},
}

// diatonicChord builds the triad (or seventh chord) on a scale degree.
func diatonicChord(degree, tonic int, sc *Scale, sevenths bool) Chord {
	st := sc.chordSteps(degree)
	n := 3
	if sevenths {
		n = 4
	}
	c := Chord{Degree: degree, Tones: make([]int, n)}
	for i := range c.Tones {
		c.Tones[i] = (tonic + st[(degree+2*i)%7]) % 12
	}
	c.Root = c.Tones[0]
	c.Name = chordName(c.Tones)
	return c
}

// dominantChord builds a dominant seventh on a degree of the major scale
// (the blues I7, IV7 and V7).
func dominantChord(degree, tonic int) Chord {
	root := (tonic + ionian[degree]) % 12
	c := Chord{Degree: degree, Root: root, Tones: []int{root, (root + 4) % 12, (root + 7) % 12, (root + 10) % 12}}
	c.Name = chordName(c.Tones)
	return c
}

// styleChord decodes a chord gene for a style.
func styleChord(gene byte, tonic int, sc *Scale, st *Style) Chord {
	degree := st.ChordDegrees[int(gene)%len(st.ChordDegrees)]
	if st.Dominant {
		return dominantChord(degree, tonic)
	}
	return diatonicChord(degree, tonic, sc, st.Sevenths)
}

func chordName(tones []int) string {
	iv := func(i int) int { return (tones[i] - tones[0] + 12) % 12 }
	name := noteNames[tones[0]]
	third, fifth := iv(1), iv(2)
	if len(tones) == 3 {
		switch {
		case third == 3 && fifth == 6:
			return name + "dim"
		case third == 4 && fifth == 8:
			return name + "+"
		case third == 3:
			return name + "m"
		}
		return name
	}
	switch q := [3]int{third, fifth, iv(3)}; q {
	case [3]int{4, 7, 11}:
		return name + "maj7"
	case [3]int{4, 7, 10}:
		return name + "7"
	case [3]int{3, 7, 10}:
		return name + "m7"
	case [3]int{3, 6, 10}:
		return name + "m7b5"
	case [3]int{3, 6, 9}:
		return name + "dim7"
	case [3]int{3, 7, 11}:
		return name + "m(maj7)"
	case [3]int{4, 8, 11}:
		return name + "+maj7"
	}
	return name + "?"
}

// FormatChords lists the progression, one bar per entry, 8 bars per line.
func FormatChords(chords []Chord) string {
	var b strings.Builder
	for i, c := range chords {
		if i > 0 {
			if i%8 == 0 {
				b.WriteString(" |\n")
			} else {
				b.WriteString(" | ")
			}
		}
		b.WriteString(c.Name)
	}
	return b.String()
}
