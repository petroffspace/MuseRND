# MUseRND

Music generator: every instrument comes from `/dev/urandom`. A genome holds
melody genes (3 bytes per note) plus per-bar genes for chords, bass and drums
(see the layout comment in `internal/melody/arrange.go`). Genomes are evolved
until a music-theory detector rates the whole arrangement as pleasant for
a **genre** (`style.go`, `genres.go`: 20 presets in groups Original, Band,
Electronic, World, 3/4 & 6/8), then exported.
A style changes gene decoding (note lengths, chord vocabulary, chord rhythms,
drum sounds, instruments, swing) and the detectors (scale, harmony model,
bass role, reference drum grooves, part weights) — never adds fixed notes.
Go, standard library only.

- Build: `go build -o musernd .` — tests: `go test ./...`
- Run: `./musernd generate -style blues -count 10 EXPERIMENTS/NNN`
- Each song is saved as `.genome` (the evolved bytes + notes/accomp/bpm,
  format in `genomefile.go`) next to its `.mid` and `.musicxml`.
  `./musernd render EXPERIMENTS/NNN/*.genome` rebuilds the scores after code
  changes; `./musernd score song.genome` shows the full verdict.
- Code: `internal/melody` — style (genre presets), notes (melody genes),
  arrange (genome layout and decoding of all parts), score (melody
  detector), compose (style-specific accompaniment detectors + total), evolve
  (search; scoring runs in parallel), harmony (triads, 7ths, blues
  dominants), midi (swing timing), musicxml, genomefile (v2 stores style).
- Nothing is rule-generated: chords, bass and drums are decoded from genes;
  musical rules live only in the detectors that score them.
- Output is ONLY the genome plus Standard MIDI (`.mid`) and uncompressed
  MusicXML (`.musicxml`). No BMP or WAV generation — that code was removed on purpose
  (2026-10-06); do not re-add it.

## Web UI
- `./musernd serve -addr :9090 -library LIBRARY` — browser UI (embedded
  static files in `internal/web/static`, API in `internal/web/server.go`).
- Inputs: genre, style (= mood: balanced/calm/energetic/dark/bright), scale
  (auto or 12 scales/modes), key, tempo (auto = genre x mood), duration in
  seconds (exact whole bars), parts, per-part GM instrument / drum kit with
  dice (suggested or any), search quality (15/45/120 s budget, stops at 94).
- Saving is MIDI + MusicXML (+ genome) only — the user explicitly does NOT
  want WAV/MP3. In-browser playback only: FluidR3_GM soundfonts via
  soundfont-player (CDN) and Web-Audio-synthesized drums.
- Songs live in `LIBRARY/<id>/` (song.genome v3 with Settings, song.mid,
  song.musicxml, meta.json). Re-instrumenting re-renders without changing notes.
- Variations (`POST /api/songs/{id}/vary`): search seeded from a song's
  genome; kept parts are locked (`melody.Lock`) and never mutated; amount
  subtle/moderate = random changes per starting genome, fresh = new random
  bytes for unlocked parts. Meta records parent, kept parts, amount.
- Locks can cover a bar range (`Options.LockFrom/LockTo`; UI: drag on the
  piano roll): bar genes in the range are never touched; a partly locked
  melody is protected by re-decoding after each change and undoing changes
  that move or alter notes in the range (`locker.melodyOK`). Variations
  inherit the parent's key/scale so locked chords and bass keep their pitches.
- Edits after generation (`POST /api/songs/{id}/edit`): transpose ±12
  (`Settings.Transpose`, applied only when written out via
  `Arrangement.Transposed`, so scores never change) and tempo (`Settings.BPM`
  with `Settings.Bars` pinned so the genome still fits). In place; meta keeps
  `origBpm` for reset.
- Transpose lock (`Settings.TransposeLock`: parts melody/chords/bass + bar
  range): those parts keep their pitch in those bars. The key follows the
  harmony part bar by bar (`Arrangement.BarTonic`), so locking everything in
  the first bars = a key change, written as key-signature changes in
  MusicXML and MIDI. Always render via `Arrangement.Output()`.
- Tempo lock (`Settings.TempoSection`): a bar range with its own tempo; the
  edit API's `keepTempo` gives the selected bars the tempo they have now and
  the rest the new BPM (one section at a time). `Style.NewTimeline` maps
  sixteenths to seconds (playback, durations, `barTimes`); MIDI/MusicXML get
  tempo-change events/markings.
- Instrument lock (`Settings.InstrumentLock`: parts + bars + the programs
  they keep): those parts play other instruments in that range (one section
  at a time) — MIDI program changes, a second MusicXML score-instrument
  (`-I2` / drum ids `…b`) referenced per note, and separate playback parts.
  Whole-song keep just leaves those parts' instruments unchanged.
- Browser player: one gain per part kind (mutes), one soundfont per play
  part, cached by kind+program for the session (soundfont CDN can be slow).
- User SoundFonts: `serve -soundfonts SOUNDFONTS` stores uploaded
  .sf2/.sf3/.sfogg/.dls (header-checked, `internal/web/soundfonts.go`). When
  one is selected, playback switches engines: SpessaSynth (vendored, Apache-2.0,
  `static/vendor/`, bundled with esbuild — see vendor/README.txt) plays the
  song's exported MIDI with that SoundFont, incl. real drum kits; mutes use
  channel `isMuted` (lead 0, chords 1, bass 2, drums 9).
- The UI has one shared "Selected bars" range (drag on the piano roll) used
  by both the transpose lock and the variation lock.
- Ratings (👍/👎 in meta.json) train `melody.Taste` per genre from the 10
  *character* features only (never quality scores, so dislikes can't make
  music worse). Blended into the search total when "Use my ratings" is on:
  25% max, ramping with the number of ratings (full at 6).

## Genres, meters, moods
- Genre presets: `Styles` (IDs must equal their index; stored in v2 genome
  files). Genre-specific scoring: `Transitions` (jazz/bossa ii-V-I),
  `AutoScale` (afrobeat dorian), `ModeBias`, bass kinds incl. `BassSyncopated`
  and `BassDownbeat`, reference `Grooves`, `ChordRhythms`, `DrumVolume`.
- Meters: `Style.Meter` {4,4}/{3,4}/{6,8}; always use `BarSteps()`,
  `BeatSteps()` (6 in 6/8 — tempo counts dotted quarters), `Strong()`,
  `QuarterBPM()` — never assume 16 steps per bar (`StepsPerBar` = 4/4 and the
  16-bit gene capacity). 12-step bars use the low 12 bits of drum masks.
- Moods: `MoodSliders` (energy, darkness, density, swing, complexity, each
  -1..1) stored in `Settings.Sliders`; presets in `Moods`. Settings without
  sliders naming an old mood (balanced/calm/energetic/dark/bright) keep the
  legacy behaviour so saved songs rebuild identically. Swing is continuous
  (0 straight .. 1 = 2:1).
- Search: a weak accompaniment part (< 0.6) costs points (`weakPenalty`), and
  a "spread this bar's pattern" mutation helps grooves converge.

## Extra parts
- Up to 12 extra parts (16 instruments total) via `Settings.Extras`
  (`internal/melody/extras.go`): roles counter, arpeggio, pad, stabs,
  percussion. Genes: `ExtraGenes` (16) bytes per bar per extra, appended
  after the bar genes, so songs without extras keep their genome layout.
- Part kinds: `ExtraKind(i)` = `PartExtra+i`; names extra1..extra12
  (`PartKey`/`KindOf`); lock bit `LockOf(kind)` (bits 4..15). Pitched extras
  use MIDI channels 4-9 and 11-16 (0-based 3-8, 10-15); percussion extras
  share channel 10 and never send program/volume there (`Style.IsPerc`).
- Scored per role (`scoreExtras`), 8% each, at most 30% together.

## Arrangement rules (user feedback, 2026-10-07)
- The user disliked melody and chords sounding in unison. Keep chords in the
  background: voiced closely *below* the bar's lowest melody note (top note
  >= a minor third under it, E3..G4), never doubling the melody's pitch class
  at an attack, soft (`ChordVelocity`), sustained pad/strings for pop.
  The melody range starts at A3. The detector's "above chords" and "fills
  gaps" checks (`scoreSeparation`) reward keeping the parts apart.

## Folder conventions
- Experiments go in `EXPERIMENTS/NNN/` (001, 002, …), each with a short
  `README.md` of what was tried and the outcome. Use the next free number.
- `rpi/` is only for optional Raspberry Pi 5 (linux/arm64) binary builds —
  not a main target, never for generated content:
  `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o rpi/musernd .`
- The WAV<->BMP lossless converter lives separately in `/home/alex/Claude/music_visualiser`.
