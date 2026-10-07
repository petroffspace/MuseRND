package web

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// User SoundFonts: uploaded SF2/SF3/SFOGG/DLS files the browser player can
// use instead of the default online FluidR3 sounds.

// MaxSoundFont is the largest SoundFont accepted for upload.
const MaxSoundFont = 2 << 30

var soundFontName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._()+-]{0,120}\.(sf2|sf3|sfogg|dls)$`)

type soundFont struct {
	Name     string    `json:"name"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
}

func (s *Server) soundFontPath(name string) (string, error) {
	if !soundFontName.MatchString(name) || strings.Contains(name, "..") {
		return "", errors.New("SoundFont names must end in .sf2, .sf3, .sfogg or .dls and use letters, digits, spaces, . _ - ( ) +")
	}
	return filepath.Join(s.SoundFonts, name), nil
}

func (s *Server) listSoundFonts(w http.ResponseWriter, r *http.Request) {
	entries, _ := os.ReadDir(s.SoundFonts)
	list := []soundFont{}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || !info.Mode().IsRegular() || !soundFontName.MatchString(e.Name()) {
			continue
		}
		list = append(list, soundFont{e.Name(), info.Size(), info.ModTime()})
	}
	sort.Slice(list, func(i, j int) bool { return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name) })
	writeJSON(w, list)
}

// uploadSoundFont stores the request body as SOUNDFONTS/<name>. The body is
// the raw file (no multipart), so large files stream straight to disk.
func (s *Server) uploadSoundFont(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	path, err := s.soundFontPath(name)
	if err != nil {
		httpError(w, http.StatusBadRequest, err)
		return
	}
	if err := os.MkdirAll(s.SoundFonts, 0o755); err != nil {
		httpError(w, http.StatusInternalServerError, err)
		return
	}
	body := http.MaxBytesReader(w, r.Body, MaxSoundFont)
	head := make([]byte, 12)
	if _, err := io.ReadFull(body, head); err != nil {
		httpError(w, http.StatusBadRequest, errors.New("file too short to be a SoundFont"))
		return
	}
	if string(head[0:4]) != "RIFF" || (string(head[8:12]) != "sfbk" && string(head[8:12]) != "DLS ") {
		httpError(w, http.StatusBadRequest, errors.New("not a SoundFont: expected an SF2/SF3/SFOGG (RIFF sfbk) or DLS (RIFF DLS) file"))
		return
	}
	tmp, err := os.CreateTemp(s.SoundFonts, ".upload-*")
	if err != nil {
		httpError(w, http.StatusInternalServerError, err)
		return
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	n, err := tmp.Write(head)
	if err == nil {
		var m int64
		m, err = io.Copy(tmp, body)
		n += int(m)
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		httpError(w, http.StatusBadRequest, fmt.Errorf("upload failed: %w", err))
		return
	}
	os.Chmod(tmp.Name(), 0o644)
	if err := os.Rename(tmp.Name(), path); err != nil {
		httpError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, soundFont{name, int64(n), time.Now()})
}

func (s *Server) getSoundFont(w http.ResponseWriter, r *http.Request) {
	path, err := s.soundFontPath(r.PathValue("name"))
	if err != nil {
		httpError(w, http.StatusBadRequest, err)
		return
	}
	if _, err := os.Stat(path); err != nil {
		httpError(w, http.StatusNotFound, errors.New("no such SoundFont"))
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeFile(w, r, path) // supports caching and ranges
}

func (s *Server) deleteSoundFont(w http.ResponseWriter, r *http.Request) {
	path, err := s.soundFontPath(r.PathValue("name"))
	if err != nil {
		httpError(w, http.StatusBadRequest, err)
		return
	}
	if err := os.Remove(path); err != nil {
		httpError(w, http.StatusNotFound, errors.New("no such SoundFont"))
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}
