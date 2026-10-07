package melody

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// MusicXML export: uncompressed score-partwise MusicXML 4.0 with one part per
// instrument, notes split at barlines and tied, spelled for the detected key.

const divisions = 4 // per quarter note, so one division = one sixteenth

// noteValues are the written durations (in sixteenths) a single note can
// have, longest first, with their MusicXML type and dot count.
var noteValues = []struct {
	steps int
	typ   string
	dots  int
}{
	{16, "whole", 0}, {12, "half", 1}, {8, "half", 0}, {6, "quarter", 1},
	{4, "quarter", 0}, {3, "eighth", 1}, {2, "eighth", 0}, {1, "16th", 0},
}

// Drum notation per GM key: name, staff position, notehead and voice
// (1 = hands, 2 = feet).
var drumNotation = map[int]struct {
	name     string
	step     string
	octave   int
	notehead string
	voice    int
}{
	36: {"Bass Drum", "F", 4, "", 2},
	38: {"Snare Drum", "C", 5, "", 1},
	39: {"Hand Clap", "C", 5, "x", 1},
	42: {"Closed Hi-Hat", "G", 5, "x", 1},
	46: {"Open Hi-Hat", "G", 5, "circle-x", 1},
	49: {"Crash Cymbal", "A", 5, "x", 1},
	51: {"Ride Cymbal", "F", 5, "x", 1},
	37: {"Side Stick", "C", 5, "slashed", 1},
	59: {"Ride Cymbal 2", "F", 5, "x", 1},
	82: {"Shaker", "B", 5, "triangle", 1},
}

// drumNote is the notation of a GM percussion key, with a default staff
// position for sounds the table does not list.
func drumNote(key int) struct {
	name     string
	step     string
	octave   int
	notehead string
	voice    int
} {
	if d, ok := drumNotation[key]; ok {
		return d
	}
	name := GMPercussion[key]
	if name == "" {
		name = fmt.Sprintf("Percussion %d", key)
	}
	return struct {
		name     string
		step     string
		octave   int
		notehead string
		voice    int
	}{name, "E", 5, "", 1}
}

// slot is a note or chord (several pitches) or, with no pitches, a rest.
type slot struct {
	start, steps int
	pitches      []int
	velocity     int
}

// MusicXML returns the arrangement as an uncompressed MusicXML document.
func MusicXML(a Arrangement, bpm int, title string) []byte {
	if bpm <= 0 {
		bpm = 120
	}
	bars := max(1, (a.Steps+a.Style.BarSteps()-1)/a.Style.BarSteps())
	fifths := keySignature(a.Tonic, a.Scale)
	spellers := map[int]speller{}
	spellerAt := func(m int) speller {
		t := a.TonicAt(m)
		if _, ok := spellers[t]; !ok {
			spellers[t] = newSpeller(t, a.Scale)
		}
		return spellers[t]
	}

	var b bytes.Buffer
	w := func(format string, args ...any) { fmt.Fprintf(&b, format, args...) }
	w(`<?xml version="1.0" encoding="UTF-8" standalone="no"?>
<!DOCTYPE score-partwise PUBLIC "-//Recordare//DTD MusicXML 4.0 Partwise//EN" "http://www.musicxml.org/dtds/partwise.dtd">
<score-partwise version="4.0">
  <work><work-title>%s</work-title></work>
  <identification>
    <creator type="composer">MUseRND</creator>
    <encoding><software>MUseRND</software></encoding>
    <miscellaneous><miscellaneous-field name="key">%s</miscellaneous-field></miscellaneous>
  </identification>
  <part-list>
`, escape(title), a.KeyName())
	for i, p := range a.Parts {
		id := fmt.Sprintf("P%d", i+1)
		w("    <score-part id=%q>\n      <part-name>%s</part-name>\n", id, escape(p.Name))
		// The part's instrument, plus the alternative one if an instrument
		// lock switches it somewhere (drum ids get a "b" suffix, pitched
		// parts use I2); notes then name the instrument they are played on.
		kinds := []gmInstrument{a.Style.Instruments[p.Kind]}
		if a.Style.HasAlt(p.Kind) {
			kinds = append(kinds, a.Style.AltInstruments[p.Kind])
		}
		var scoreIns, midiIns strings.Builder
		perc := a.Style.IsPerc(p.Kind)
		for n, ins := range kinds {
			if perc {
				dk := a.Style.DrumKeys
				keys := []int{dk.Kick, dk.Snare, dk.Hat, dk.Crash}
				if p.Kind.IsExtra() { // a percussion extra plays one sound
					keys = []int{a.Style.Extras[p.Kind-PartExtra].Key}
				}
				suffix, kit := "", ""
				if n == 1 {
					suffix = "b"
				}
				if ins.program != 0 && p.Kind == PartDrums { // GM2/GS drum kit
					kit = fmt.Sprintf("<midi-program>%d</midi-program>", ins.program+1)
				}
				for _, k := range keys {
					name := drumNote(k).name
					if len(kinds) > 1 {
						name += " (" + ins.name + ")"
					}
					fmt.Fprintf(&scoreIns, "      <score-instrument id=\"%s-I%d%s\"><instrument-name>%s</instrument-name></score-instrument>\n", id, k, suffix, escape(name))
					fmt.Fprintf(&midiIns, "      <midi-instrument id=\"%s-I%d%s\"><midi-channel>10</midi-channel>%s<midi-unpitched>%d</midi-unpitched><volume>78.7402</volume></midi-instrument>\n", id, k, suffix, kit, k+1)
				}
				continue
			}
			fmt.Fprintf(&scoreIns, "      <score-instrument id=\"%s-I%d\"><instrument-name>%s</instrument-name></score-instrument>\n", id, n+1, escape(ins.name))
			fmt.Fprintf(&midiIns, "      <midi-instrument id=\"%s-I%d\"><midi-channel>%d</midi-channel><midi-program>%d</midi-program><volume>78.7402</volume></midi-instrument>\n",
				id, n+1, ins.channel+1, ins.program+1)
		}
		b.WriteString(scoreIns.String())
		b.WriteString(midiIns.String())
		w("    </score-part>\n")
	}
	w("  </part-list>\n")

	for i, p := range a.Parts {
		id := fmt.Sprintf("P%d", i+1)
		w("  <part id=%q>\n", id)
		perc := a.Style.IsPerc(p.Kind)
		voices := partVoices(p, bars, perc, a.Style.BarSteps())
		for m := 0; m < bars; m++ {
			w("    <measure number=\"%d\">\n", m+1)
			if t := a.Style.BarBPM(m, bpm); i == 0 && m > 0 && t != a.Style.BarBPM(m-1, bpm) { // tempo change
				w(`      <direction placement="above">
        <direction-type><metronome>%s<per-minute>%d</per-minute></metronome></direction-type>
        <sound tempo="%s"/>
      </direction>
`, beatUnitXML(a.Style), t, quarterTempo(a.Style, t))
			}
			if m > 0 && a.TonicAt(m) != a.TonicAt(m-1) && !perc { // key change
				w("      <attributes><key><fifths>%d</fifths><mode>%s</mode></key></attributes>\n",
					keySignature(a.TonicAt(m), a.Scale), a.Scale.XMLMode)
			}
			if m == 0 {
				w("      <attributes>\n        <divisions>%d</divisions>\n", divisions)
				if !perc {
					mode := a.Scale.XMLMode
					w("        <key><fifths>%d</fifths><mode>%s</mode></key>\n", fifths, mode)
				}
				beats, unit := a.Style.meter()
				w("        <time><beats>%d</beats><beat-type>%d</beat-type></time>\n", beats, unit)
				switch {
				case perc:
					w("        <clef><sign>percussion</sign></clef>\n")
				case p.Kind == PartBass, averagePitch(p) < 60:
					w("        <clef><sign>F</sign><line>4</line></clef>\n")
				default:
					w("        <clef><sign>G</sign><line>2</line></clef>\n")
				}
				w("      </attributes>\n")
				if i == 0 {
					w(`      <direction placement="above">
        <direction-type><metronome>%s<per-minute>%d</per-minute></metronome></direction-type>
        <sound tempo="%s">%s</sound>
      </direction>
`, beatUnitXML(a.Style), a.Style.BarBPM(0, bpm), quarterTempo(a.Style, a.Style.BarBPM(0, bpm)), swingXML(a.Style.Swing))
				}
			}
			for v, slots := range voices {
				if v > 0 {
					w("      <backup><duration>%d</duration></backup>\n", a.Style.BarSteps())
				}
				insNo := -1 // which instrument the notes name (-1: no instrument element)
				if perc || a.Style.HasAlt(p.Kind) {
					insNo = 0
					if _, alt := a.Style.InstrumentAt(p.Kind, m); alt {
						insNo = 1
					}
				}
				writeMeasureVoice(&b, slots, m, v+1, perc, id, spellerAt(m), insNo, a.Style.BarSteps())
			}
			if m == bars-1 {
				w("      <barline location=\"right\"><bar-style>light-heavy</bar-style></barline>\n")
			}
			w("    </measure>\n")
		}
		w("  </part>\n")
	}
	w("</score-partwise>\n")
	return b.Bytes()
}

// partVoices groups a part's events into one or two monophonic voices of
// slots (simultaneous onsets become chords).
func partVoices(p Part, bars int, perc bool, bs int) [][]slot {
	end := bars * bs
	group := func(evs []Event, ringToNext bool) []slot {
		var out []slot
		for _, e := range evs {
			if n := len(out); n > 0 && out[n-1].start == e.Start {
				out[n-1].pitches = append(out[n-1].pitches, e.Pitch)
				out[n-1].velocity = max(out[n-1].velocity, e.Velocity)
				continue
			}
			out = append(out, slot{e.Start, e.Steps, []int{e.Pitch}, e.Velocity})
		}
		if ringToNext {
			// Drum hits are written to last until the next hit (or bar end).
			for i := range out {
				next := end
				if i+1 < len(out) {
					next = out[i+1].start
				}
				barEnd := (out[i].start/bs + 1) * bs
				out[i].steps = min(next, barEnd) - out[i].start
			}
		}
		for i := range out {
			sort.Ints(out[i].pitches)
		}
		return out
	}
	if !perc {
		return [][]slot{group(p.Events, false)}
	}
	var hands, feet []Event
	for _, e := range p.Events {
		if drumNote(e.Pitch).voice == 2 {
			feet = append(feet, e)
		} else {
			hands = append(hands, e)
		}
	}
	return [][]slot{group(hands, true), group(feet, true)}
}

// writeMeasureVoice writes measure m of one voice, filling gaps with rests
// and splitting notes into tied written values.
func writeMeasureVoice(b *bytes.Buffer, slots []slot, m, voice int, perc bool, partID string, sp speller, insNo, bs int) {
	barStart, barEnd := m*bs, (m+1)*bs
	t := barStart
	emit := func(s slot, from, to int) {
		for from < to {
			v := noteValues[len(noteValues)-1]
			for _, nv := range noteValues {
				if nv.steps <= to-from {
					v = nv
					break
				}
			}
			tieStop := len(s.pitches) > 0 && from > s.start
			tieStart := len(s.pitches) > 0 && from+v.steps < s.start+s.steps
			writeNote(b, s, v.steps, v.typ, v.dots, voice, tieStop, tieStart, perc, partID, sp, insNo)
			from += v.steps
		}
	}
	for _, s := range slots {
		sEnd := s.start + s.steps
		if sEnd <= barStart || s.start >= barEnd {
			continue
		}
		from, to := max(s.start, barStart), min(sEnd, barEnd)
		if from > t {
			emit(slot{start: t, steps: from - t}, t, from) // rest
		}
		emit(s, from, to)
		t = to
	}
	if t < barEnd {
		emit(slot{start: t, steps: barEnd - t}, t, barEnd)
	}
}

func writeNote(b *bytes.Buffer, s slot, steps int, typ string, dots, voice int, tieStop, tieStart bool, perc bool, partID string, sp speller, insNo int) {
	if len(s.pitches) == 0 {
		fmt.Fprintf(b, "      <note><rest/><duration>%d</duration><voice>%d</voice><type>%s</type>%s</note>\n",
			steps, voice, typ, dotXML(dots))
		return
	}
	for i, pitch := range s.pitches {
		b.WriteString("      <note")
		fmt.Fprintf(b, " dynamics=\"%.2f\">", float64(midiVelocity(s.velocity))*100/90)
		if i > 0 {
			b.WriteString("<chord/>")
		}
		if perc {
			dn := drumNote(pitch)
			fmt.Fprintf(b, "<unpitched><display-step>%s</display-step><display-octave>%d</display-octave></unpitched>", dn.step, dn.octave)
		} else {
			step, alter, octave := sp.spell(pitch)
			b.WriteString("<pitch><step>" + step + "</step>")
			if alter != 0 {
				fmt.Fprintf(b, "<alter>%d</alter>", alter)
			}
			fmt.Fprintf(b, "<octave>%d</octave></pitch>", octave)
		}
		fmt.Fprintf(b, "<duration>%d</duration>", steps)
		if tieStop {
			b.WriteString(`<tie type="stop"/>`)
		}
		if tieStart {
			b.WriteString(`<tie type="start"/>`)
		}
		if perc {
			suffix := ""
			if insNo == 1 {
				suffix = "b"
			}
			fmt.Fprintf(b, "<instrument id=\"%s-I%d%s\"/>", partID, pitch, suffix)
		} else if insNo >= 0 {
			fmt.Fprintf(b, "<instrument id=\"%s-I%d\"/>", partID, insNo+1)
		}
		fmt.Fprintf(b, "<voice>%d</voice><type>%s</type>%s", voice, typ, dotXML(dots))
		if perc {
			stem := "up"
			if voice == 2 {
				stem = "down"
			}
			fmt.Fprintf(b, "<stem>%s</stem>", stem)
			if nh := drumNote(pitch).notehead; nh != "" {
				fmt.Fprintf(b, "<notehead>%s</notehead>", nh)
			}
		}
		if tieStop || tieStart {
			b.WriteString("<notations>")
			if tieStop {
				b.WriteString(`<tied type="stop"/>`)
			}
			if tieStart {
				b.WriteString(`<tied type="start"/>`)
			}
			b.WriteString("</notations>")
		}
		b.WriteString("</note>\n")
	}
}

// averagePitch is the mean pitch of a part's notes (60 if it has none).
func averagePitch(p Part) int {
	if len(p.Events) == 0 {
		return 60
	}
	t := 0
	for _, e := range p.Events {
		t += e.Pitch
	}
	return t / len(p.Events)
}

// beatUnitXML is the metronome's beat: a quarter, or a dotted quarter in 6/8.
func beatUnitXML(st *Style) string {
	if st.BeatSteps() == 6 {
		return "<beat-unit>quarter</beat-unit><beat-unit-dot/>"
	}
	return "<beat-unit>quarter</beat-unit>"
}

// quarterTempo is MusicXML's sound tempo (always in quarter notes).
func quarterTempo(st *Style, bpm int) string {
	return strconv.FormatFloat(st.QuarterBPM(float64(bpm)), 'f', -1, 64)
}

// swingXML describes the swing ratio of the off-beat eighths (e.g. 2:1).
func swingXML(swing float64) string {
	if swing <= 0 {
		return ""
	}
	first := 240 + int(math.Round(80*swing))
	second := TicksPerQuarter - first
	g := gcd(first, second)
	return fmt.Sprintf("<swing><first>%d</first><second>%d</second><swing-type>eighth</swing-type></swing>", first/g, second/g)
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

func dotXML(n int) string {
	s := ""
	for ; n > 0; n-- {
		s += "<dot/>"
	}
	return s
}

func escape(s string) string {
	var b bytes.Buffer
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

// speller names pitches the way the key's scale does (e.g. D# not Eb in
// E minor), falling back to sharps or flats per the key signature.
type speller struct {
	tonic, tonicLetter int
	degrees            [12]int // scale degree 0..6 for each interval above the tonic, or -1
	sharps             bool
}

var (
	letterNames = [7]string{"C", "D", "E", "F", "G", "A", "B"}
	letterPC    = [7]int{0, 2, 4, 5, 7, 9, 11}
	sharpNames  = [12][2]int{{0, 0}, {0, 1}, {1, 0}, {1, 1}, {2, 0}, {3, 0}, {3, 1}, {4, 0}, {4, 1}, {5, 0}, {5, 1}, {6, 0}} // letter, alter
	flatNames   = [12][2]int{{0, 0}, {1, -1}, {1, 0}, {2, -1}, {2, 0}, {3, 0}, {4, -1}, {4, 0}, {5, -1}, {5, 0}, {6, -1}, {6, 0}}
)

func newSpeller(tonic int, sc *Scale) speller {
	sp := speller{tonic: tonic, sharps: keySignature(tonic, sc) >= 0}
	if sp.sharps {
		sp.tonicLetter = sharpNames[tonic][0]
	} else {
		sp.tonicLetter = flatNames[tonic][0]
	}
	for i := range sp.degrees {
		sp.degrees[i] = -1
	}
	for d, iv := range sc.Steps {
		sp.degrees[iv] = d
	}
	if sc.RaiseLeading {
		sp.degrees[11] = 6 // raised leading tone
	}
	if sc.BlueNotes { // blue notes are spelled as lowered degrees: b3, b5, b7
		sp.degrees[3], sp.degrees[6], sp.degrees[10] = 2, 4, 6
	}
	return sp
}

// spell returns MusicXML step, alter and octave for a MIDI pitch.
func (sp speller) spell(pitch int) (step string, alter, octave int) {
	pc := pitch % 12
	var letter int
	if d := sp.degrees[(pc-sp.tonic+12)%12]; d >= 0 {
		letter = (sp.tonicLetter + d) % 7
		alter = (pc-letterPC[letter]+18)%12 - 6
	} else if sp.sharps {
		letter, alter = sharpNames[pc][0], sharpNames[pc][1]
	} else {
		letter, alter = flatNames[pc][0], flatNames[pc][1]
	}
	octave = (pitch-alter)/12 - 1
	return letterNames[letter], alter, octave
}
