package melody

import (
	"bytes"
	"encoding/binary"
	"math"
	"sort"
)

// MIDI export: Standard MIDI File format 1, one track per part plus a
// conductor track, General MIDI instruments, drums on channel 10.

const (
	TicksPerQuarter = 480
)

// gmInstrument is the General MIDI setup for a part.
type gmInstrument struct {
	channel byte // 0-based; 9 is the GM percussion channel
	program byte // 0-based GM program number
	name    string
}

// midiVelocity maps 0..255 to MIDI 1..127.
func midiVelocity(v int) byte { return byte(max(1, min(127, v*127/255))) }

// midiPan maps 0..MaxPan to the 15%..85% part of the stereo field.
func midiPan(p int) byte { return byte(19 + p*89/MaxPan) }

type midiEvent struct {
	tick int
	off  bool // note-offs sort before other events at the same tick
	data []byte
}

// StepTick converts a time in sixteenths to MIDI ticks. swing (0..1) delays
// the off-beat eighth of each beat from half way (straight) to two thirds of
// the beat (1 = 2:1 shuffle); the sixteenths between move with it.
func StepTick(step int, swing float64) int {
	off := 240 + int(math.Round(80*swing)) // the off-beat eighth
	pos := [4]int{0, off / 2, off, off + (TicksPerQuarter-off)/2}
	return step/4*TicksPerQuarter + pos[step%4]
}

// MIDI returns the arrangement as a Standard MIDI File.
func MIDI(a Arrangement, bpm int, title string) []byte {
	if bpm <= 0 {
		bpm = 120
	}
	var tracks [][]midiEvent

	// Conductor track: name, tempo, time and key signature.
	usPerQuarter := int(math.Round(60_000_000 / a.Style.QuarterBPM(float64(a.Style.BarBPM(0, bpm)))))
	beats, unit := a.Style.meter()
	clocks := byte(24 * 4 / unit) // MIDI clocks per metronome click
	if a.Style.BeatSteps() == 6 {
		clocks = 36 // dotted quarter
	}
	logUnit := byte(0)
	for u := unit; u > 1; u /= 2 {
		logUnit++
	}
	mode := byte(0)
	if a.Scale.Minor {
		mode = 1
	}
	tracks = append(tracks, []midiEvent{
		{0, false, metaEvent(0x03, []byte(title))},
		{0, false, metaEvent(0x51, []byte{byte(usPerQuarter >> 16), byte(usPerQuarter >> 8), byte(usPerQuarter)})},
		{0, false, metaEvent(0x58, []byte{byte(beats), logUnit, clocks, 8})}, // time signature
		{0, false, metaEvent(0x59, []byte{byte(int8(keySignature(a.Tonic, a.Scale))), mode})},
	})
	for b := 1; b < a.Bars; b++ { // tempo changes
		if t := a.Style.BarBPM(b, bpm); t != a.Style.BarBPM(b-1, bpm) {
			us := int(math.Round(60_000_000 / a.Style.QuarterBPM(float64(t))))
			tracks[0] = append(tracks[0], midiEvent{StepTick(b*a.Style.BarSteps(), a.Style.Swing), false,
				metaEvent(0x51, []byte{byte(us >> 16), byte(us >> 8), byte(us)})})
		}
	}
	for b := 1; b < len(a.BarTonic); b++ { // key changes
		if a.BarTonic[b] != a.BarTonic[b-1] {
			sig := []byte{byte(int8(keySignature(a.BarTonic[b], a.Scale))), mode}
			tracks[0] = append(tracks[0], midiEvent{StepTick(b*a.Style.BarSteps(), a.Style.Swing), false, metaEvent(0x59, sig)})
		}
	}

	for _, p := range a.Parts {
		ins, _ := a.Style.InstrumentAt(p.Kind, 0)
		ch := ins.channel
		evs := []midiEvent{
			{0, false, metaEvent(0x03, []byte(p.Name))},
			{0, false, metaEvent(0x04, []byte(ins.name))},
		}
		perc := a.Style.IsPerc(p.Kind)
		if !(perc && p.Kind != PartDrums) { // percussion extras share the drum channel, its kit and volume
			evs = append(evs,
				midiEvent{0, false, []byte{0xC0 | ch, ins.program}},
				midiEvent{0, false, []byte{0xB0 | ch, 7, 100}}) // channel volume
		}
		if p.Kind == PartChords {
			// chord tones share one channel, so it is centred
			evs = append(evs, midiEvent{0, false, []byte{0xB0 | ch, 10, 64}})
		}
		// program changes where an instrument lock starts or ends (before
		// the notes, so they sort ahead of note-ons at the same tick)
		for b := 1; b < a.Bars; b++ {
			cur, _ := a.Style.InstrumentAt(p.Kind, b)
			prev, _ := a.Style.InstrumentAt(p.Kind, b-1)
			if cur.program != prev.program && !(perc && p.Kind != PartDrums) {
				evs = append(evs, midiEvent{StepTick(b*a.Style.BarSteps(), a.Style.Swing), false, []byte{0xC0 | ch, cur.program}})
			}
		}
		lastPan := -1
		for _, e := range p.Events {
			t := StepTick(e.Start, a.Style.Swing)
			if !perc && p.Kind != PartChords && e.Pan != lastPan {
				evs = append(evs, midiEvent{t, false, []byte{0xB0 | ch, 10, midiPan(e.Pan)}})
				lastPan = e.Pan
			}
			key, vel := byte(e.Pitch), midiVelocity(e.Velocity)
			evs = append(evs,
				midiEvent{t, false, []byte{0x90 | ch, key, vel}},
				midiEvent{StepTick(e.Start+e.Steps, a.Style.Swing), true, []byte{0x80 | ch, key, 0}})
		}
		tracks = append(tracks, evs)
	}

	var out bytes.Buffer
	out.WriteString("MThd")
	be := binary.BigEndian
	hdr := make([]byte, 10)
	be.PutUint32(hdr[0:4], 6)
	be.PutUint16(hdr[4:6], 1) // format 1
	be.PutUint16(hdr[6:8], uint16(len(tracks)))
	be.PutUint16(hdr[8:10], TicksPerQuarter)
	out.Write(hdr)
	for _, evs := range tracks {
		out.WriteString("MTrk")
		body := encodeTrack(evs)
		var n [4]byte
		be.PutUint32(n[:], uint32(len(body)))
		out.Write(n[:])
		out.Write(body)
	}
	return out.Bytes()
}

func metaEvent(typ byte, data []byte) []byte {
	return append(append([]byte{0xFF, typ}, varLen(len(data))...), data...)
}

func encodeTrack(evs []midiEvent) []byte {
	sort.SliceStable(evs, func(i, j int) bool {
		if evs[i].tick != evs[j].tick {
			return evs[i].tick < evs[j].tick
		}
		return evs[i].off && !evs[j].off
	})
	var b []byte
	last := 0
	for _, e := range evs {
		b = append(b, varLen(e.tick-last)...)
		b = append(b, e.data...)
		last = e.tick
	}
	return append(b, 0x00, 0xFF, 0x2F, 0x00) // end of track
}

// varLen encodes a MIDI variable-length quantity.
func varLen(v int) []byte {
	b := []byte{byte(v & 0x7F)}
	for v >>= 7; v > 0; v >>= 7 {
		b = append([]byte{byte(v&0x7F) | 0x80}, b...)
	}
	return b
}
