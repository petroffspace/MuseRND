package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestGenerateAndLibrary(t *testing.T) {
	qualities[0].Seconds = 1 // keep the test fast
	srv := New(t.TempDir(), "/dev/urandom", t.TempDir())
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	get := func(path string, v any) int {
		r, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer r.Body.Close()
		if v != nil {
			json.NewDecoder(r.Body).Decode(v)
		}
		return r.StatusCode
	}
	post := func(path, body string, v any) int {
		r, err := http.Post(ts.URL+path, "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer r.Body.Close()
		if v != nil {
			json.NewDecoder(r.Body).Decode(v)
		}
		return r.StatusCode
	}

	var opt map[string]any
	if get("/api/options", &opt) != 200 || len(opt["gm"].([]any)) != 128 {
		t.Fatal("options")
	}
	if post("/api/generate", `{"genre":"polka"}`, nil) != 400 {
		t.Fatal("bad genre accepted")
	}

	var gen struct{ JobID string }
	body := `{"genre":"ambient","mood":"calm","scale":"lydian","tonic":2,"seconds":20,"bpm":80,
		"parts":["chords","bass","drums"],"instruments":{"lead":88,"drums":25},"quality":"quick"}`
	if post("/api/generate", body, &gen) != 200 || gen.JobID == "" {
		t.Fatal("generate")
	}
	var j job
	for i := 0; i < 100; i++ {
		get("/api/jobs/"+gen.JobID, &j)
		if j.Status == "done" || j.Status == "failed" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if j.Status != "done" || j.SongID == "" {
		t.Fatalf("job ended %s %s", j.Status, j.Error)
	}

	var song struct {
		Meta     meta
		Parts    []playPart
		Duration float64
	}
	if get("/api/songs/"+j.SongID, &song) != 200 {
		t.Fatal("song")
	}
	if song.Meta.Key != "D lydian" || song.Meta.BPM != 80 || song.Meta.Bars != 7 {
		t.Errorf("meta %+v", song.Meta)
	}
	if song.Meta.Instruments["lead"].Name != "Pad 1 (new age)" || song.Meta.Instruments["drums"].Name != "TR-808 Kit" {
		t.Errorf("instruments %+v", song.Meta.Instruments)
	}
	if len(song.Parts) != 4 || song.Duration < 20 || song.Duration > 22 {
		t.Errorf("%d parts, %.1f s", len(song.Parts), song.Duration)
	}

	// re-instrument keeps the notes
	var re struct {
		Meta  meta
		Parts []playPart
	}
	if post("/api/songs/"+j.SongID+"/instruments", `{"instruments":{"lead":11}}`, &re) != 200 || re.Meta.Instruments["lead"].Name != "Vibraphone" {
		t.Fatalf("reinstrument %+v", re.Meta.Instruments)
	}
	if len(re.Parts[0].Notes) != len(song.Parts[0].Notes) {
		t.Fatal("re-instrumenting changed the notes")
	}

	for _, f := range []string{"mid", "musicxml", "genome"} {
		r, _ := http.Get(ts.URL + "/api/songs/" + j.SongID + "/download/" + f)
		if r.StatusCode != 200 || !strings.Contains(r.Header.Get("Content-Disposition"), "calm-ambient-d-lydian."+f) {
			t.Errorf("download %s: %d %q", f, r.StatusCode, r.Header.Get("Content-Disposition"))
		}
		r.Body.Close()
	}
	if get("/api/songs/..%2f..%2fetc", nil) == 200 {
		t.Fatal("path traversal accepted")
	}

	// ratings, taste and variations
	type jobView struct {
		Status, SongID, Error string
		Taste                 *tasteUse
	}
	wait := func(jobID string) jobView {
		var j jobView
		for i := 0; i < 100; i++ {
			get("/api/jobs/"+jobID, &j)
			if j.Status == "done" || j.Status == "failed" {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if j.Status != "done" {
			t.Fatalf("job %s: %s %s", jobID, j.Status, j.Error)
		}
		return j
	}
	var rated meta
	if post("/api/songs/"+j.SongID+"/rating", `{"rating":1}`, &rated) != 200 || rated.Rating != 1 {
		t.Fatal("rating")
	}
	if post("/api/songs/"+j.SongID+"/rating", `{"rating":5}`, nil) != 400 {
		t.Fatal("bad rating accepted")
	}
	var taste struct {
		Ready           bool
		Liked, Disliked int
	}
	get("/api/taste?genre=ambient", &taste)
	if taste.Ready || taste.Liked != 1 {
		t.Fatalf("taste with one rating: %+v", taste)
	}

	var v struct{ JobID string }
	if post("/api/songs/"+j.SongID+"/vary", `{"keep":["melody","chords","bass","drums"],"amount":"subtle","quality":"quick"}`, nil) != 400 {
		t.Fatal("variation with everything kept accepted")
	}
	if post("/api/songs/"+j.SongID+"/vary", `{"keep":["melody"],"amount":"fresh","quality":"quick","useTaste":true}`, &v) != 200 {
		t.Fatal("vary")
	}
	vj := wait(v.JobID)
	var vs struct {
		Meta  meta
		Parts []playPart
	}
	get("/api/songs/"+vj.SongID, &vs)
	if vs.Meta.Parent != j.SongID || len(vs.Meta.Kept) != 1 || vs.Meta.Kept[0] != "melody" || !strings.HasSuffix(vs.Meta.Title, "variation") {
		t.Fatalf("variation meta %+v", vs.Meta)
	}
	if len(vs.Parts[0].Notes) != len(re.Parts[0].Notes) {
		t.Fatal("kept melody changed")
	}
	for i, n := range vs.Parts[0].Notes {
		if n != re.Parts[0].Notes[i] {
			t.Fatalf("kept melody note %d changed", i)
		}
	}
	post("/api/songs/"+vj.SongID+"/rating", `{"rating":-1}`, nil)
	get("/api/taste?genre=ambient", &taste)
	if !taste.Ready || taste.Liked != 1 || taste.Disliked != 1 {
		t.Fatalf("taste with two ratings: %+v", taste)
	}
	if post("/api/songs/"+j.SongID+"/vary", `{"keep":["drums"],"amount":"moderate","quality":"quick","useTaste":true}`, &v) != 200 {
		t.Fatal("vary with taste")
	}
	if tj := wait(v.JobID); tj.Taste == nil || tj.Taste.Weight == 0 {
		t.Fatalf("taste not used: %+v", tj.Taste)
	}
	// section lock: melody of bars 2-3 kept, everything else free
	if post("/api/songs/"+j.SongID+"/vary", `{"keep":["melody"],"from":2,"to":99,"amount":"fresh","quality":"quick"}`, nil) != 400 {
		t.Fatal("out-of-range bars accepted")
	}
	if post("/api/songs/"+j.SongID+"/vary", `{"keep":["melody"],"from":2,"to":3,"amount":"fresh","quality":"quick"}`, &v) != 200 {
		t.Fatal("section vary")
	}
	sj := wait(v.JobID)
	var ss struct {
		Meta  meta
		Parts []playPart
	}
	get("/api/songs/"+sj.SongID, &ss)
	if ss.Meta.LockFrom != 2 || ss.Meta.LockTo != 3 || ss.Meta.Key != re.Meta.Key {
		t.Fatalf("section meta %+v (parent key %s)", ss.Meta, re.Meta.Key)
	}
	barSec := 240 / float64(ss.Meta.BPM)
	inBars := func(ns []playNote) []playNote {
		var out []playNote
		for _, n := range ns {
			if n.T < 3*barSec-1e-6 && n.T+n.D > 1*barSec+1e-6 {
				out = append(out, n)
			}
		}
		return out
	}
	a, b := inBars(re.Parts[0].Notes), inBars(ss.Parts[0].Notes)
	if len(a) != len(b) { // (a section may be all rests in a sparse ambient melody)
		t.Fatalf("locked bars: %d notes, want %d", len(b), len(a))
	}
	for i := range a {
		if a[i].P != b[i].P || math.Abs(a[i].T-b[i].T) > 1e-6 || math.Abs(a[i].D-b[i].D) > 1e-6 {
			t.Fatalf("locked note %d changed: %+v -> %+v", i, a[i], b[i])
		}
	}

	// transpose and tempo edits keep notes (shifted) and scores
	var ed struct {
		Meta     meta
		Parts    []playPart
		Duration float64
	}
	if post("/api/songs/"+j.SongID+"/edit", `{"transpose":3,"bpm":150}`, &ed) != 200 {
		t.Fatal("edit")
	}
	if ed.Meta.Transpose != 3 || ed.Meta.BPM != 150 || ed.Meta.Bars != re.Meta.Bars || ed.Meta.Key != "F lydian" {
		t.Fatalf("edited meta: transpose %d bpm %d bars %d key %s", ed.Meta.Transpose, ed.Meta.BPM, ed.Meta.Bars, ed.Meta.Key)
	}
	if math.Abs(ed.Meta.Score.Total-re.Meta.Score.Total) > 1e-9 {
		t.Fatalf("score changed %.3f -> %.3f", re.Meta.Score.Total, ed.Meta.Score.Total)
	}
	for k, n := range re.Parts[0].Notes {
		e := ed.Parts[0].Notes[k]
		if e.P != n.P+3 || math.Abs(e.T-n.T*80/150) > 1e-6 {
			t.Fatalf("note %d: %+v -> %+v", k, n, e)
		}
	}
	if post("/api/songs/"+j.SongID+"/edit", `{"transpose":20}`, nil) != 400 || post("/api/songs/"+j.SongID+"/edit", `{"bpm":5}`, nil) != 400 {
		t.Fatal("bad edit accepted")
	}
	// key change: everything kept in bars 1-3, the rest up 2
	if post("/api/songs/"+j.SongID+"/edit", `{"transpose":2,"transposeLock":{"parts":["melody","chords","bass"],"from":1,"to":3}}`, &ed) != 200 {
		t.Fatal("edit with lock")
	}
	if ed.Meta.Key != re.Meta.Key || ed.Meta.KeyEnd != "E lydian" {
		t.Fatalf("key change: %s -> %s", ed.Meta.Key, ed.Meta.KeyEnd)
	}
	// tempo with locked bars: bars 1-3 keep their current 150 bpm (from the
	// edit above), the rest goes to 160
	var tp struct {
		Meta     meta
		Duration float64
		BarTimes []float64
	}
	if post("/api/songs/"+j.SongID+"/edit", `{"bpm":160,"keepTempo":{"from":1,"to":3}}`, &tp) != 200 {
		t.Fatal("tempo edit with lock")
	}
	nb := float64(tp.Meta.Bars)
	if want := 3*1.6 + (nb-3)*1.5; math.Abs(tp.Duration-want) > 1e-6 || math.Abs(tp.Meta.Seconds-want) > 1e-6 {
		t.Fatalf("duration %.3f / %.3f, want %.3f", tp.Duration, tp.Meta.Seconds, want)
	}
	if ts := tp.Meta.Settings.TempoSection; ts == nil || ts.BPM != 150 || ts.From != 1 || ts.To != 3 || tp.Meta.BPM != 160 {
		t.Fatalf("tempo section %+v, bpm %d", ts, tp.Meta.BPM)
	}
	if math.Abs(tp.BarTimes[3]-4.8) > 1e-6 || math.Abs(tp.BarTimes[4]-6.3) > 1e-6 {
		t.Fatalf("bar times %v", tp.BarTimes[:5])
	}
	if post("/api/songs/"+j.SongID+"/edit", `{"bpm":100,"keepTempo":{"from":1,"to":99}}`, nil) != 400 {
		t.Fatal("out-of-range tempo lock accepted")
	}
	if post("/api/songs/"+j.SongID+"/edit", `{"transpose":2,"transposeLock":{"parts":["drums"]}}`, nil) != 400 {
		t.Fatal("drum lock accepted")
	}
	ed.Meta = meta{}                                                                                // fresh decode: empty fields are omitted from the JSON
	if code := post("/api/songs/"+j.SongID+"/edit", `{"transpose":0,"bpm":80}`, &ed); code != 200 { // back to the original
		t.Fatalf("undo: %d", code)
	}
	if ed.Meta.Key != re.Meta.Key || ed.Meta.KeyEnd != "" || ed.Parts[0].Notes[0] != re.Parts[0].Notes[0] {
		t.Fatalf("undoing the edit did not restore the song: key %q/%q end %q note %+v/%+v", ed.Meta.Key, re.Meta.Key, ed.Meta.KeyEnd, ed.Parts[0].Notes[0], re.Parts[0].Notes[0])
	}

	// instrument lock: melody keeps vibraphone in bars 1-2, becomes a flute
	// from bar 3; chords keep their instrument everywhere
	var il struct {
		Meta     meta
		Parts    []playPart
		BarTimes []float64
	}
	body = `{"instruments":{"lead":73,"chords":0},"keep":{"parts":["melody"],"from":1,"to":2}}`
	if post("/api/songs/"+j.SongID+"/instruments", body, &il) != 200 {
		t.Fatal("instrument lock")
	}
	// every melody note in bars 1-2 is on the vibraphone, every later one on
	// the flute (a sparse melody may have no notes in bars 1-2 at all)
	for _, p := range il.Parts {
		if p.Kind != "lead" {
			continue
		}
		for _, n := range p.Notes {
			inLocked := n.T < il.BarTimes[2]-1e-9
			if inLocked && p.Name != "Vibraphone" || !inLocked && p.Name != "Flute" {
				t.Fatalf("melody note at %.2f s played on %s", n.T, p.Name)
			}
		}
	}
	chordsBefore := il.Meta.Instruments["chords"]
	if post("/api/songs/"+j.SongID+"/instruments", `{"instruments":{"chords":5},"keep":{"parts":["chords"]}}`, &il) != 200 {
		t.Fatal("whole-song keep")
	}
	if il.Meta.Instruments["chords"] != chordsBefore {
		t.Fatalf("kept chords changed: %+v -> %+v", chordsBefore, il.Meta.Instruments["chords"])
	}
	if post("/api/songs/"+j.SongID+"/instruments", `{"instruments":{"lead":1},"keep":{"parts":["melody"],"from":2,"to":99}}`, nil) != 400 {
		t.Fatal("bad instrument lock accepted")
	}
	post("/api/songs/"+j.SongID+"/instruments", `{"instruments":{"lead":11,"chords":`+fmt.Sprint(chordsBefore.Program)+`}}`, &il) // no lock: one instrument per part again

	// re-instrumenting keeps rating and lineage
	var ri struct{ Meta meta }
	post("/api/songs/"+vj.SongID+"/instruments", `{"instruments":{"bass":33}}`, &ri)
	if ri.Meta.Rating != -1 || ri.Meta.Parent != j.SongID {
		t.Fatalf("reinstrument lost meta: %+v", ri.Meta)
	}

	var list []meta
	get("/api/songs", &list)
	if len(list) != 4 {
		t.Fatalf("library has %d songs", len(list))
	}
	req, _ := http.NewRequest("DELETE", ts.URL+"/api/songs/"+j.SongID, nil)
	http.DefaultClient.Do(req)
	get("/api/songs", &list)
	if len(list) != 3 {
		t.Fatal("delete")
	}
}

func TestSoundFonts(t *testing.T) {
	srv := New(t.TempDir(), "/dev/urandom", t.TempDir())
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	upload := func(name string, body []byte) int {
		r, err := http.Post(ts.URL+"/api/soundfonts?name="+url.QueryEscape(name), "application/octet-stream", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		return r.StatusCode
	}
	sf2 := append([]byte("RIFF\x10\x00\x00\x00sfbk"), bytes.Repeat([]byte{7}, 1000)...)
	if c := upload("My Piano (v2).sf2", sf2); c != 200 {
		t.Fatalf("upload: %d", c)
	}
	if upload("kit.dls", append([]byte("RIFF\x04\x00\x00\x00DLS "), 1, 2, 3)) != 200 {
		t.Fatal("dls upload")
	}
	for name, body := range map[string][]byte{
		"x.sf2":       []byte("MThd not a soundfont"), // wrong content
		"../evil.sf2": sf2,                            // path traversal
		"noext":       sf2,                            // wrong extension
		"song.mid":    sf2,
		".hidden.sf2": sf2,
		"tiny.sf2":    []byte("RIFF"),
	} {
		if c := upload(name, body); c != 400 {
			t.Errorf("%q accepted (%d)", name, c)
		}
	}
	var list []soundFont
	r, _ := http.Get(ts.URL + "/api/soundfonts")
	json.NewDecoder(r.Body).Decode(&list)
	r.Body.Close()
	if len(list) != 2 || list[1].Name != "My Piano (v2).sf2" || list[1].Size != int64(len(sf2)) {
		t.Fatalf("list %+v", list)
	}
	r, _ = http.Get(ts.URL + "/api/soundfonts/" + url.PathEscape("My Piano (v2).sf2"))
	got, _ := io.ReadAll(r.Body)
	r.Body.Close()
	if !bytes.Equal(got, sf2) {
		t.Fatal("download differs from upload")
	}
	req, _ := http.NewRequest("DELETE", ts.URL+"/api/soundfonts/kit.dls", nil)
	r, _ = http.DefaultClient.Do(req)
	r.Body.Close()
	r, _ = http.Get(ts.URL + "/api/soundfonts/kit.dls")
	r.Body.Close()
	if r.StatusCode != 404 {
		t.Fatal("deleted SoundFont still served")
	}
}

func TestExtraPartsAPI(t *testing.T) {
	qualities[0].Seconds = 1
	srv := New(t.TempDir(), "/dev/urandom", t.TempDir())
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	post := func(path, body string, v any) int {
		r, err := http.Post(ts.URL+path, "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer r.Body.Close()
		if v != nil {
			json.NewDecoder(r.Body).Decode(v)
		}
		return r.StatusCode
	}
	wait := func(id string) string {
		var j struct{ Status, SongID, Error string }
		for i := 0; i < 100; i++ {
			r, _ := http.Get(ts.URL + "/api/jobs/" + id)
			json.NewDecoder(r.Body).Decode(&j)
			r.Body.Close()
			if j.Status == "done" || j.Status == "failed" {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if j.Status != "done" {
			t.Fatalf("job %s %s", j.Status, j.Error)
		}
		return j.SongID
	}
	var gen struct{ JobID string }
	body := `{"genre":"house","mood":"balanced","scale":"auto","tonic":-1,"seconds":20,"parts":["chords","bass","drums"],
		"instruments":{},"quality":"quick","extras":[{"role":"counter","program":73},{"role":"percussion","program":63}]}`
	if post("/api/generate", body, &gen) != 200 {
		t.Fatal("generate with extras")
	}
	id := wait(gen.JobID)
	type song struct {
		Meta  meta
		Parts []playPart
	}
	var s song
	r, _ := http.Get(ts.URL + "/api/songs/" + id)
	json.NewDecoder(r.Body).Decode(&s)
	r.Body.Close()
	labels := map[string]playPart{}
	for _, p := range s.Parts {
		labels[p.Kind] = p
	}
	if labels["extra1"].Label != "Counter-melody 1" || labels["extra1"].Name != "Flute" || labels["extra2"].Name != "Open Hi Conga" || !labels["extra2"].Drums {
		t.Fatalf("extra parts: %+v / %+v", labels["extra1"], labels["extra2"])
	}
	if s.Meta.Instruments["extra2"].Program != 63 {
		t.Fatalf("meta instruments %+v", s.Meta.Instruments)
	}
	// re-instrument the extras (percussion takes a new sound)
	if post("/api/songs/"+id+"/instruments", `{"instruments":{"extra1":11,"extra2":64}}`, &s) != 200 ||
		s.Meta.Instruments["extra1"].Name != "Vibraphone" || s.Meta.Instruments["extra2"].Name != "Low Conga" {
		t.Fatalf("reinstrument extras: %+v", s.Meta.Instruments)
	}
	// a variation that locks the percussion extra keeps its hits
	if post("/api/songs/"+id+"/vary", `{"keep":["extra2","melody"],"amount":"fresh","quality":"quick"}`, &gen) != 200 {
		t.Fatal("vary")
	}
	vid := wait(gen.JobID)
	var v song
	r, _ = http.Get(ts.URL + "/api/songs/" + vid)
	json.NewDecoder(r.Body).Decode(&v)
	r.Body.Close()
	hits := func(x song) string {
		for _, p := range x.Parts {
			if p.Kind == "extra2" {
				return fmt.Sprint(p.Notes)
			}
		}
		return ""
	}
	if hits(v) == "" || hits(v) != hits(s) {
		t.Fatal("locked percussion extra changed")
	}
}
