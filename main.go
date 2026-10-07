// Command musernd composes music from /dev/urandom.
//
//	musernd generate [-style pop|blues|house|ambient] [flags] DIR
//	musernd render [-bpm N] song.genome...
//	musernd score song.genome
//	musernd serve [-addr :9090] [-library LIBRARY] [-soundfonts SOUNDFONTS]
//
// generate evolves random genomes from /dev/urandom — melody, chords, bass
// and drums alike (see internal/melody) — until a music-theory detector
// scores the whole arrangement as pleasant for the chosen style (genre:
// note lengths, chord vocabulary, grooves, instruments, what is rewarded),
// and writes each composition as
// its genome (.genome), Standard MIDI (.mid) and uncompressed MusicXML
// (.musicxml) into DIR. Scores and chord progressions are printed and
// appended to DIR/generate.log. render rebuilds .mid and .musicxml from saved
// genomes (e.g. after code changes); score shows a genome's full verdict;
// serve runs the web UI.
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"musernd/internal/melody"
	"musernd/internal/web"
)

func usage() {
	fmt.Fprintf(os.Stderr, `Usage:
  %[1]s generate [-style GENRE] [-count N] [-start N] [-name PREFIX] [-notes N] [-target S] [-generations N]
            [-attempts N] [-population N] [-bpm N] [-source FILE]
            [-accomp all|none|bass,chords,drums] DIR
  %[1]s render [-bpm N] song.genome...
  %[1]s score song.genome
  %[1]s serve [-addr :9090] [-library LIBRARY] [-soundfonts SOUNDFONTS]

Genres: %[2]s
`, os.Args[0], genreNames())
	os.Exit(2)
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	cmds := map[string]func([]string) error{
		"generate": generate,
		"render":   render,
		"score":    score,
		"serve":    serve,
	}
	cmd, ok := cmds[os.Args[1]]
	if !ok {
		usage()
	}
	if err := cmd(os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func generate(args []string) error {
	fs := flag.NewFlagSet("generate", flag.ExitOnError)
	styleName := fs.String("style", "pop", "genre: "+genreNames())
	notes := fs.Int("notes", 0, "melody length in notes (0 = the style's default)")
	target := fs.Float64("target", 90, "stop when the total score (melody + accompaniment) reaches this (0-100)")
	gens := fs.Int("generations", 30000, "maximum number of generations per attempt")
	attempts := fs.Int("attempts", 3, "restart from fresh random genomes up to this many times if the target is missed")
	popSize := fs.Int("population", 64, "genomes per generation")
	count := fs.Int("count", 1, "number of compositions to generate")
	start := fs.Int("start", 1, "number of the first composition (to continue a series)")
	name := fs.String("name", "melody", "file name prefix")
	bpm := fs.Int("bpm", 0, "tempo (0 = the style's default)")
	source := fs.String("source", "/dev/urandom", "random byte source")
	accomp := fs.String("accomp", "all", "accompaniment: all, none, or a list of bass,chords,drums")
	fs.Parse(args)
	st, err := melody.StyleByName(*styleName)
	if err != nil {
		return err
	}
	acc, err := melody.ParseAccompaniment(*accomp)
	if err != nil {
		return err
	}
	acc &= st.Acc // e.g. ambient has no drums
	if *notes == 0 {
		*notes = st.Notes
	}
	if *bpm == 0 {
		*bpm = st.BPM
	}
	if fs.NArg() != 1 || *notes < 8 || *popSize < 8 || *count < 1 || *start < 1 || *attempts < 1 {
		usage()
	}
	dir := fs.Arg(0)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	logf, err := os.OpenFile(filepath.Join(dir, "generate.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer logf.Close()
	fmt.Fprintf(logf, "# %s\n", strings.Join(os.Args, " "))

	src, err := os.Open(*source)
	if err != nil {
		return err
	}
	defer src.Close()
	rng := melody.NewRand(src)

	digits := max(len(fmt.Sprint(*start+*count-1)), 2)
	for c := *start; c < *start+*count; c++ {
		base := filepath.Join(dir, fmt.Sprintf("%s%0*d", *name, digits, c))
		fmt.Printf("%s: evolving %s, %d notes from %s, target %.0f\n", filepath.Base(base), st.Name, *notes, *source, *target)

		var genome []byte
		var best melody.Composition
		total := 0
		for try := 1; try <= *attempts; try++ {
			shown := -1
			g, s, n := melody.Evolve(rng, melody.Options{
				Notes: *notes, Acc: acc, Style: st, Population: *popSize, Generations: *gens, Target: *target,
				Progress: func(gen int, s melody.Composition) {
					if int(s.Total) > shown { // one line per whole point gained
						shown = int(s.Total)
						fmt.Printf("  gen %6d  total %5.1f  melody %5.1f  %s\n", gen, s.Total, s.Melody.Total, s.Melody.KeyName)
					}
				},
			})
			total += n
			if genome == nil || s.Total > best.Total {
				genome, best = g, s
			}
			if best.Total >= *target {
				break
			}
			if try < *attempts {
				fmt.Printf("  attempt %d missed the target (%.1f), restarting\n", try, s.Total)
			}
		}
		status := "target reached"
		if best.Total < *target {
			status = "target missed, kept best"
		}
		fmt.Printf("  %s after %d generations\n", status, total)
		fmt.Print(best)

		gf := melody.GenomeFile{Notes: *notes, Acc: acc, Style: st, BPM: *bpm, Genome: genome}
		if err := os.WriteFile(base+".genome", melody.MarshalGenome(gf), 0o644); err != nil {
			return err
		}
		arr, err := writeScores(base, gf)
		if err != nil {
			return err
		}
		title := filepath.Base(base)
		if arr.Chords != nil {
			fmt.Println(melody.FormatChords(arr.Chords))
		}
		fmt.Printf("  wrote %[1]s.genome %[1]s.mid %[1]s.musicxml\n", title)
		// one write per song, so parallel runs sharing a log don't interleave
		entry := fmt.Sprintf("%s\t%s\ttotal %.1f\tmelody %.1f\tharmony %.2f\tbass %.2f\tdrums %.2f\t%s\t%d bars\t%s after %d generations\n",
			title, st.Name, best.Total, best.Melody.Total, best.Harmony, best.Bass, best.Drums, best.Melody.KeyName, arr.Bars, status, total)
		if arr.Chords != nil {
			entry += "\tchords: " + strings.ReplaceAll(melody.FormatChords(arr.Chords), "\n", " ") + "\n"
		}
		if _, err := logf.WriteString(entry); err != nil {
			return err
		}
	}
	return nil
}

// writeScores writes base.mid and base.musicxml for a genome.
func writeScores(base string, gf melody.GenomeFile) (melody.Arrangement, error) {
	_, scored := melody.EvaluateGenome(gf.Genome, gf.Notes, gf.Acc, gf.Style)
	arr := scored.Output()
	title := filepath.Base(base)
	if err := os.WriteFile(base+".mid", melody.MIDI(arr, gf.BPM, title), 0o644); err != nil {
		return arr, err
	}
	return arr, os.WriteFile(base+".musicxml", melody.MusicXML(arr, gf.BPM, title), 0o644)
}

func readGenome(path string) (melody.GenomeFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return melody.GenomeFile{}, err
	}
	gf, err := melody.UnmarshalGenome(b)
	if err != nil {
		return gf, fmt.Errorf("%s: %w", path, err)
	}
	return gf, nil
}

// render rebuilds .mid and .musicxml next to each saved genome.
func render(args []string) error {
	fs := flag.NewFlagSet("render", flag.ExitOnError)
	bpm := fs.Int("bpm", 0, "tempo (0 = the tempo saved in the genome file)")
	fs.Parse(args)
	if fs.NArg() < 1 {
		usage()
	}
	for _, path := range fs.Args() {
		gf, err := readGenome(path)
		if err != nil {
			return err
		}
		if *bpm > 0 {
			gf.BPM = *bpm
		}
		base := strings.TrimSuffix(path, filepath.Ext(path))
		if _, err := writeScores(base, gf); err != nil {
			return err
		}
		fmt.Printf("%s -> %s.mid %s.musicxml\n", path, filepath.Base(base), filepath.Base(base))
	}
	return nil
}

// score prints the detector's verdict and chord progression for a genome.
func score(args []string) error {
	if len(args) != 1 {
		usage()
	}
	gf, err := readGenome(args[0])
	if err != nil {
		return err
	}
	c, arr := melody.EvaluateGenome(gf.Genome, gf.Notes, gf.Acc, gf.Style)
	fmt.Printf("%s: %s, %d notes, %d bars, %d bpm\n", args[0], gf.Style.Name, gf.Notes, arr.Bars, gf.BPM)
	fmt.Print(c)
	if arr.Chords != nil {
		fmt.Println(melody.FormatChords(arr.Chords))
	}
	return nil
}

// serve runs the web UI.
func serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", ":9090", "listen address")
	library := fs.String("library", "LIBRARY", "directory for songs made in the UI")
	source := fs.String("source", "/dev/urandom", "random byte source")
	soundFonts := fs.String("soundfonts", "SOUNDFONTS", "directory for uploaded SoundFonts (.sf2/.sf3/.dls)")
	fs.Parse(args)
	for _, d := range []string{*library, *soundFonts} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	fmt.Printf("MUseRND UI on http://localhost%s (songs in %s, SoundFonts in %s)\n", *addr, *library, *soundFonts)
	return http.ListenAndServe(*addr, web.New(*library, *source, *soundFonts).Handler())
}

// genreNames lists the genre IDs for help texts.
func genreNames() string {
	var names []string
	for _, st := range melody.Styles {
		names = append(names, st.Name)
	}
	return strings.Join(names, ", ")
}
