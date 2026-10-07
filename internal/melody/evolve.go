package melody

import (
	"bufio"
	"encoding/binary"
	"io"
	"runtime"
	"sort"
	"sync"
)

// Rand draws random numbers from a byte stream such as /dev/urandom.
type Rand struct{ r *bufio.Reader }

func NewRand(src io.Reader) *Rand { return &Rand{bufio.NewReaderSize(src, 1<<16)} }

// Bytes fills b from the source.
func (r *Rand) Bytes(b []byte) {
	if _, err := io.ReadFull(r.r, b); err != nil {
		panic("random source failed: " + err.Error())
	}
}

// Intn returns a value in [0, n).
func (r *Rand) Intn(n int) int {
	var b [4]byte
	r.Bytes(b[:])
	return int(binary.LittleEndian.Uint32(b[:]) % uint32(n))
}

// Options controls the search.
type Options struct {
	Notes       int           // melody length; each note is 3 random bytes
	Acc         Accompaniment // parts whose genes are evolved and scored
	Style       *Style        // genre the detectors score for
	Population  int           // genomes per generation
	Generations int           // upper bound on generations
	Target      float64       // stop once the best total score reaches this
	// Progress, if set, is called whenever the best score improves.
	Progress func(gen int, best Composition)
	// Stop, if set, is checked every generation; returning true ends the
	// search early (cancel button, time budget).
	Stop func() bool

	// Variations: Seed is a parent genome every starting genome is derived
	// from; Lock keeps parts unchanged for the whole search; Amount is how
	// many random changes each starting genome gets (< 0: fresh random bytes
	// for every unlocked part).
	Seed   []byte
	Lock   Lock
	Amount int
	// LockFrom..LockTo (bars, To exclusive) limits Lock to a section of the
	// song; LockTo 0 means the whole song. Outside the section every part
	// may change.
	LockFrom, LockTo int

	// Taste, if set, is blended into the total with TasteWeight (0..1).
	Taste       *Taste
	TasteWeight float64
}

type candidate struct {
	genome []byte
	score  Composition
}

// Evolve starts from random genomes (melody and accompaniment genes alike)
// and breeds them towards higher scores. Every random decision (initial
// bytes, selection, crossover points, mutations) is drawn from rng.
func Evolve(rng *Rand, opt Options) (genome []byte, best Composition, generations int) {
	st := opt.Style
	if st == nil {
		st = Pop
	}
	acc := opt.Acc & st.Acc
	size := GenomeSize(opt.Notes, st)
	if acc == AccompNone {
		size = opt.Notes * 3
	}
	eval := func(g []byte) Composition {
		c, _ := EvaluateGenome(g, opt.Notes, acc, st)
		if opt.Taste != nil {
			c.Taste = opt.Taste.Score(c.Features)
			c.Total = (1-opt.TasteWeight)*c.Base + opt.TasteWeight*100*c.Taste
		}
		return c
	}
	mask := opt.Lock
	if acc == AccompNone {
		mask |= LockChords | LockBass | LockDrums // no bar genes to change
	}
	lock := newLocker(opt.Seed, opt.Notes, st, mask, opt.LockFrom, opt.LockTo)
	pop := make([]candidate, opt.Population)
	for i := range pop {
		pop[i].genome = make([]byte, size)
		if opt.Seed == nil {
			rng.Bytes(pop[i].genome)
			continue
		}
		copy(pop[i].genome, opt.Seed)
		if opt.Amount < 0 {
			refresh(rng, pop[i].genome, opt.Notes, st, lock)
			continue
		}
		for m := 0; m < max(1, opt.Amount); m++ {
			mutateLocked(rng, pop[i].genome, opt.Notes, st, lock)
		}
	}
	evalAll(pop, eval)
	sortPop(pop)
	if opt.Progress != nil {
		opt.Progress(0, pop[0].score)
	}

	const elite = 4
	gen := 0
	for gen = 1; gen <= opt.Generations && pop[0].score.Total < opt.Target; gen++ {
		if opt.Stop != nil && opt.Stop() {
			break
		}
		next := make([]candidate, 0, len(pop))
		for i := 0; i < elite && i < len(pop); i++ {
			next = append(next, pop[i])
		}
		for len(next) < len(pop) {
			a := tournament(rng, pop)
			child := make([]byte, size)
			copy(child, a.genome)
			if rng.Intn(100) < 60 {
				crossoverLocked(rng, child, tournament(rng, pop).genome, opt.Notes, st, lock)
			}
			for m := 1 + rng.Intn(3); m > 0; m-- {
				mutateLocked(rng, child, opt.Notes, st, lock)
			}
			next = append(next, candidate{genome: child})
		}
		evalAll(next[elite:], eval)
		prev := pop[0].score.Total
		pop = next
		sortPop(pop)
		if opt.Progress != nil && pop[0].score.Total > prev {
			opt.Progress(gen, pop[0].score)
		}
	}
	return pop[0].genome, pop[0].score, gen - 1
}

// evalAll scores candidates in parallel. Children are bred sequentially
// beforehand, so the random stream (and thus the result for a given
// stream) does not depend on scheduling.
func evalAll(cs []candidate, eval func([]byte) Composition) {
	workers := min(runtime.NumCPU(), len(cs))
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := w; i < len(cs); i += workers {
				cs[i].score = eval(cs[i].genome)
			}
		}(w)
	}
	wg.Wait()
}

func sortPop(pop []candidate) {
	sort.SliceStable(pop, func(i, j int) bool { return pop[i].score.Total > pop[j].score.Total })
}

func tournament(rng *Rand, pop []candidate) candidate {
	best := pop[rng.Intn(len(pop))]
	for i := 0; i < 2; i++ {
		if c := pop[rng.Intn(len(pop))]; c.score.Total > best.score.Total {
			best = c
		}
	}
	return best
}

// crossover replaces a random run of notes, or of bars, in dst with the
// same run of src.
func crossover(rng *Rand, dst, src []byte, notes int, st *Style) {
	unit, off, n := 3, 0, notes
	if len(dst) > notes*3 && rng.Intn(2) == 0 {
		bars := MaxBars(notes, st)
		unit, off, n = BarGenes, notes*3, bars
		if k := rng.Intn(1 + len(st.Extras)); k > 0 { // an extra part's block
			unit, off = ExtraGenes, notes*3+bars*BarGenes+(k-1)*bars*ExtraGenes
		}
	}
	a, b := rng.Intn(n), rng.Intn(n)
	if a > b {
		a, b = b, a
	}
	copy(dst[off+a*unit:off+(b+1)*unit], src[off+a*unit:off+(b+1)*unit])
}

// activeNotes is how many melody genes are heard: with a fixed length the
// rest lie beyond the end and mutating them would be wasted.
func activeNotes(genome []byte, notes int, st *Style) int {
	if st.Bars == 0 {
		return notes
	}
	limit, steps := st.Bars*st.BarSteps(), 0
	for i := 0; i < notes; i++ {
		steps += st.Durations[genome[i*3+chG]>>5]
		if steps >= limit {
			return max(2, i+1)
		}
	}
	return notes
}

// activeBars is the number of bars the melody in a genome spans.
func activeBars(genome []byte, notes int, st *Style) int {
	if st.Bars > 0 {
		return st.Bars
	}
	steps := 0
	for i := 0; i < notes; i++ {
		steps += st.Durations[genome[i*3+chG]>>5]
	}
	return max(1, (steps+st.BarSteps()-1)/st.BarSteps())
}

// Lock marks parts a search may not change (variations).
type Lock int

const (
	LockMelody Lock = 1 << iota
	LockChords
	LockBass
	LockDrums
)

// bar gene sections: chord, bass, drums, and the lock protecting each
var barSections = [3]struct {
	from, to int
	lock     Lock
}{{0, 2, LockChords}, {2, 10, LockBass}, {10, 17, LockDrums}}

// mutateLocked applies one random change to an unlocked part: 3 in 4 changes
// go to the melody (the harder part) when both melody and bars may change.
func mutateLocked(rng *Rand, g []byte, notes int, st *Style, lk *locker) {
	barsFree := len(g) > notes*3 && !lk.allBarsLocked()
	melodyFree := !lk.melodyWhole()
	switch {
	case melodyFree && (!barsFree || rng.Intn(4) != 0):
		if !lk.melodyPartial() {
			mutate(rng, g[:activeNotes(g, notes, st)*3])
			return
		}
		// A change must leave the locked section's melody exactly as it was
		// (same notes at the same times); undo and retry otherwise.
		before := append([]byte(nil), g[:notes*3]...)
		for try := 0; try < 6; try++ {
			mutate(rng, g[:activeNotes(g, notes, st)*3])
			if lk.melodyOK(g) {
				return
			}
			copy(g, before)
		}
	case barsFree:
		mutateBar(rng, g, notes, st, lk)
	}
}

// crossoverLocked is crossover that keeps a partly locked melody intact.
func crossoverLocked(rng *Rand, dst, src []byte, notes int, st *Style, lk *locker) {
	if !lk.melodyPartial() {
		crossover(rng, dst, src, notes, st)
		return
	}
	before := append([]byte(nil), dst...)
	crossover(rng, dst, src, notes, st)
	if !lk.melodyOK(dst) {
		copy(dst, before)
	}
}

// refresh replaces every unlocked part of a genome with fresh random bytes.
func refresh(rng *Rand, g []byte, notes int, st *Style, lk *locker) {
	switch {
	case lk.mask&LockMelody == 0:
		rng.Bytes(g[:notes*3])
	case lk.melodyPartial():
		// Notes before the section keep their lengths (so the locked notes
		// stay in place) but get new pitches, volumes and pans; notes after
		// it are free.
		first, last := lk.melodySpan(g)
		var x [3]byte
		for i := 0; i < notes; i++ {
			p := g[i*3 : i*3+3]
			switch {
			case i < first:
				rng.Bytes(x[:])
				p[chR], p[chB] = x[0], x[1]
				p[chG] = p[chG]&^MaxPan | x[2]&MaxPan
			case i > last:
				rng.Bytes(p)
			}
		}
	}
	if len(g) <= notes*3 {
		return
	}
	for b := 0; b < MaxBars(notes, st); b++ {
		for _, sec := range barSectionsFor(g, notes, st) {
			if !lk.barLocked(b, sectionLock(sec.kind)) {
				rng.Bytes(sec.genes(b))
			}
		}
	}
}

// locker knows which parts of which bars a variation may not change.
type locker struct {
	notes    int
	st       *Style
	mask     Lock
	from, to int  // bar range the mask applies to (to exclusive)
	whole    bool // the range is the whole song
	melSig   []Note
	sigStart []int // start times (sixteenths) of melSig notes
}

func newLocker(seed []byte, notes int, st *Style, mask Lock, from, to int) *locker {
	bars := MaxBars(notes, st)
	if to <= 0 || to > bars {
		to = bars
	}
	from = max(0, min(from, to))
	lk := &locker{notes: notes, st: st, mask: mask, from: from, to: to, whole: from == 0 && to == bars}
	if lk.melodyPartial() && seed != nil {
		lk.melSig, lk.sigStart = lk.melodyIn(seed)
	}
	return lk
}

func (lk *locker) barLocked(b int, part Lock) bool {
	return lk.mask&part != 0 && b >= lk.from && b < lk.to
}

func (lk *locker) allBarsLocked() bool {
	if !lk.whole || lk.mask&(LockChords|LockBass|LockDrums) != LockChords|LockBass|LockDrums {
		return false
	}
	for i := range lk.st.Extras {
		if lk.mask&LockOf(ExtraKind(i)) == 0 {
			return false
		}
	}
	return true
}

func (lk *locker) melodyWhole() bool   { return lk.mask&LockMelody != 0 && lk.whole }
func (lk *locker) melodyPartial() bool { return lk.mask&LockMelody != 0 && !lk.whole }

// melodyIn returns the melody notes (rests included) that sound in the
// locked bars, with their start times.
func (lk *locker) melodyIn(g []byte) ([]Note, []int) {
	lo, hi := lk.from*lk.st.BarSteps(), lk.to*lk.st.BarSteps()
	var ns []Note
	var starts []int
	t := 0
	for _, n := range Decode(g[:lk.notes*3], lk.st) {
		if t < hi && t+n.Steps > lo {
			ns = append(ns, n)
			starts = append(starts, t)
		}
		t += n.Steps
	}
	return ns, starts
}

// melodyOK reports whether a genome's melody in the locked bars is
// unchanged: the same notes, starting at the same times.
func (lk *locker) melodyOK(g []byte) bool {
	ns, starts := lk.melodyIn(g)
	if len(ns) != len(lk.melSig) {
		return false
	}
	for i := range ns {
		if ns[i] != lk.melSig[i] || starts[i] != lk.sigStart[i] {
			return false
		}
	}
	return true
}

// melodySpan returns the indexes of the first and last melody genes that
// sound in the locked bars.
func (lk *locker) melodySpan(g []byte) (first, last int) {
	lo, hi := lk.from*lk.st.BarSteps(), lk.to*lk.st.BarSteps()
	first, last = lk.notes, -1
	t := 0
	for i, n := range Decode(g[:lk.notes*3], lk.st) {
		if t < hi && t+n.Steps > lo {
			first, last = min(first, i), i
		}
		t += n.Steps
	}
	return first, last
}

// section is one part's genes within a bar: a slice of the bar genes
// (chords, bass, drums) or an extra part's whole block.
type section struct {
	kind  int // 0 chords, 1 bass, 2 drums, 3+i extra part i
	genes func(b int) []byte
}

// barSectionsFor lists the sections of a style.
func barSectionsFor(genome []byte, notes int, st *Style) []section {
	var secs []section
	for i, sec := range barSections {
		sec := sec
		secs = append(secs, section{i, func(b int) []byte { return barGenes(genome, notes, b)[sec.from:sec.to] }})
	}
	for i := range st.Extras {
		i := i
		secs = append(secs, section{3 + i, func(b int) []byte { return extraGenes(genome, notes, st, i, b) }})
	}
	return secs
}

// sectionLock is the lock protecting a section.
func sectionLock(kind int) Lock {
	if kind < 3 {
		return barSections[kind].lock
	}
	return LockOf(ExtraKind(kind - 3))
}

// mutateBar changes unlocked accompaniment genes (chords, bass, drums or an
// extra part) of one bar in use.
func mutateBar(rng *Rand, genome []byte, notes int, st *Style, lk *locker) {
	bars := min(activeBars(genome, notes, st), MaxBars(notes, st))
	secs := barSectionsFor(genome, notes, st)
	var b int
	var free []section
	for try := 0; try < 8 && len(free) == 0; try++ {
		b = rng.Intn(bars)
		for _, sec := range secs {
			if !lk.barLocked(b, sectionLock(sec.kind)) {
				free = append(free, sec)
			}
		}
	}
	if len(free) == 0 {
		return
	}
	sec := free[rng.Intn(len(free))]
	g := sec.genes(b)
	switch op := rng.Intn(100); {
	case op < 30: // fresh random byte
		var x [1]byte
		rng.Bytes(x[:])
		g[rng.Intn(len(g))] = x[0]
	case op < 60: // copy this part of another bar (themes, grooves)
		copy(g, sec.genes(rng.Intn(bars)))
	case op < 78: // repeat the bar one or two before
		if b > 0 {
			copy(g, sec.genes(max(0, b-1-rng.Intn(2))))
		}
	case op < 84 && sec.kind != 0: // spread this bar's pattern (a groove) to every
		// bar, every other bar or every fourth bar it may change
		every := []int{1, 2, 4}[rng.Intn(3)]
		for o := b % every; o < bars; o += every {
			if o != b && !lk.barLocked(o, sectionLock(sec.kind)) {
				copy(sec.genes(o), g)
			}
		}
	default: // a typical change for this part
		switch sec.kind {
		case 0: // new chord
			g[0] = byte(rng.Intn(7))
		case 1: // new bass note in one slot
			g[rng.Intn(8)] = byte(0x80 | rng.Intn(6))
		default: // toggle one bit (a drum hit, a stab, an arpeggio step ...)
			bit := rng.Intn(len(g) * 8)
			g[bit/8] ^= 1 << (bit % 8)
		}
	}
}

// mutate changes the melody genes.
func mutate(rng *Rand, pix []byte) {
	n := len(pix) / 3
	i := rng.Intn(n)
	p := pix[i*3 : i*3+3]
	switch op := rng.Intn(100); {
	case op < 20: // fresh random channel byte
		var b [1]byte
		rng.Bytes(b[:])
		p[rng.Intn(3)] = b[0]
	case op < 40: // move pitch by 1-3 semitones
		steps := []int{-3, -2, -1, 1, 2, 3}[rng.Intn(6)]
		p[chR] = pitchToR(LowestPitch + int(p[chR])*PitchRange/256 + steps)
	case op < 50: // new duration, keep pan
		p[chG] = byte(rng.Intn(8)<<5) | p[chG]&MaxPan
	case op < 55: // toggle rest
		if p[chB] < RestBelow {
			p[chB] = byte(96 + rng.Intn(160))
		} else {
			p[chB] = byte(rng.Intn(RestBelow))
		}
	case op < 75: // copy a short figure elsewhere (creates motifs)
		copyRun(rng, pix, 0)
	case op < 85: // copy a figure transposed (sequence)
		copyRun(rng, pix, []int{-5, -4, -3, -2, 2, 3, 4, 5, 7}[rng.Intn(9)])
	case op < 95: // move the note near its predecessor's pitch
		if i > 0 {
			prev := LowestPitch + int(pix[(i-1)*3+chR])*PitchRange/256
			p[chR] = pitchToR(prev + rng.Intn(5) - 2)
		}
	default: // swap two notes
		j := rng.Intn(n)
		q := pix[j*3 : j*3+3]
		for k := 0; k < 3; k++ {
			p[k], q[k] = q[k], p[k]
		}
	}
}

func copyRun(rng *Rand, pix []byte, transpose int) {
	n := len(pix) / 3
	length := 2 + rng.Intn(7)
	if length > n/2 {
		length = max(1, n/2)
	}
	from, to := rng.Intn(n-length+1), rng.Intn(n-length+1)
	run := make([]byte, length*3)
	copy(run, pix[from*3:(from+length)*3])
	if transpose != 0 {
		for k := 0; k < length; k++ {
			r := &run[k*3+chR]
			*r = pitchToR(LowestPitch + int(*r)*PitchRange/256 + transpose)
		}
	}
	copy(pix[to*3:], run)
}
