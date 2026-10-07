// Package web serves the MUseRND browser UI and its JSON API: generation
// jobs with live progress, a library of saved songs, playback data, MIDI /
// MusicXML / genome downloads and re-instrumentation.
package web

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"musernd/internal/melody"
)

//go:embed static
var static embed.FS

// Search budgets per quality setting. The search stops at the budget or when
// the total score reaches Target, whichever comes first.
var qualities = []struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Seconds int    `json:"seconds"`
}{
	{"quick", "Quick (~15 s)", 15},
	{"normal", "Normal (~45 s)", 45},
	{"best", "Best (~2 min)", 120},
}

// Target is the score at which a search stops early.
const Target = 94

// Server holds the library directory and running jobs.
type Server struct {
	Library    string // directory with one subdirectory per song
	Source     string // random byte source, normally /dev/urandom
	SoundFonts string // directory of uploaded SoundFonts

	mu   sync.Mutex
	jobs map[string]*job
	run  chan struct{} // one search at a time; each already uses every core
}

// New returns a server storing songs under library and uploaded
// SoundFonts under soundFonts.
func New(library, source, soundFonts string) *Server {
	return &Server{Library: library, Source: source, SoundFonts: soundFonts, jobs: map[string]*job{}, run: make(chan struct{}, 1)}
}

type point struct {
	T     float64 `json:"t"` // seconds since start
	Total float64 `json:"total"`
}

type breakdown struct {
	Total   float64 `json:"total"`
	Melody  float64 `json:"melody"`
	Harmony float64 `json:"harmony"`
	Bass    float64 `json:"bass"`
	Drums   float64 `json:"drums"`
	Extras  float64 `json:"extras,omitempty"` // average of the extra parts
	Key     string  `json:"key"`
}

func breakdownOf(c melody.Composition) breakdown {
	ex := 0.0
	for _, v := range c.Extras {
		ex += v * 100 / float64(len(c.Extras))
	}
	return breakdown{c.Total, c.Melody.Total, c.Harmony * 100, c.Bass * 100, c.Drums * 100, ex, c.Melody.KeyName}
}

type job struct {
	ID       string          `json:"id"`
	Settings melody.Settings `json:"settings"`
	Status   string          `json:"status"` // queued, running, done, cancelled, failed
	Gen      int64           `json:"generations"`
	Elapsed  float64         `json:"elapsed"`
	Budget   int             `json:"budget"`
	Best     *breakdown      `json:"best,omitempty"`
	History  []point         `json:"history"`
	SongID   string          `json:"songId,omitempty"`
	Error    string          `json:"error,omitempty"`
	Taste    *tasteUse       `json:"taste,omitempty"`  // learned taste blended in
	Parent   string          `json:"parent,omitempty"` // variation of this song

	useTaste bool
	lock     melody.Lock
	amount   int
	kept     []string
	amountID string
	lockFrom int // bars, 1-based inclusive; 0 = whole song
	lockTo   int

	cancel atomic.Bool
	gen    atomic.Int64
	start  time.Time
}

// Handler returns the HTTP handler for the UI and API.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	sub, _ := fs.Sub(static, "static")
	mux.Handle("GET /", http.FileServerFS(sub))
	mux.HandleFunc("GET /api/options", s.options)
	mux.HandleFunc("POST /api/generate", s.generate)
	mux.HandleFunc("GET /api/jobs/{id}", s.jobStatus)
	mux.HandleFunc("POST /api/jobs/{id}/cancel", s.jobCancel)
	mux.HandleFunc("GET /api/songs", s.songs)
	mux.HandleFunc("GET /api/songs/{id}", s.song)
	mux.HandleFunc("DELETE /api/songs/{id}", s.deleteSong)
	mux.HandleFunc("POST /api/songs/{id}/instruments", s.reinstrument)
	mux.HandleFunc("POST /api/songs/{id}/edit", s.editSong)
	mux.HandleFunc("GET /api/songs/{id}/download/{format}", s.download)
	mux.HandleFunc("POST /api/songs/{id}/vary", s.vary)
	mux.HandleFunc("POST /api/songs/{id}/rating", s.rate)
	mux.HandleFunc("GET /api/taste", s.tasteInfo)
	mux.HandleFunc("GET /api/soundfonts", s.listSoundFonts)
	mux.HandleFunc("POST /api/soundfonts", s.uploadSoundFont)
	mux.HandleFunc("GET /api/soundfonts/{name}", s.getSoundFont)
	mux.HandleFunc("DELETE /api/soundfonts/{name}", s.deleteSoundFont)
	return mux
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func httpError(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

// ---- options ---------------------------------------------------------------

type instrumentChoice struct {
	Program int    `json:"program"`
	Name    string `json:"name"`
}

func (s *Server) options(w http.ResponseWriter, r *http.Request) {
	type genre struct {
		ID          string                      `json:"id"`
		Title       string                      `json:"title"`
		Group       string                      `json:"group"`
		Meter       string                      `json:"meter"`
		BarSteps    int                         `json:"barSteps"`
		BeatSteps   int                         `json:"beatSteps"`
		BPM         int                         `json:"bpm"`
		Swing       float64                     `json:"swing"`
		Instruments map[string]instrumentChoice `json:"instruments"`
		Suggested   map[string][]int            `json:"suggested"`
	}
	var genres []genre
	for _, st := range melody.Styles {
		g := genre{ID: st.Name, Title: st.Title, Group: st.Group, Meter: st.TimeSignature(),
			BarSteps: st.BarSteps(), BeatSteps: st.BeatSteps(), BPM: st.BPM, Swing: st.Swing, Instruments: map[string]instrumentChoice{}, Suggested: melody.Suggested[st.Name]}
		for part, kind := range melody.InstrumentParts {
			p, n := st.Instrument(kind)
			g.Instruments[part] = instrumentChoice{p, n}
		}
		genres = append(genres, g)
	}
	var scales []map[string]string
	for _, sc := range melody.Scales {
		scales = append(scales, map[string]string{"id": sc.ID, "name": sc.Name})
	}
	var kits []instrumentChoice
	for p, n := range melody.DrumKits {
		kits = append(kits, instrumentChoice{p, n})
	}
	sort.Slice(kits, func(i, j int) bool { return kits[i].Program < kits[j].Program })
	writeJSON(w, map[string]any{
		"genres":     genres,
		"moods":      melody.Moods,
		"scales":     scales,
		"keys":       []string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"},
		"gm":         melody.GMNames,
		"drumKits":   kits,
		"qualities":  qualities,
		"roles":      melody.RoleInfo(),
		"percussion": percussionList(),
		"maxParts":   4 + melody.MaxExtras,
		"limits": map[string]int{"minBpm": melody.MinBPM, "maxBpm": melody.MaxBPM,
			"minSeconds": 10, "maxSeconds": 600},
	})
}

// ---- generation ------------------------------------------------------------

func newID(prefix string) string {
	var b [3]byte
	rand.Read(b[:])
	return time.Now().Format("20060102-150405") + "-" + prefix + "-" + hex.EncodeToString(b[:])
}

func (s *Server) generate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		melody.Settings
		UseTaste bool `json:"useTaste"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, err)
		return
	}
	if _, _, _, err := melody.BuildStyle(req.Settings); err != nil {
		httpError(w, http.StatusBadRequest, err)
		return
	}
	j := &job{ID: newID(req.Genre), Settings: req.Settings, useTaste: req.UseTaste}
	s.start(w, j)
}

func budgetFor(quality string) int {
	for _, q := range qualities {
		if q.ID == quality {
			return q.Seconds
		}
	}
	return qualities[1].Seconds
}

// start queues a job and answers with its ID.
func (s *Server) start(w http.ResponseWriter, j *job) {
	j.Status, j.Budget, j.History = "queued", budgetFor(j.Settings.Quality), []point{}
	s.mu.Lock()
	s.jobs[j.ID] = j
	s.mu.Unlock()
	go s.runJob(j)
	writeJSON(w, map[string]string{"jobId": j.ID})
}

// Variation amounts: random changes per starting genome (-1: fresh bytes).
var amounts = map[string]int{"subtle": 4, "moderate": 20, "fresh": -1}

// vary starts a variation of a library song: the kept parts stay exactly as
// they are, the others are re-evolved starting from the song's genome.
func (s *Server) vary(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Keep     []string `json:"keep"`
		From     int      `json:"from"` // locked bars, 1-based inclusive; 0 = whole song
		To       int      `json:"to"`
		Amount   string   `json:"amount"`
		Quality  string   `json:"quality"`
		UseTaste bool     `json:"useTaste"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, err)
		return
	}
	amount, ok := amounts[req.Amount]
	if !ok {
		httpError(w, http.StatusBadRequest, errors.New("amount must be subtle, moderate or fresh"))
		return
	}
	gf, err := s.loadGenome(id)
	if err != nil {
		httpError(w, http.StatusNotFound, err)
		return
	}
	if gf.Settings == nil || gf.Style.Bars == 0 {
		httpError(w, http.StatusBadRequest, errors.New("only songs made in the UI can be varied"))
		return
	}
	bars := gf.Style.Bars
	whole := req.From == 0 && req.To == 0 || req.From <= 1 && req.To >= bars
	if !whole && (req.From < 1 || req.To < req.From || req.To > bars) {
		httpError(w, http.StatusBadRequest, fmt.Errorf("locked bars must be within 1-%d", bars))
		return
	}
	if whole {
		req.From, req.To = 0, 0
	}
	locks := map[string]melody.Lock{}
	present := map[string]bool{"melody": true, "chords": gf.Acc&melody.AccompChords != 0,
		"bass": gf.Acc&melody.AccompBass != 0, "drums": gf.Acc&melody.AccompDrums != 0}
	for _, kind := range gf.Style.PartKinds() {
		name := melody.PartKey(kind)
		if kind == melody.PartLead {
			name = "melody"
		}
		locks[name] = melody.LockOf(kind)
		if kind.IsExtra() {
			present[name] = true
		}
	}
	var lock melody.Lock
	var kept []string
	free := 0
	for part, l := range locks {
		keep := false
		for _, k := range req.Keep {
			keep = keep || k == part
		}
		switch {
		case keep:
			lock |= l
			if present[part] {
				kept = append(kept, part)
			}
		case present[part]:
			free++
		}
	}
	if free == 0 && whole {
		httpError(w, http.StatusBadRequest, errors.New("nothing to vary: unlock at least one part or lock only some bars"))
		return
	}
	sort.Strings(kept)
	set := *gf.Settings
	if req.Quality != "" {
		set.Quality = req.Quality
	}
	// A variation stays in the original's key, so locked chords and bass
	// keep their exact pitches whatever the free melody does.
	_, parent := melody.EvaluateGenome(gf.Genome, gf.Notes, gf.Acc, gf.Style)
	set.Scale, set.Tonic = parent.Scale.ID, parent.Tonic
	j := &job{ID: newID(set.Genre), Settings: set, Parent: id, useTaste: req.UseTaste,
		lock: lock, amount: amount, kept: kept, amountID: req.Amount, lockFrom: req.From, lockTo: req.To}
	s.start(w, j)
}

func (s *Server) runJob(j *job) {
	s.run <- struct{}{}
	defer func() { <-s.run }()
	s.mu.Lock()
	if j.cancel.Load() {
		j.Status = "cancelled"
		s.mu.Unlock()
		return
	}
	j.Status = "running"
	j.start = time.Now()
	s.mu.Unlock()

	err := s.compose(j)

	s.mu.Lock()
	defer s.mu.Unlock()
	j.Elapsed = time.Since(j.start).Seconds()
	j.Gen = j.gen.Load()
	switch {
	case err != nil:
		j.Status, j.Error = "failed", err.Error()
	case j.cancel.Load() && j.SongID == "":
		j.Status = "cancelled"
	default:
		j.Status = "done"
	}
}

func (s *Server) compose(j *job) error {
	st, acc, bpm, err := melody.BuildStyle(j.Settings)
	if err != nil {
		return err
	}
	src, err := os.Open(s.Source)
	if err != nil {
		return err
	}
	defer src.Close()

	notes := melody.NotesFor(st)
	var seed []byte
	parentTitle := ""
	if j.Parent != "" {
		pg, err := s.loadGenome(j.Parent)
		if err != nil {
			return err
		}
		if pg.Notes != notes || len(pg.Genome) != melody.GenomeSize(notes, st) {
			return errors.New("parent genome does not match its settings")
		}
		seed = pg.Genome
		if pm, err := s.loadMeta(j.Parent); err == nil {
			parentTitle = pm.Title
		}
	}
	var taste *melody.Taste
	if j.useTaste {
		t, info := s.learnTaste(j.Settings.Genre)
		if t != nil {
			taste = t
			s.mu.Lock()
			j.Taste = info
			s.mu.Unlock()
		}
	}
	deadline := j.start.Add(time.Duration(j.Budget) * time.Second)
	genome, best, _ := melody.Evolve(melody.NewRand(src), melody.Options{
		Notes: notes, Acc: acc, Style: st, Population: 64, Generations: 1 << 30, Target: Target,
		Seed: seed, Lock: j.lock, Amount: j.amount, Taste: taste, TasteWeight: tasteWeight(taste),
		LockFrom: max(0, j.lockFrom-1), LockTo: j.lockTo,
		Progress: func(gen int, c melody.Composition) {
			b := breakdownOf(c)
			s.mu.Lock()
			j.Best = &b
			j.History = append(j.History, point{time.Since(j.start).Seconds(), c.Total})
			s.mu.Unlock()
		},
		Stop: func() bool {
			j.gen.Add(1)
			return j.cancel.Load() || time.Now().After(deadline)
		},
	})
	if j.cancel.Load() {
		return nil // cancelled: discard
	}
	gf := melody.GenomeFile{Notes: notes, Acc: acc, Style: st, BPM: bpm, Genome: genome, Settings: &j.Settings}
	id := j.ID
	m := meta{Created: time.Now(), Generations: j.gen.Load(), Parent: j.Parent, ParentTitle: parentTitle,
		Kept: j.kept, Amount: j.amountID, Taste: j.Taste, LockFrom: j.lockFrom, LockTo: j.lockTo}
	if taste != nil {
		m.TasteScore = best.Taste * 100
	}
	if err := s.saveSong(id, gf, m); err != nil {
		return err
	}
	s.mu.Lock()
	j.SongID = id
	s.mu.Unlock()
	return nil
}

func (s *Server) jobStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[r.PathValue("id")]
	if !ok {
		httpError(w, http.StatusNotFound, errors.New("no such job"))
		return
	}
	if j.Status == "running" {
		j.Elapsed = time.Since(j.start).Seconds()
		j.Gen = j.gen.Load()
	}
	writeJSON(w, j)
}

func (s *Server) jobCancel(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	j, ok := s.jobs[r.PathValue("id")]
	s.mu.Unlock()
	if !ok {
		httpError(w, http.StatusNotFound, errors.New("no such job"))
		return
	}
	j.cancel.Store(true)
	writeJSON(w, map[string]bool{"ok": true})
}

// ---- library ---------------------------------------------------------------

// meta is a song's summary, stored as meta.json next to its files.
type meta struct {
	ID          string                      `json:"id"`
	Title       string                      `json:"title"`
	Created     time.Time                   `json:"created"`
	Settings    melody.Settings             `json:"settings"`
	BPM         int                         `json:"bpm"`
	Bars        int                         `json:"bars"`
	Seconds     float64                     `json:"seconds"`
	Key         string                      `json:"key"`
	Score       breakdown                   `json:"score"`
	Chords      []string                    `json:"chords"`
	Instruments map[string]instrumentChoice `json:"instruments"`
	Generations int64                       `json:"generations"`

	Rating      int       `json:"rating"`                // -1 dislike, 0 none, 1 like
	Features    []float64 `json:"features,omitempty"`    // for taste learning
	Parent      string    `json:"parent,omitempty"`      // variation of this song
	ParentTitle string    `json:"parentTitle,omitempty"` //
	Kept        []string  `json:"kept,omitempty"`        // parts kept from the parent
	OrigBPM     int       `json:"origBpm"`               // tempo it was generated at
	Meter       string    `json:"meter"`                 // time signature, e.g. "6/8"
	Transpose   int       `json:"transpose"`             // semitones applied when written out
	KeyEnd      string    `json:"keyEnd,omitempty"`      // key at the end, after a key change
	BaseTonic   int       `json:"baseTonic"`             // key before transposing (pitch class)
	ScaleName   string    `json:"scaleName"`             //
	LockFrom    int       `json:"lockFrom,omitempty"`    // ... in these bars (1-based, inclusive;
	LockTo      int       `json:"lockTo,omitempty"`      // 0 = whole song)
	Amount      string    `json:"amount,omitempty"`      // subtle, moderate, fresh
	Taste       *tasteUse `json:"taste,omitempty"`       // learned taste used in the search
	TasteScore  float64   `json:"tasteScore,omitempty"`  // how well it matched (0..100)
}

// tasteUse describes a taste model blended into a search.
type tasteUse struct {
	Weight   float64       `json:"weight"`
	Liked    int           `json:"liked"`
	Disliked int           `json:"disliked"`
	Prefs    []melody.Pref `json:"prefs"`
}

var (
	validID    = regexp.MustCompile(`^[0-9]{8}-[0-9]{6}-[a-z]+-[0-9a-f]{6}$`)
	unsafeName = regexp.MustCompile(`[^a-z0-9]+`)
)

func (s *Server) songDir(id string) (string, error) {
	if !validID.MatchString(id) {
		return "", errors.New("bad song id")
	}
	return filepath.Join(s.Library, id), nil
}

// saveSong writes song.genome, song.mid, song.musicxml and meta.json.
// m carries the fields that are not computed here (created, generations,
// rating, lineage, taste).
func (s *Server) saveSong(id string, gf melody.GenomeFile, m meta) error {
	dir, err := s.songDir(id)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	c, scored := melody.EvaluateGenome(gf.Genome, gf.Notes, gf.Acc, gf.Style)
	arr := scored.Output()
	m.ID, m.Settings, m.BPM, m.Bars = id, *gf.Settings, gf.BPM, arr.Bars
	m.Transpose, m.BaseTonic, m.ScaleName = gf.Style.Transpose, scored.Tonic, scored.Scale.Name
	m.Meter = gf.Style.TimeSignature()
	if m.OrigBPM == 0 {
		m.OrigBPM = gf.BPM
	}
	m.Seconds = gf.Style.NewTimeline(arr.Bars, gf.BPM).At(arr.Steps)
	m.Key, m.Score = arr.KeyName(), breakdownOf(c)
	m.KeyEnd = ""
	if last := arr.TonicAt(arr.Bars - 1); last != arr.Tonic {
		m.KeyEnd = melody.KeyName(last, arr.Scale)
	}
	m.Instruments, m.Features, m.Chords = map[string]instrumentChoice{}, c.Features, nil
	mood := gf.Settings.Mood
	if mood == "" {
		mood = "balanced"
	}
	m.Title = fmt.Sprintf("%s%s %s · %s", strings.ToUpper(mood[:1]), mood[1:], gf.Settings.Genre, m.Key)
	if m.Parent != "" {
		m.Title += " · variation"
	}
	for _, ch := range arr.Chords {
		m.Chords = append(m.Chords, ch.Name)
	}
	for _, kind := range gf.Style.PartKinds() {
		_, n := gf.Style.Instrument(kind)
		m.Instruments[melody.PartKey(kind)] = instrumentChoice{gf.Style.PartProgram(kind), n}
	}
	files := map[string][]byte{
		"song.genome":   melody.MarshalGenome(gf),
		"song.mid":      melody.MIDI(arr, gf.BPM, m.Title),
		"song.musicxml": melody.MusicXML(arr, gf.BPM, m.Title),
	}
	mj, _ := json.MarshalIndent(m, "", "  ")
	files["meta.json"] = mj
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) loadMeta(id string) (meta, error) {
	var m meta
	dir, err := s.songDir(id)
	if err != nil {
		return m, err
	}
	b, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil {
		return m, err
	}
	return m, json.Unmarshal(b, &m)
}

func (s *Server) songs(w http.ResponseWriter, r *http.Request) {
	entries, _ := os.ReadDir(s.Library)
	list := []meta{}
	for _, e := range entries {
		if m, err := s.loadMeta(e.Name()); err == nil {
			list = append(list, m)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Created.After(list[j].Created) })
	writeJSON(w, list)
}

func (s *Server) loadGenome(id string) (melody.GenomeFile, error) {
	dir, err := s.songDir(id)
	if err != nil {
		return melody.GenomeFile{}, err
	}
	b, err := os.ReadFile(filepath.Join(dir, "song.genome"))
	if err != nil {
		return melody.GenomeFile{}, err
	}
	return melody.UnmarshalGenome(b)
}

// Playback data: every note with times in seconds (swing applied).
type playNote struct {
	T float64 `json:"t"`
	D float64 `json:"d"`
	P int     `json:"p"`
	V int     `json:"v"` // MIDI velocity 1..127
}

type playPart struct {
	Kind    string     `json:"kind"`  // lead, chords, bass, drums, extra1..extra12
	Label   string     `json:"label"` // display name, e.g. "Arpeggio 1"
	Role    string     `json:"role,omitempty"`
	Channel int        `json:"channel"` // MIDI channel (0-based) in the exported file
	Name    string     `json:"name"`
	Program int        `json:"program"`
	Drums   bool       `json:"drums"`
	Notes   []playNote `json:"notes"`
}

func (s *Server) song(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	m, err := s.loadMeta(id)
	if err != nil {
		httpError(w, http.StatusNotFound, err)
		return
	}
	gf, err := s.loadGenome(id)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err)
		return
	}
	_, scored := melody.EvaluateGenome(gf.Genome, gf.Notes, gf.Acc, gf.Style)
	arr := scored.Output()
	// fields that songs saved by older versions lack
	m.Transpose, m.BaseTonic, m.ScaleName = gf.Style.Transpose, scored.Tonic, scored.Scale.Name
	if m.OrigBPM == 0 {
		m.OrigBPM = m.BPM
	}
	tl := gf.Style.NewTimeline(arr.Bars, gf.BPM)
	at := tl.At

	var parts []playPart
	for _, p := range arr.Parts {
		// one play part per instrument the part uses (an instrument lock
		// can switch it mid-song)
		byIns := map[bool]*playPart{}
		var order []bool
		for _, e := range p.Events {
			prog, name, alt := gf.Style.InstrumentAtBar(p.Kind, e.Start/gf.Style.BarSteps())
			pp := byIns[alt]
			if pp == nil {
				pp = &playPart{Kind: melody.PartKey(p.Kind), Label: p.Name, Role: gf.Style.ExtraRole(p.Kind),
					Name: name, Program: prog, Drums: gf.Style.IsPerc(p.Kind), Channel: gf.Style.Channel(p.Kind)}
				byIns[alt] = pp
				order = append(order, alt)
			}
			t := at(e.Start)
			pp.Notes = append(pp.Notes, playNote{t, at(e.Start+e.Steps) - t, e.Pitch, max(1, min(127, e.Velocity*127/255))})
		}
		for _, alt := range order {
			parts = append(parts, *byIns[alt])
		}
	}
	type barChord struct {
		T    float64 `json:"t"`
		Name string  `json:"name"`
	}
	var chords []barChord
	for b, ch := range arr.Chords {
		chords = append(chords, barChord{at(b * gf.Style.BarSteps()), ch.Name})
	}
	writeJSON(w, map[string]any{"meta": m, "parts": parts, "chords": chords, "duration": at(arr.Steps), "barTimes": tl.BarStarts()})
}

func (s *Server) deleteSong(w http.ResponseWriter, r *http.Request) {
	dir, err := s.songDir(r.PathValue("id"))
	if err != nil {
		httpError(w, http.StatusBadRequest, err)
		return
	}
	if err := os.RemoveAll(dir); err != nil {
		httpError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// reinstrument changes a song's instruments and re-renders its files. The
// notes are unchanged: instruments are not part of the score.
func (s *Server) reinstrument(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Instruments map[string]int `json:"instruments"` // part (lead, chords, bass, drums) -> program
		// Keep: parts (melody, chords, bass, drums) that keep their current
		// instrument in bars From..To (1-based; 0 = whole song).
		Keep *struct {
			Parts    []string `json:"parts"`
			From, To int
		} `json:"keep"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, err)
		return
	}
	gf, err := s.loadGenome(id)
	if err != nil {
		httpError(w, http.StatusNotFound, err)
		return
	}
	m, err := s.loadMeta(id)
	if err != nil {
		httpError(w, http.StatusNotFound, err)
		return
	}
	if gf.Settings == nil {
		httpError(w, http.StatusBadRequest, errors.New("song has no settings"))
		return
	}
	set := *gf.Settings
	set.Instruments = map[string]int{}
	for k, v := range gf.Settings.Instruments {
		set.Instruments[k] = v
	}
	for k, v := range req.Instruments {
		set.Instruments[k] = v
	}
	bars := gf.Style.Bars
	switch k := req.Keep; {
	case k == nil || len(k.Parts) == 0:
		set.InstrumentLock = nil // new instruments everywhere
	case k.From == 0 && k.To == 0 || k.From <= 1 && k.To >= bars:
		// whole song: locked parts keep their instrument; an existing
		// mid-song change of theirs stays as it is
		for _, p := range k.Parts {
			kind, ok := melody.KindOf(p)
			if !ok {
				httpError(w, http.StatusBadRequest, fmt.Errorf("unknown part %q", p))
				return
			}
			set.Instruments[melody.PartKey(kind)] = gf.Style.PartProgram(kind) // the main instrument
		}
	default:
		if k.From < 1 || k.To < k.From || k.To > bars {
			httpError(w, http.StatusBadRequest, fmt.Errorf("bars %d-%d out of range 1-%d", k.From, k.To, bars))
			return
		}
		lock := &melody.InstrumentLock{Parts: k.Parts, From: k.From, To: k.To, Instruments: map[string]int{}}
		for _, p := range k.Parts {
			kind, ok := melody.KindOf(p)
			if !ok {
				httpError(w, http.StatusBadRequest, fmt.Errorf("unknown part %q", p))
				return
			}
			// the instrument the part plays now at the start of the range
			lock.Instruments[p], _, _ = gf.Style.InstrumentAtBar(kind, k.From-1)
		}
		set.InstrumentLock = lock
	}
	st, acc, bpm, err := melody.BuildStyle(set)
	if err != nil {
		httpError(w, http.StatusBadRequest, err)
		return
	}
	gf.Style, gf.Acc, gf.BPM, gf.Settings = st, acc, bpm, &set
	if err := s.saveSong(id, gf, m); err != nil { // m keeps created, rating, lineage
		httpError(w, http.StatusInternalServerError, err)
		return
	}
	s.song(w, r)
}

func (s *Server) download(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	dir, err := s.songDir(id)
	if err != nil {
		httpError(w, http.StatusBadRequest, err)
		return
	}
	types := map[string]string{
		"mid":      "audio/midi",
		"musicxml": "application/vnd.recordare.musicxml+xml",
		"genome":   "application/octet-stream",
	}
	format := r.PathValue("format")
	ct, ok := types[format]
	if !ok {
		httpError(w, http.StatusBadRequest, errors.New("format must be mid, musicxml or genome"))
		return
	}
	m, err := s.loadMeta(id)
	if err != nil {
		httpError(w, http.StatusNotFound, err)
		return
	}
	name := strings.Trim(unsafeName.ReplaceAllString(strings.ReplaceAll(strings.ToLower(m.Title), "#", "sharp"), "-"), "-")
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.%s"`, name, format))
	http.ServeFile(w, r, filepath.Join(dir, "song."+format))
}

// TasteWeight is the most a learned taste counts in the search total (with
// six or more ratings; less with fewer, see melody.Taste.Weight).
const TasteWeight = 0.25

func tasteWeight(t *melody.Taste) float64 {
	if t == nil {
		return 0
	}
	return t.Weight(TasteWeight)
}

// learnTaste builds a taste model from the rated songs of a genre.
func (s *Server) learnTaste(genre string) (*melody.Taste, *tasteUse) {
	entries, _ := os.ReadDir(s.Library)
	var liked, disliked [][]float64
	for _, e := range entries {
		m, err := s.loadMeta(e.Name())
		if err != nil || m.Rating == 0 || m.Settings.Genre != genre {
			continue
		}
		f := m.Features
		if len(f) != len(melody.FeatureNames) { // older song: measure it now
			gf, err := s.loadGenome(m.ID)
			if err != nil {
				continue
			}
			c, _ := melody.EvaluateGenome(gf.Genome, gf.Notes, gf.Acc, gf.Style)
			f = c.Features
		}
		if m.Rating > 0 {
			liked = append(liked, f)
		} else {
			disliked = append(disliked, f)
		}
	}
	t, err := melody.LearnTaste(liked, disliked)
	if err != nil {
		return nil, &tasteUse{Liked: len(liked), Disliked: len(disliked)}
	}
	return t, &tasteUse{Weight: t.Weight(TasteWeight), Liked: len(liked), Disliked: len(disliked), Prefs: t.Prefs(4)}
}

func (s *Server) tasteInfo(w http.ResponseWriter, r *http.Request) {
	t, info := s.learnTaste(r.URL.Query().Get("genre"))
	writeJSON(w, map[string]any{"ready": t != nil, "liked": info.Liked, "disliked": info.Disliked,
		"prefs": info.Prefs, "weight": info.Weight, "maxWeight": TasteWeight})
}

// rate stores a like (1), dislike (-1) or no rating (0).
func (s *Server) rate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Rating int `json:"rating"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256)).Decode(&req); err != nil || req.Rating < -1 || req.Rating > 1 {
		httpError(w, http.StatusBadRequest, errors.New("rating must be -1, 0 or 1"))
		return
	}
	m, err := s.loadMeta(id)
	if err != nil {
		httpError(w, http.StatusNotFound, err)
		return
	}
	m.Rating = req.Rating
	if err := s.writeMeta(m); err != nil {
		httpError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, m)
}

func (s *Server) writeMeta(m meta) error {
	dir, err := s.songDir(m.ID)
	if err != nil {
		return err
	}
	mj, _ := json.MarshalIndent(m, "", "  ")
	return os.WriteFile(filepath.Join(dir, "meta.json"), mj, 0o644)
}

// editSong changes a song's transposition and/or tempo and re-renders it.
// The notes and scores are unchanged: the bars are pinned, so the genome
// still fits at the new tempo.
func (s *Server) editSong(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Transpose     *int                  `json:"transpose"` // semitones from the generated key
		TransposeLock *melody.TransposeLock `json:"transposeLock"`
		BPM           *int                  `json:"bpm"`
		// KeepTempo: bars (1-based, inclusive) that keep their current tempo
		// when BPM changes; the rest of the song takes the new tempo.
		KeepTempo *struct{ From, To int } `json:"keepTempo"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256)).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, err)
		return
	}
	gf, err := s.loadGenome(id)
	if err != nil {
		httpError(w, http.StatusNotFound, err)
		return
	}
	m, err := s.loadMeta(id)
	if err != nil {
		httpError(w, http.StatusNotFound, err)
		return
	}
	if gf.Settings == nil || gf.Style.Bars == 0 {
		httpError(w, http.StatusBadRequest, errors.New("only songs made in the UI can be edited"))
		return
	}
	set := *gf.Settings
	set.Bars = gf.Style.Bars // keep the length in bars whatever the tempo
	if req.Transpose != nil {
		set.Transpose = *req.Transpose
		set.TransposeLock = req.TransposeLock // nil or no parts: transpose everything
		if tl := set.TransposeLock; tl != nil && len(tl.Parts) == 0 {
			set.TransposeLock = nil
		}
	}
	if req.BPM != nil {
		set.TempoSection = nil
		if k := req.KeepTempo; k != nil {
			if k.From < 1 || k.To < k.From || k.To > gf.Style.Bars {
				httpError(w, http.StatusBadRequest, fmt.Errorf("bars %d-%d out of range 1-%d", k.From, k.To, gf.Style.Bars))
				return
			}
			if k.From == 1 && k.To == gf.Style.Bars {
				httpError(w, http.StatusBadRequest, errors.New("all bars are locked: nothing would change"))
				return
			}
			set.TempoSection = &melody.TempoSection{From: k.From, To: k.To, BPM: gf.Style.BarBPM(k.From-1, gf.BPM)}
		}
		set.BPM = *req.BPM
	}
	st, acc, bpm, err := melody.BuildStyle(set)
	if err != nil {
		httpError(w, http.StatusBadRequest, err)
		return
	}
	set.Seconds = int(math.Round(st.NewTimeline(st.Bars, bpm).At(st.Bars * st.BarSteps())))
	gf.Style, gf.Acc, gf.BPM, gf.Settings = st, acc, bpm, &set
	if err := s.saveSong(id, gf, m); err != nil {
		httpError(w, http.StatusInternalServerError, err)
		return
	}
	s.song(w, r)
}

// percussionList is the GM percussion sounds for the UI, by key.
func percussionList() []instrumentChoice {
	var out []instrumentChoice
	for k, n := range melody.GMPercussion {
		out = append(out, instrumentChoice{k, n})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Program < out[j].Program })
	return out
}
