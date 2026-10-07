package melody

import (
	"fmt"
	"math"
	"math/bits"
	"strings"
)

// Composition is the detector's verdict on a whole arrangement. Melody is the
// melody-only Score (0..100); Harmony, Bass and Drums are 0..1. Total is the
// style-weighted blend (0..100) over the parts that are present.
type Composition struct {
	Total   float64
	Melody  Score
	Harmony float64
	Bass    float64
	Drums   float64
	acc     Accompaniment

	// Base is Total before any taste adjustment; Taste is the learned
	// liking (0..1) when a taste model was used, else -1.
	Base  float64
	Taste float64
	// Features describe the piece for taste learning (see FeatureNames).
	Features []float64

	// Extras are the scores (0..1) of the extra parts, in order.
	Extras []float64

	// named sub-scores, for the report
	hSub, bSub, dSub []sub
	xSub             [][]sub
}

type sub struct {
	name string
	v    float64
}

func (c Composition) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "total %.1f/100\n", c.Total)
	b.WriteString(indent(c.Melody.String()))
	line := func(name string, v float64, subs []sub) {
		fmt.Fprintf(&b, "  %-9s %4.2f  (", name, v)
		for i, s := range subs {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%s %.2f", s.name, s.v)
		}
		b.WriteString(")\n")
	}
	if c.acc != AccompNone {
		line("harmony", c.Harmony, c.hSub)
	}
	if c.acc&AccompBass != 0 {
		line("bass", c.Bass, c.bSub)
	}
	if c.acc&AccompDrums != 0 {
		line("drums", c.Drums, c.dSub)
	}
	for i, v := range c.Extras {
		line(PartKey(ExtraKind(i)), v, c.xSub[i])
	}
	return b.String()
}

func indent(s string) string {
	return "  " + strings.ReplaceAll(strings.TrimRight(s, "\n"), "\n", "\n  ") + "\n"
}

// weigh combines sub-scores with weights summing to 1.
func weigh(subs []sub, w ...float64) float64 {
	t := 0.0
	for i, s := range subs {
		t += w[i] * s.v
	}
	return t
}

// EvaluateGenome decodes and scores a genome for a melody of n notes in a style.
func EvaluateGenome(genome []byte, n int, acc Accompaniment, st *Style) (c Composition, a Arrangement) {
	acc &= st.Acc
	a = Arrange(genome, n, acc, st)
	c = Composition{acc: acc, Melody: Evaluate(a.Notes, st), Taste: -1}
	defer func() { c.Base = c.Total; c.Features = features(&c, &a) }()
	if acc == AccompNone && len(st.Extras) == 0 {
		c.Total = c.Melody.Total
		return c, a
	}
	w := st.Weights

	c.scoreHarmony(&a)
	if a.Part(PartChords) != nil {
		c.scoreSeparation(&a)
	}
	sum, wsum := w[0]*c.Melody.Total+w[1]*100*c.Harmony, w[0]+w[1]

	var kicks []uint16
	if acc&AccompDrums != 0 {
		kicks = c.scoreDrums(genome, n, &a)
		sum += w[3] * 100 * c.Drums
		wsum += w[3]
	}
	if acc&AccompBass != 0 {
		c.scoreBass(&a, kicks)
		sum += w[2] * 100 * c.Bass
		wsum += w[2]
	}
	// extra parts: 0.08 each, at most 0.3 together, so the four main parts
	// keep most of the say
	if nx := len(st.Extras); nx > 0 {
		c.scoreExtras(&a, genome, n, kicks)
		wx := min(0.08, 0.3/float64(nx))
		for _, v := range c.Extras {
			sum += wx * 100 * v
			wsum += wx
		}
	}
	c.Total = sum / wsum
	// A weak part drags the total down, so a song cannot reach its target
	// on a strong melody alone while, say, its drums are still random.
	weak := func(v float64) {
		if v < weakPart {
			c.Total -= weakPenalty * (weakPart - v)
		}
	}
	weak(c.Harmony)
	if acc&AccompBass != 0 {
		weak(c.Bass)
	}
	if acc&AccompDrums != 0 && st.Weights[3] > 0 {
		weak(c.Drums)
	}
	for _, v := range c.Extras {
		weak(v)
	}
	return c, a
}

// Parts scoring below weakPart (0..1) cost weakPenalty points per unit.
const (
	weakPart    = 0.6
	weakPenalty = 15.0
)

// characterFeatures marks the Features that describe a piece's character
// (what it is like) rather than its quality (how well it follows the
// rules). Taste is learned from character only, so a dislike can never teach
// the search to make worse music.
var characterFeatures = []bool{
	false, false, false, false, true, true,
	true, true, true, true, true, true,
	true, true, false, false, false,
}

// FeatureNames name the Features of a Composition, each roughly 0..1.
var FeatureNames = []string{
	"key clarity", "in-scale notes", "smooth melody", "steady rhythm", "silence", "melodic range",
	"repetition", "note density", "note length", "register", "chord changes", "chord rhythm",
	"bass activity", "drum activity", "harmony score", "bass score", "drum score",
}

// features measures a scored arrangement for taste learning.
func features(c *Composition, a *Arrangement) []float64 {
	m := c.Melody
	bars := float64(max(1, a.Bars))
	var pitched, steps, pitchSum float64
	for _, n := range a.Notes {
		if n.Pitch != Rest {
			pitched++
			steps += float64(n.Steps)
			pitchSum += float64(n.Pitch)
		}
	}
	avgLen, register := 0.0, 0.0
	if pitched > 0 {
		avgLen = steps / pitched
		register = (pitchSum/pitched - LowestPitch) / PitchRange
	}
	changes := 0.0
	for i := 1; i < len(a.Chords); i++ {
		if a.Chords[i].Degree != a.Chords[i-1].Degree {
			changes++
		}
	}
	if len(a.Chords) > 1 {
		changes /= float64(len(a.Chords) - 1)
	}
	count := func(k PartKind, distinctStarts bool) float64 {
		p := a.Part(k)
		if p == nil {
			return 0
		}
		if !distinctStarts {
			return float64(len(p.Events))
		}
		n, last := 0.0, -1
		for _, e := range p.Events {
			if e.Start != last {
				n++
				last = e.Start
			}
		}
		return n
	}
	clip := func(v float64) float64 { return clamp01(v) }
	return []float64{
		m.Key, m.InScale, m.Melodic, m.Rhythm, clip(m.RestShare), clip(float64(m.Span) / 24),
		m.Repeat, clip(pitched / bars / 16), clip(avgLen / 16), clip(register), changes,
		clip(count(PartChords, true) / bars / 4), clip(count(PartBass, false) / bars / 8),
		clip(count(PartDrums, false) / bars / 32), c.Harmony, c.Bass, c.Drums,
	}
}

// melodyTimeline returns the sounding melody pitch (or Rest) and onset flag
// for every sixteenth of the arrangement.
func melodyTimeline(a *Arrangement) (pitch []int, onset []bool) {
	pitch = make([]int, a.Steps)
	onset = make([]bool, a.Steps)
	for i := range pitch {
		pitch[i] = Rest
	}
	t := 0
	for _, n := range a.Notes {
		if t < len(onset) {
			onset[t] = true
		}
		for k := 0; k < n.Steps && t+k < len(pitch); k++ {
			pitch[t+k] = n.Pitch
		}
		t += n.Steps
	}
	return pitch, onset
}

func inChord(pc int, c Chord) bool {
	for _, t := range c.Tones {
		if pc == t {
			return true
		}
	}
	return false
}

// chordFit is the weighted share of melody time on chord tones (notes struck
// on strong beats count most); in blues, blue notes over a chord count half.
func chordFit(a *Arrangement) float64 {
	pitch, onset := melodyTimeline(a)
	var in, all float64
	for i, p := range pitch {
		if p == Rest {
			continue
		}
		w := 1.0
		if onset[i] && a.Style.Strong(i) > 0 {
			w = 4
		} else if onset[i] && i%a.Style.BeatSteps() == 0 {
			w = 2
		}
		all += w
		ch := a.Chords[i/a.Style.BarSteps()]
		switch {
		case inChord(p%12, ch):
			in += w
		case a.Scale.BlueNotes && bluesScale[(p-a.Tonic+120)%12] >= 0.8:
			in += w / 2
		}
	}
	if all == 0 {
		return 0
	}
	return clamp01((in / all) / 0.75) // ~25% passing notes is normal
}

// twelveBar lists the degrees allowed in each bar of the 12-bar blues
// (I=0, IV=3, V=4), including the common quick-change and turnaround options.
var twelveBar = [12][]int{{0}, {0, 3}, {0}, {0}, {3}, {3}, {0}, {0}, {4}, {3, 4}, {0}, {4, 0}}

// scoreHarmony rates the chord genes with the style's harmony model.
func (c *Composition) scoreHarmony(a *Arrangement) {
	ch := a.Chords
	fit := chordFit(a)
	last := ch[len(ch)-1].Degree

	switch a.Style.Harmony {
	case TwelveBar:
		match := 0.0
		for b, x := range ch {
			for _, d := range twelveBar[b%12] {
				if x.Degree == d {
					match++
					break
				}
			}
		}
		end := 0.0
		if last == 0 {
			end = 1
		}
		c.hSub = []sub{{"12-bar form", match / float64(len(ch))}, {"fit", fit}, {"ends on I", end}}
		c.Harmony = weigh(c.hSub, 0.6, 0.25, 0.15)

	case Loop:
		rep, distinct := 1.0, map[int]bool{}
		if len(ch) > 4 {
			same := 0.0
			for b := 4; b < len(ch); b++ {
				if ch[b].Degree == ch[b-4].Degree {
					same++
				}
			}
			rep = clamp01(same / float64(len(ch)-4) / 0.85)
		}
		for _, x := range ch {
			distinct[x.Degree] = true
		}
		var few float64
		switch n := len(distinct); {
		case n >= 2 && n <= 4:
			few = 1
		case n == 1:
			few = 0.6
		case n == 5:
			few = 0.5
		default:
			few = 0.2
		}
		c.hSub = []sub{{"4-bar loop", rep}, {"few chords", few}, {"fit", fit}, {"progression", progression(ch, a.Style)}}
		c.Harmony = weigh(c.hSub, 0.35, 0.2, 0.35, 0.1)

	case Drift:
		// chords should last 2-4 bars and change smoothly (shared tones)
		var runs, smooth, changes float64
		runLen, nRuns := 1, 0.0
		addRun := func(l int) {
			nRuns++
			switch {
			case l >= 2 && l <= 4:
				runs++
			case l > 4:
				runs += 0.7
			default:
				runs += 0.3
			}
		}
		for b := 1; b < len(ch); b++ {
			if ch[b].Degree == ch[b-1].Degree {
				runLen++
				continue
			}
			addRun(runLen)
			runLen = 1
			changes++
			shared := 0
			for _, t := range ch[b].Tones {
				if inChord(t, ch[b-1]) {
					shared++
				}
			}
			smooth += min(1, float64(shared)/2)
		}
		addRun(runLen)
		smoothness := 1.0
		if changes > 0 {
			smoothness = smooth / changes
		}
		end := 0.0
		if last == 0 {
			end = 1
		}
		c.hSub = []sub{{"slow changes", runs / nRuns}, {"common tones", smoothness}, {"fit", fit}, {"ends on I", end}}
		c.Harmony = weigh(c.hSub, 0.3, 0.25, 0.35, 0.1)

	default: // Functional
		cad := 0.0
		if ch[0].Degree == 0 {
			cad += 0.2
		}
		if last == 0 {
			cad += 0.5
		}
		if n := len(ch); n > 1 {
			switch ch[n-2].Degree {
			case 4, 6:
				cad += 0.3
			case 3:
				cad += 0.15
			}
		}
		c.hSub = []sub{{"fit", fit}, {"progression", progression(ch, a.Style)}, {"cadence", cad}}
		c.Harmony = weigh(c.hSub, 0.45, 0.35, 0.2)
	}
}

// scoreSeparation rates how well melody and chords stay out of each other's
// way, and folds it into Harmony (20%): the melody should sit high enough for
// the chords to be voiced fully below it, and chord hits after the downbeat
// should fall where the melody holds or rests, not on its attacks.
func (c *Composition) scoreSeparation(a *Arrangement) {
	mel, onset := melodyTimeline(a)
	above := 0.0
	for b := 0; b < a.Bars; b++ {
		low := 1000
		for _, m := range mel[b*a.Style.BarSteps() : (b+1)*a.Style.BarSteps()] {
			if m != Rest {
				low = min(low, m)
			}
		}
		if low-3 >= chordTopMin { // chords fit under without being squeezed
			above++
		}
	}
	var hits, clear float64
	last := -1
	for _, e := range a.Part(PartChords).Events {
		if e.Start == last {
			continue // one check per chord attack, not per chord tone
		}
		last = e.Start
		if e.Start%a.Style.BarSteps() == 0 {
			continue // the downbeat is shared by design
		}
		hits++
		if !onset[e.Start] {
			clear++
		}
	}
	gaps := 1.0
	if hits > 0 {
		gaps = clear / hits
	}
	sep := []sub{{"above chords", above / float64(a.Bars)}, {"fills gaps", gaps}}
	c.hSub = append(c.hSub, sep...)
	c.Harmony = 0.8*c.Harmony + 0.2*weigh(sep, 0.5, 0.5)
}

// progression rates chord-to-chord moves with the transition table; holding
// a chord for two bars is fine, longer is dull.
func progression(ch []Chord, st *Style) float64 {
	table := &transition
	if st.Transitions != nil {
		table = st.Transitions
	}
	if len(ch) < 2 {
		return 1
	}
	var prog float64
	run := 1
	for i := 1; i < len(ch); i++ {
		from, to := ch[i-1].Degree, ch[i].Degree
		v := float64(table[from][to]+2) / 7
		if from == to {
			run++
			if run <= 2 {
				v = 0.6
			}
		} else {
			run = 1
		}
		prog += v
	}
	return prog / float64(len(ch)-1)
}

// densityScore rates a count against an ideal range, with partial credit
// one step outside it.
func densityScore(n, lo, hi int) float64 {
	switch {
	case n >= lo && n <= hi:
		return 1
	case n == lo-1 || n == hi+1:
		return 0.6
	case n > hi+1:
		return 0.3
	}
	return 0
}

// scoreBass rates the bass part for the style's bass role; kicks holds each
// bar's kick mask when the arrangement has drums.
func (c *Composition) scoreBass(a *Arrangement, kicks []uint16) {
	p := a.Part(PartBass)
	if len(p.Events) == 0 {
		c.bSub, c.Bass = []sub{{"notes", 0}}, 0
		return
	}
	onAt := make([]int32, a.Steps) // event index + 1 of the bass note starting at each step
	perBar := make([]int, a.Bars)
	for i, e := range p.Events {
		onAt[e.Start] = int32(i + 1)
		perBar[e.Start/a.Style.BarSteps()]++
	}
	kickAt := func(step int) bool {
		return kicks != nil && kicks[step/a.Style.BarSteps()]&(1<<(step%a.Style.BarSteps())) != 0
	}
	bars := float64(a.Bars)

	// shared measurements
	var root, fit, motion, onBeat, offBeat, onKick, rootPC, length float64
	for b := 0; b < a.Bars; b++ {
		if i := onAt[b*a.Style.BarSteps()]; i > 0 {
			switch e, ch := p.Events[i-1], a.Chords[b]; {
			case e.Pitch%12 == ch.Root:
				root++
			case inChord(e.Pitch%12, ch):
				root += 0.5
			}
		}
	}
	for i, e := range p.Events {
		ch := a.Chords[e.Start/a.Style.BarSteps()]
		switch {
		case inChord(e.Pitch%12, ch):
			fit++
		case e.Start%4 != 0: // passing tone off the beat
			fit += 0.7
		default:
			fit += 0.2
		}
		if e.Pitch%12 == ch.Root {
			rootPC++
		}
		if i > 0 {
			switch iv := abs(e.Pitch - p.Events[i-1].Pitch); {
			case iv <= 2:
				motion++
			case iv <= 5:
				motion += 0.9
			case iv == 7 || iv == 12:
				motion += 0.75
			default:
				motion += 0.4
			}
		}
		switch beat := a.Style.BeatSteps(); e.Start % beat {
		case 0:
			onBeat++
		case beat / 2:
			offBeat++
		}
		if kickAt(e.Start) {
			onKick++
		}
		length += float64(e.Steps)
	}
	n := float64(len(p.Events))
	root /= bars
	fit /= n
	if n > 1 {
		motion /= n - 1
	} else {
		motion = 1
	}
	density := func(lo, hi int) float64 {
		t := 0.0
		for _, k := range perBar {
			t += densityScore(k, lo, hi)
		}
		return t / bars
	}

	switch a.Style.Bass {
	case BassWalking:
		beats := a.Style.BarSteps() / a.Style.BeatSteps()
		c.bSub = []sub{{"downbeat root", root}, {"chord fit", fit}, {"stepwise", motion},
			{"walking density", density(beats, 2*beats)}, {"on the beat", onBeat / n}}
		c.Bass = weigh(c.bSub, 0.25, 0.2, 0.25, 0.15, 0.15)

	case BassOffbeat:
		clash := 1.0
		if kicks != nil {
			clash = 1 - onKick/n
		}
		c.bSub = []sub{{"offbeat", clamp01(offBeat / n / 0.75)}, {"avoids kick", clash},
			{"roots", clamp01(rootPC / n / 0.7)}, {"density", density(3, 5)}}
		c.Bass = weigh(c.bSub, 0.35, 0.2, 0.25, 0.2)

	case BassSyncopated:
		off := 1 - onBeat/n // share of notes off the beat
		c.bSub = []sub{{"syncopation", clamp01(1 - math.Abs(off-0.6)*2)}, {"chord fit", fit},
			{"roots", clamp01(rootPC / n / 0.5)}, {"density", density(3, 7)}}
		c.Bass = weigh(c.bSub, 0.3, 0.25, 0.2, 0.25)

	case BassDownbeat:
		beats := a.Style.BarSteps() / a.Style.BeatSteps()
		c.bSub = []sub{{"downbeat root", root}, {"on the beat", onBeat / n},
			{"sparse", density(1, max(1, beats-1))}, {"chord fit", fit}}
		c.Bass = weigh(c.bSub, 0.4, 0.2, 0.2, 0.2)

	case BassDrone:
		c.bSub = []sub{{"downbeat root", root}, {"long notes", clamp01(length / n / 8)},
			{"sparse", density(1, 2)}, {"chord fit", fit}}
		c.Bass = weigh(c.bSub, 0.3, 0.3, 0.2, 0.2)

	default: // BassWithKick
		lock := 1.0
		if kicks != nil {
			var kickHits, kickWithBass float64
			for b, k := range kicks {
				for s := 0; s < a.Style.BarSteps(); s++ {
					if k&(1<<s) != 0 {
						kickHits++
						if onAt[b*a.Style.BarSteps()+s] > 0 {
							kickWithBass++
						}
					}
				}
			}
			lock = 0.5*safeDiv(kickWithBass, kickHits) + 0.5*clamp01(onKick/n/0.5)
		}
		c.bSub = []sub{{"downbeat root", root}, {"chord fit", fit}, {"motion", motion},
			{"density", density(2, 5)}, {"with kick", lock}}
		c.Bass = weigh(c.bSub, 0.3, 0.25, 0.15, 0.15, 0.15)
	}
}

// jaccard compares two hit masks (1 = identical, both empty counts as equal).
func jaccard(a, b uint16) float64 {
	u := bits.OnesCount16(a | b)
	if u == 0 {
		return 1
	}
	return float64(bits.OnesCount16(a&b)) / float64(u)
}

// scoreDrums compares every bar with the style's reference grooves, and
// rewards a steady groove, phrase-end fills (if the style has them) and
// crashes on phrase starts. It returns the kick masks for the bass check.
func (c *Composition) scoreDrums(genome []byte, n int, a *Arrangement) []uint16 {
	type pattern struct{ kick, snare, hat uint16 }
	st := a.Style
	pats := make([]pattern, a.Bars)
	kicks := make([]uint16, a.Bars)
	var like, fill, fillBars, crashStart, phraseStarts, strayCrash float64
	for b := 0; b < a.Bars; b++ {
		k, s, h, cr := drumMasks(genome, n, b, st.BarSteps())
		pats[b] = pattern{k, s, h}
		kicks[b] = k

		best := 0.0
		for _, g := range st.Grooves {
			best = max(best, 0.4*jaccard(k, g.Kick)+0.35*jaccard(s, g.Snare)+0.25*jaccard(h, g.Hat))
		}
		phraseEnd := b%8 == 7 && b != a.Bars-1
		if st.Fills && phraseEnd {
			fillBars++
			bs, beat := st.BarSteps(), st.BeatSteps()
			lastBeat := uint16((1<<beat - 1) << (bs - beat))
			if bits.OnesCount16(s) >= 4 && bits.OnesCount16(s&lastBeat) >= 2 { // snare run in the last beat
				fill++
				best = max(best, 0.8) // a fill may leave the groove
			}
		}
		like += best

		if b%8 == 0 {
			phraseStarts++
			if cr {
				crashStart++
			}
		} else if cr && b != a.Bars-1 {
			strayCrash++
		}
	}
	bars := float64(a.Bars)

	// steady: most bars repeat the pattern of 1 or 2 bars earlier. It is
	// scaled by genre likeness, so repeating an off-genre pattern earns little.
	groove := 1.0
	if a.Bars > 2 {
		rep := 0.0
		for b := 2; b < a.Bars; b++ {
			if pats[b] == pats[b-1] || pats[b] == pats[b-2] {
				rep++
			}
		}
		groove = clamp01(rep / float64(a.Bars-2) / 0.7)
	}
	groove *= like / bars
	crash := 0.6*safeDiv(crashStart, phraseStarts) + 0.4*clamp01(1-strayCrash/bars*4)

	if st.Fills {
		fills := 1.0
		if fillBars > 0 {
			fills = fill / fillBars
		}
		c.dSub = []sub{{"genre groove", like / bars}, {"steady", groove}, {"fills", fills}, {"crash", crash}}
		c.Drums = weigh(c.dSub, 0.5, 0.25, 0.1, 0.15)
	} else {
		c.dSub = []sub{{"genre groove", like / bars}, {"steady", groove}, {"crash", crash}}
		c.Drums = weigh(c.dSub, 0.55, 0.3, 0.15)
	}
	return kicks
}

func safeDiv(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}
