package melody

import (
	"fmt"
	"math"
	"strings"
)

// Score is the "pleasantness" detector's verdict on a melody. Each component
// is in 0..1; Total is their weighted sum scaled to 0..100.
type Score struct {
	Total float64

	Key     float64 // tonal centre clarity (Krumhansl-Schmuckler correlation)
	InScale float64 // share of note time inside the detected key's scale
	Melodic float64 // stepwise motion, few awkward leaps or tritones
	Rhythm  float64 // notes on the eighth-note grid, a small set of lengths
	Rests   float64 // some breathing room, but not mostly silence
	Range   float64 // singable span
	Motif   float64 // recurring melodic/rhythmic figures (but not a loop)
	Cadence float64 // starts and ends on stable degrees of the key
	KeyName string
	// raw measurements (for taste learning)
	RestShare float64 // share of time that is silence
	Span      int     // semitones covered by the middle 90% of note time
	Repeat    float64 // share of 4-note figures that recur
	Notes     int
	Duration  int // total length in sixteenth notes
}

var weights = struct{ key, inScale, melodic, rhythm, rests, rng, motif, cadence float64 }{
	key: 10, inScale: 25, melodic: 20, rhythm: 10, rests: 5, rng: 10, motif: 15, cadence: 5,
}

func (s Score) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "score %.1f/100  key %s, %d notes, %d sixteenths\n", s.Total, s.KeyName, s.Notes, s.Duration)
	row := func(name string, v, w float64) { fmt.Fprintf(&b, "  %-9s %4.2f  (weight %2.0f)\n", name, v, w) }
	row("key", s.Key, weights.key)
	row("in-scale", s.InScale, weights.inScale)
	row("melodic", s.Melodic, weights.melodic)
	row("rhythm", s.Rhythm, weights.rhythm)
	row("rests", s.Rests, weights.rests)
	row("range", s.Range, weights.rng)
	row("motif", s.Motif, weights.motif)
	row("cadence", s.Cadence, weights.cadence)
	return b.String()
}

// Krumhansl-Kessler key profiles.
var (
	majorProfile = [12]float64{6.35, 2.23, 3.48, 2.33, 4.38, 4.09, 2.52, 5.19, 2.39, 3.66, 2.29, 2.88}
	minorProfile = [12]float64{6.33, 2.68, 3.52, 5.38, 2.60, 3.53, 2.54, 4.75, 3.98, 2.69, 3.34, 3.17}
)

// Krumhansl-Kessler profiles centred and divided by their norm, times 1000.
// For a fixed histogram, the dot product with these ranks keys exactly as
// Pearson correlation does, but in integers so key detection (which drives
// the harmony) is identical on every platform.
var (
	majorKeyInt = [12]int{655, -286, -1, -263, 205, 139, -220, 390, -250, 41, -273, -138}
	minorKeyInt = [12]int{655, -257, -47, 418, -277, -45, -292, 260, 68, -255, -92, -135}
)

// Blues tonal profile (relative weights of each degree above the tonic in
// blues melodies: minor pentatonic + blue fifth, with the major third and
// sixth as colour notes), and how much each degree counts as "in scale".
var (
	bluesKeyInt  = [12]int{6, -3, 0, 4, 1, 3, 1, 5, -3, 0, 4, -3}
	bluesProfile = [12]float64{6, -3, 0, 4, 1, 3, 1, 5, -3, 0, 4, -3}
	bluesScale   = [12]float64{1, 0, 0.5, 1, 0.7, 1, 0.8, 1, 0, 0.6, 1, 0}
)

// DetectKey returns the tonic pitch class and scale of a melody, weighting
// each note by its length. A style may force the scale and/or the tonic;
// otherwise blues styles use the blues scale, and the rest choose between
// major and minor (optionally restricted by ModeBias) with Krumhansl
// profiles. Integer arithmetic keeps the result identical on every platform.
func DetectKey(notes []Note, st *Style) (tonic int, sc *Scale) {
	var hist [12]int
	for _, n := range notes {
		if n.Pitch != Rest {
			hist[n.Pitch%12] += n.Steps
		}
	}
	type candidate struct {
		sc   *Scale
		prof [12]int
	}
	var cands []candidate
	switch {
	case st.Scale != nil:
		cands = []candidate{{st.Scale, scaleKeyInt(st.Scale)}}
	case st.Blues:
		cands = []candidate{{ScaleBlues, bluesKeyInt}}
	case st.AutoScale != nil:
		cands = []candidate{{st.AutoScale, scaleKeyInt(st.AutoScale)}}
	default:
		if st.ModeBias >= 0 {
			cands = append(cands, candidate{ScaleMajor, majorKeyInt})
		}
		if st.ModeBias <= 0 {
			cands = append(cands, candidate{ScaleMinor, minorKeyInt})
		}
	}
	best := math.MinInt
	for k := 0; k < 12; k++ {
		if st.Tonic > 0 && k != st.Tonic-1 {
			continue
		}
		for _, c := range cands {
			dot := 0
			for i, h := range hist {
				dot += h * c.prof[(i-k+12)%12]
			}
			if dot > best {
				best, tonic, sc = dot, k, c.sc
			}
		}
	}
	return tonic, sc
}

// scaleKeyInt is a key-finding profile for any scale: scale notes count for,
// other notes against, and the tonic and fifth are emphasised so modes that
// share notes (C ionian, D dorian, A aeolian ...) are told apart.
func scaleKeyInt(sc *Scale) (p [12]int) {
	for i, w := range sc.Weights {
		p[i] = int(w*6) - 3
	}
	p[0] += 4
	p[7] += 2
	return p
}

// KeyName formats a key, e.g. "A minor", "D dorian" or "A blues".
func KeyName(tonic int, sc *Scale) string {
	return noteNames[tonic] + " " + sc.Name
}

// Evaluate scores a melody against a style's melodic ideals.
func Evaluate(notes []Note, st *Style) Score {
	var s Score
	s.Notes = len(notes)

	var hist [12]float64
	var pitched []Note
	restSteps, totalSteps := 0, 0
	for _, n := range notes {
		totalSteps += n.Steps
		if n.Pitch == Rest {
			restSteps += n.Steps
			continue
		}
		pitched = append(pitched, n)
		hist[n.Pitch%12] += float64(n.Steps)
	}
	s.Duration = totalSteps
	if len(pitched) < 4 {
		s.KeyName = "none"
		return s
	}

	// Key: best correlation over 24 keys.
	tonic, sc := DetectKey(notes, st)
	prof := &majorProfile
	switch {
	case sc == ScaleBlues:
		prof = &bluesProfile
	case sc == ScaleMinor:
		prof = &minorProfile
	case sc != ScaleMajor:
		var p [12]float64
		for i, v := range scaleKeyInt(sc) {
			p[i] = float64(v)
		}
		prof = &p
	}
	var rot [12]float64
	for i := range rot {
		rot[i] = prof[(i-tonic+12)%12]
	}
	s.Key = clamp01(correlation(hist[:], rot[:]))
	s.KeyName = KeyName(tonic, sc)
	scale := sc.Weights
	degree := func(p int) int { return (p - tonic + 120) % 12 }

	// In-scale share of note time (squared so a few wrong notes hurt).
	var in, all float64
	for _, n := range pitched {
		all += float64(n.Steps)
		in += scale[degree(n.Pitch)] * float64(n.Steps)
	}
	s.InScale = math.Pow(in/all, 2)

	// Melodic intervals between consecutive sounding notes.
	var mel float64
	repeatRun := 0
	for i := 1; i < len(pitched); i++ {
		iv := pitched[i].Pitch - pitched[i-1].Pitch
		if iv == 0 {
			repeatRun++
		} else {
			repeatRun = 0
		}
		v := intervalScore(abs(iv))
		if repeatRun >= 3 {
			v *= 0.3
		}
		mel += v
	}
	s.Melodic = mel / float64(len(pitched)-1)

	// Rhythm: onsets on the style's grid, and only a few distinct lengths.
	onGrid, t := 0, 0
	lengths := map[int]bool{}
	for _, n := range notes {
		if t%st.Grid == 0 {
			onGrid++
		}
		t += n.Steps
		lengths[n.Steps] = true
	}
	distinct := 1.0
	switch len(lengths) {
	case 1:
		distinct = 0.5 // monotone rhythm is dull
	case 2, 3:
		distinct = 1
	case 4:
		distinct = 0.7
	default:
		distinct = 0.4
	}
	s.Rhythm = 0.6*float64(onGrid)/float64(len(notes)) + 0.4*distinct

	// Rests: aim for the style's share of silence.
	r := float64(restSteps) / float64(totalSteps)
	s.RestShare = r
	s.Rests = clamp01(1 - math.Abs(r-st.RestTarget)*4)

	// Range of the middle 90% of note time, so a lone climax or low note
	// does not count as the melody's whole span.
	var byPitch [128]int
	for _, n := range pitched {
		byPitch[n.Pitch] += n.Steps
	}
	lo, hi := percentilePitch(&byPitch, int(all)*5/100), percentilePitch(&byPitch, int(all)*95/100)
	s.Span = hi - lo
	switch span := hi - lo; {
	case span <= st.RangeFull:
		s.Range = 1
	case span <= st.RangeFull+7:
		s.Range = 0.8
	default:
		s.Range = clamp01(0.8 - float64(span-st.RangeFull-7)*0.06)
	}

	// Motif: share of positions whose 4-note figure (intervals + lengths)
	// recurs elsewhere. Some repetition is musical; total repetition is not.
	if len(pitched) >= 5 {
		// A figure packs 3 intervals and 3 lengths, one byte each.
		seen := make(map[uint64]int, len(pitched))
		figs := make([]uint64, 0, len(pitched)-3)
		for i := 0; i+3 < len(pitched); i++ {
			var f uint64
			for k := 0; k < 3; k++ {
				iv := pitched[i+k+1].Pitch - pitched[i+k].Pitch
				f = f<<16 | uint64(uint8(int8(iv)))<<8 | uint64(pitched[i+k].Steps)
			}
			figs = append(figs, f)
			seen[f]++
		}
		rep := 0
		for _, f := range figs {
			if seen[f] > 1 {
				rep++
			}
		}
		frac := float64(rep) / float64(len(figs))
		s.Repeat = frac
		if ideal := st.MotifIdeal; frac <= ideal {
			s.Motif = frac / ideal
		} else {
			s.Motif = clamp01(1 - (frac-ideal)*2)
		}
	}

	// Cadence: begin and end on tonic/third/fifth; long final tonic is best.
	stable := func(n Note) float64 {
		switch degree(n.Pitch) {
		case 0:
			return 1
		case 7, 3, 4:
			return 0.5
		}
		return 0
	}
	first, last := pitched[0], pitched[len(pitched)-1]
	s.Cadence = 0.3*stable(first) + 0.5*stable(last)
	if last.Steps >= 4 {
		s.Cadence += 0.2
	}

	s.Total = weights.key*s.Key + weights.inScale*s.InScale + weights.melodic*s.Melodic +
		weights.rhythm*s.Rhythm + weights.rests*s.Rests + weights.rng*s.Range +
		weights.motif*s.Motif + weights.cadence*s.Cadence
	return s
}

// percentilePitch returns the lowest pitch at which the cumulative note
// time exceeds rank.
func percentilePitch(byPitch *[128]int, rank int) int {
	acc := 0
	for p, steps := range byPitch {
		acc += steps
		if acc > rank {
			return p
		}
	}
	return len(byPitch) - 1
}

func intervalScore(iv int) float64 {
	switch iv {
	case 0:
		return 0.6
	case 1, 2:
		return 1
	case 3, 4:
		return 0.85
	case 5:
		return 0.7
	case 7:
		return 0.6
	case 12:
		return 0.5
	case 8, 9:
		return 0.4
	case 6:
		return 0.1 // tritone
	case 10, 11:
		return 0.2
	}
	return 0
}

func correlation(a, b []float64) float64 {
	var ma, mb float64
	for i := range a {
		ma += a[i]
		mb += b[i]
	}
	ma /= float64(len(a))
	mb /= float64(len(b))
	var num, da, db float64
	for i := range a {
		x, y := a[i]-ma, b[i]-mb
		num += x * y
		da += x * x
		db += y * y
	}
	if da == 0 || db == 0 {
		return 0
	}
	return num / math.Sqrt(da*db)
}

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
