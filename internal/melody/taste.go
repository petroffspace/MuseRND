package melody

import (
	"errors"
	"math"
	"sort"
)

// Taste is a small model of what the listener likes, learned from rated
// songs' character Features (see characterFeatures). With both liked and
// disliked songs it scores how much a piece leans towards the liked side
// (difference of centroids, measured in
// standard deviations per feature); with only liked (or only disliked)
// songs it scores closeness to (or distance from) their centroid.
type Taste struct {
	Liked, Disliked int
	center          []float64 // midpoint between centroids, or the one centroid
	dir             []float64 // standardized centroid difference (both kinds)
	std             []float64
	mode            int // 0 both, 1 liked only, -1 disliked only
}

// Weight is how much the taste should count: it grows with the number of
// ratings, from a quarter of max at 2 ratings to max at 6 or more, so a
// couple of ratings cannot override the music rules.
func (t *Taste) Weight(max float64) float64 {
	return max * math.Min(1, float64(t.Liked+t.Disliked)/6)
}

// Pref is one learned preference, for display.
type Pref struct {
	Feature string  `json:"feature"`
	Weight  float64 `json:"weight"` // >0 likes more of it, <0 likes less
}

// LearnTaste builds a taste model from feature vectors of liked and
// disliked songs. It needs at least two rated songs.
func LearnTaste(liked, disliked [][]float64) (*Taste, error) {
	if len(liked)+len(disliked) < 2 {
		return nil, errors.New("rate at least two songs of this genre")
	}
	// keep only character features
	pick := func(rows [][]float64) [][]float64 {
		out := make([][]float64, len(rows))
		for r, f := range rows {
			for i, v := range f {
				if characterFeatures[i] {
					out[r] = append(out[r], v)
				}
			}
		}
		return out
	}
	liked, disliked = pick(liked), pick(disliked)
	all := append(append([][]float64{}, liked...), disliked...)
	n := len(all[0])
	t := &Taste{Liked: len(liked), Disliked: len(disliked), std: make([]float64, n)}
	mean := func(rows [][]float64) []float64 {
		m := make([]float64, n)
		for _, r := range rows {
			for i, v := range r {
				m[i] += v / float64(len(rows))
			}
		}
		return m
	}
	mAll := mean(all)
	for i := range t.std {
		var v float64
		for _, r := range all {
			v += (r[i] - mAll[i]) * (r[i] - mAll[i])
		}
		t.std[i] = math.Max(0.06, math.Sqrt(v/float64(len(all))))
	}
	switch {
	case len(liked) > 0 && len(disliked) > 0:
		ml, md := mean(liked), mean(disliked)
		t.center, t.dir = make([]float64, n), make([]float64, n)
		for i := range ml {
			t.center[i] = (ml[i] + md[i]) / 2
			t.dir[i] = (ml[i] - md[i]) / t.std[i]
		}
	case len(liked) > 0:
		t.mode, t.center = 1, mean(liked)
	default:
		t.mode, t.center = -1, mean(disliked)
	}
	return t, nil
}

// Score rates features 0..1 (1 = matches the learned taste).
func (t *Taste) Score(full []float64) float64 {
	f := make([]float64, 0, len(t.std))
	for i, v := range full {
		if characterFeatures[i] {
			f = append(f, v)
		}
	}
	if t.mode == 0 {
		var z, norm float64
		for i, w := range t.dir {
			z += w * (f[i] - t.center[i]) / t.std[i]
			norm += math.Abs(w)
		}
		if norm == 0 {
			return 0.5
		}
		return 1 / (1 + math.Exp(-3*z/norm))
	}
	var d float64
	for i, c := range t.center {
		x := (f[i] - c) / t.std[i]
		d += x * x
	}
	near := math.Exp(-d / float64(len(t.center)) / 2)
	if t.mode < 0 {
		return 1 - near
	}
	return near
}

// Prefs lists the strongest learned preferences (liked and disliked songs
// both needed); weak ones (under 0.3 standard deviations) are left out.
func (t *Taste) Prefs(limit int) []Pref {
	var names []string
	for i, c := range characterFeatures {
		if c {
			names = append(names, FeatureNames[i])
		}
	}
	var ps []Pref
	for i, w := range t.dir {
		if math.Abs(w) >= 0.3 {
			ps = append(ps, Pref{names[i], w})
		}
	}
	sort.Slice(ps, func(i, j int) bool { return math.Abs(ps[i].Weight) > math.Abs(ps[j].Weight) })
	if len(ps) > limit {
		ps = ps[:limit]
	}
	return ps
}
