package melody

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
)

// Genome file format (.genome), little-endian:
//
//	magic   [8]byte "MUSERND\x02"
//	notes   uint32  melody length the genome was evolved for
//	acc     uint8   Accompaniment bits
//	style   uint8   Style ID
//	bpm     uint16  tempo
//	length  uint32  number of genome bytes
//	crc32   uint32  IEEE CRC-32 of the genome bytes
//	genome  [length]byte, exactly as drawn from /dev/urandom and evolved
//
// Version 1 files (no style byte, magic "MUSERND\x01") are read as pop.
//
// Version 3 (magic "MUSERND\x03") stores the user's Settings instead of a
// style ID, so the exact style (genre + mood, scale, key, length, tempo,
// instruments) is rebuilt on load:
//
//	magic    [8]byte "MUSERND\x03"
//	jsonLen  uint32, then {"settings": Settings, "notes": n} as JSON
//	length   uint32, crc32 uint32, genome [length]byte
var (
	genomeMagic   = []byte("MUSERND\x02")
	genomeMagicV1 = []byte("MUSERND\x01")
	genomeMagicV3 = []byte("MUSERND\x03")
)

type genomeHeaderV3 struct {
	Settings Settings `json:"settings"`
	Notes    int      `json:"notes"`
}

const genomeHeaderSize = 8 + 4 + 1 + 1 + 2 + 4 + 4

// GenomeFile is a saved composition: its genome plus the settings needed to
// decode it the same way again.
type GenomeFile struct {
	Notes  int
	Acc    Accompaniment
	Style  *Style
	BPM    int
	Genome []byte
	// Settings, when set, are saved instead of the style ID (version 3) and
	// rebuild Style, Acc and BPM on load.
	Settings *Settings
}

// MarshalGenome encodes a genome file.
func MarshalGenome(f GenomeFile) []byte {
	if f.Settings != nil {
		js, err := json.Marshal(genomeHeaderV3{*f.Settings, f.Notes})
		if err != nil {
			panic(err) // Settings always marshal
		}
		le := binary.LittleEndian
		out := append([]byte{}, genomeMagicV3...)
		out = le.AppendUint32(out, uint32(len(js)))
		out = append(out, js...)
		out = le.AppendUint32(out, uint32(len(f.Genome)))
		out = le.AppendUint32(out, crc32.ChecksumIEEE(f.Genome))
		return append(out, f.Genome...)
	}
	out := make([]byte, genomeHeaderSize+len(f.Genome))
	le := binary.LittleEndian
	copy(out, genomeMagic)
	le.PutUint32(out[8:], uint32(f.Notes))
	out[12] = byte(f.Acc)
	out[13] = f.Style.ID
	le.PutUint16(out[14:], uint16(f.BPM))
	le.PutUint32(out[16:], uint32(len(f.Genome)))
	le.PutUint32(out[20:], crc32.ChecksumIEEE(f.Genome))
	copy(out[genomeHeaderSize:], f.Genome)
	return out
}

// UnmarshalGenome decodes and validates a genome file.
func UnmarshalGenome(b []byte) (GenomeFile, error) {
	var f GenomeFile
	if len(b) >= 8 && bytes.Equal(b[:8], genomeMagicV3) {
		return unmarshalV3(b)
	}
	if len(b) >= 8 && bytes.Equal(b[:8], genomeMagicV1) {
		// v1: same layout without the style byte
		v2 := append(append(append([]byte{}, genomeMagic...), b[8:13]...), Pop.ID)
		return UnmarshalGenome(append(v2, b[13:]...))
	}
	if len(b) < genomeHeaderSize || !bytes.Equal(b[:8], genomeMagic) {
		return f, errors.New("not a MUseRND genome file")
	}
	le := binary.LittleEndian
	f.Notes = int(le.Uint32(b[8:]))
	f.Acc = Accompaniment(b[12])
	st, err := StyleByID(b[13])
	if err != nil {
		return f, err
	}
	f.Style = st
	f.BPM = int(le.Uint16(b[14:]))
	n := int(le.Uint32(b[16:]))
	if n != len(b)-genomeHeaderSize {
		return f, fmt.Errorf("genome length %d does not match file size", n)
	}
	f.Genome = b[genomeHeaderSize:]
	if crc32.ChecksumIEEE(f.Genome) != le.Uint32(b[20:]) {
		return f, errors.New("genome checksum mismatch: file is corrupted")
	}
	want := GenomeSize(f.Notes, f.Style)
	if f.Acc == AccompNone {
		want = f.Notes * 3
	}
	if f.Notes < 1 || n != want {
		return f, fmt.Errorf("genome has %d bytes, want %d for %d notes", n, want, f.Notes)
	}
	return f, nil
}

func unmarshalV3(b []byte) (GenomeFile, error) {
	var f GenomeFile
	le := binary.LittleEndian
	if len(b) < 12 {
		return f, errors.New("genome file truncated")
	}
	jl := int(le.Uint32(b[8:]))
	if 12+jl+8 > len(b) {
		return f, errors.New("genome file truncated")
	}
	var h genomeHeaderV3
	if err := json.Unmarshal(b[12:12+jl], &h); err != nil {
		return f, fmt.Errorf("genome settings: %w", err)
	}
	st, acc, bpm, err := BuildStyle(h.Settings)
	if err != nil {
		return f, fmt.Errorf("genome settings: %w", err)
	}
	f = GenomeFile{Notes: h.Notes, Acc: acc, Style: st, BPM: bpm, Settings: &h.Settings}
	rest := b[12+jl:]
	n := int(le.Uint32(rest))
	if n != len(rest)-8 {
		return f, fmt.Errorf("genome length %d does not match file size", n)
	}
	f.Genome = rest[8:]
	if crc32.ChecksumIEEE(f.Genome) != le.Uint32(rest[4:]) {
		return f, errors.New("genome checksum mismatch: file is corrupted")
	}
	if want := GenomeSize(f.Notes, f.Style); f.Notes < 1 || n != want && !(acc == AccompNone && n == f.Notes*3) {
		return f, fmt.Errorf("genome has %d bytes, want %d for %d notes", n, want, f.Notes)
	}
	return f, nil
}
