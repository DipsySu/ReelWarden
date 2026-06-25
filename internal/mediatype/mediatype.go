// Package mediatype extracts a media-type hint from local untrusted signals
// (file name + parent directory tokens) per authority §12 and the resolver
// pipeline R2 contract. It is a pure, deterministic, leaf package: it never
// touches the network, the provider, or any provider-derived data. The hint
// is one of the ParsedIdentity.MediaTypeHint values:
//
//	"" | movie | tv | tv_liveaction | ova | special
//
// The hint constrains a downstream provider query (movie vs tv endpoint); it
// is a heuristic signal only and never decides the final match.
package mediatype

import (
	"sort"
	"strings"
)

// Hint values for ParsedIdentity.MediaTypeHint. The empty string means "no
// hint" (the caller should not narrow the query on type).
const (
	HintNone         = ""
	HintMovie        = "movie"
	HintTV           = "tv"
	HintTVLiveAction = "tv_liveaction"
	HintOVA          = "ova"
	HintSpecial      = "special"
)

// priority orders the hints when multiple token groups match. More specific
// release-form signals win over the broad movie/tv classification, so e.g.
// "剧场版" + "OVA" resolves to OVA. tv_liveaction outranks plain tv because the
// live-action marker is the more constrained query. Lower number = higher
// priority.
var priority = map[string]int{
	HintOVA:          0,
	HintSpecial:      1,
	HintMovie:        2,
	HintTVLiveAction: 3,
	HintTV:           4,
	HintNone:         99,
}

// CJK markers are matched as raw substrings; ASCII markers are matched
// case-insensitively against a normalized form (see Hint).
//
// liveActionMarkers flag live-action content. When combined with a TV signal
// they upgrade to tv_liveaction; on their own (e.g. a live-action film) they do
// not by themselves force a type, so they are tracked separately.
var liveActionMarkers = []string{"真人", "实写", "實寫", "实拍", "實拍", "live action", "liveaction", "live-action"}

// tvMarkers flag episodic / television content. These are matched as plain
// substrings: the CJK markers are unambiguous and the ASCII ones ("tv版",
// "drama") do not collide with ordinary title words.
//
// The English "season"/"series" markers are intentionally NOT here. As bare
// words they appear inside ordinary title tokens ("The Seasoning House") and,
// as standalone words, inside legitimate film titles ("Series 7: The
// Contenders"). They are handled by matchTV, which requires the constrained
// "Season N"/"Series N" form (see tvSeasonMarkers / matchTV).
var tvMarkers = []string{"电视版", "電視版", "tv版", "drama", "剧集", "劇集", "连续剧", "連續劇", "电视剧", "電視劇"}

// tvSeasonMarkers are English season/series words that only count as a TV
// signal in the constrained "<marker> <number>" trailing form (e.g.
// "Season 1", "Series 3"), where the number is the final token. This excludes
// substring hits ("Seasoning") and titles that merely begin with the word
// followed by more title text ("Series 7 The Contenders").
var tvSeasonMarkers = []string{"season", "series"}

// movieMarkers flag theatrical / film content.
var movieMarkers = []string{"剧场版", "劇場版", "movie", "theatrical", "the movie", "gekijouban", "gekijo-ban"}

// ovaMarkers flag original video / disc-only animation.
var ovaMarkers = []string{"ova", "oad", "ona"}

// specialMarkers flag specials / extras.
var specialMarkers = []string{"sp", "special", "特别篇", "特別篇", "特典", "番外篇", "番外", "specials"}

// Hint returns the media-type hint for a file's base name and its parent
// directory name. Both are local untrusted inputs (§7.2/§7.4) and are treated
// as data only. It returns HintNone when nothing matches.
func Hint(fileName, parentDir string) string {
	hay := normalize(fileName + " " + parentDir)
	// spaced collapses ASCII separators (. - _) to spaces so multi-word ASCII
	// markers like "live action" match dotted scene names ("Live.Action").
	spaced := strings.NewReplacer(".", " ", "-", " ", "_", " ").Replace(hay)

	hits := map[string]bool{}
	liveAction := containsAny(hay, liveActionMarkers) || containsAny(spaced, liveActionMarkers)

	if containsAny(hay, ovaMarkers) {
		hits[HintOVA] = true
	}
	if matchSpecial(hay) {
		hits[HintSpecial] = true
	}
	if containsAny(hay, movieMarkers) {
		hits[HintMovie] = true
	}
	tv := containsAny(hay, tvMarkers) || matchTV(hay)
	if tv {
		hits[HintTV] = true
	}
	if liveAction && tv {
		hits[HintTVLiveAction] = true
		delete(hits, HintTV)
	}
	// A live-action marker with no TV signal but with a movie signal stays a
	// movie; with neither it is too weak to assert a type on its own.

	return pick(hits)
}

// matchSpecial guards the very short "sp" marker so it does not fire inside
// unrelated tokens. It requires "sp" to appear as a standalone token (bounded
// by separators) and matches the longer special markers as plain substrings.
func matchSpecial(hay string) bool {
	for _, m := range specialMarkers {
		if m == "sp" {
			if hasStandaloneToken(hay, "sp") {
				return true
			}
			continue
		}
		if strings.Contains(hay, m) {
			return true
		}
	}
	return false
}

// matchTV reports a TV signal from English season/series markers without the
// false positives of unbounded substring matching. It accepts two constrained
// forms only:
//
//   - "<marker> <number>" where marker is a standalone "season"/"series" token
//     and number is the final token (e.g. "Season 1", "Series 3"). A trailing
//     title after the number ("Series 7 The Contenders") is rejected because
//     there the word is part of the title, not a season ordinal.
//   - a standalone "Sxx" episode-style token (e.g. "s01"), bounded by
//     separators.
//
// hay is assumed already normalized (lowercased, separators preserved).
func matchTV(hay string) bool {
	for _, m := range tvSeasonMarkers {
		if matchSeasonNumber(hay, m) {
			return true
		}
	}
	return matchSxx(hay)
}

// matchSeasonNumber reports whether hay contains a standalone marker token
// immediately followed (across separators) by a number token that is the final
// token in hay.
func matchSeasonNumber(hay, marker string) bool {
	start := 0
	for {
		i := strings.Index(hay[start:], marker)
		if i < 0 {
			return false
		}
		i += start
		end := i + len(marker)
		leftOK := i == 0 || isSep(rune(hay[i-1]))
		rightOK := end == len(hay) || isSep(rune(hay[end]))
		if leftOK && rightOK {
			// Skip separators, then require an all-digit token that runs to the
			// end of hay.
			j := end
			for j < len(hay) && isSep(rune(hay[j])) {
				j++
			}
			if j < len(hay) && allDigitsToEnd(hay, j) {
				return true
			}
		}
		start = end
		if start >= len(hay) {
			return false
		}
	}
}

// allDigitsToEnd reports whether hay[from:] begins with one non-empty run of
// ASCII digits and has no further title tokens after it — only separators (or
// nothing) may trail. This makes "Series 3" and "Series 3 " (the latter from
// Hint appending " "+parentDir) both count, while "Series 7 the contenders"
// does not.
func allDigitsToEnd(hay string, from int) bool {
	if from >= len(hay) || hay[from] < '0' || hay[from] > '9' {
		return false
	}
	k := from
	for k < len(hay) && hay[k] >= '0' && hay[k] <= '9' {
		k++
	}
	for ; k < len(hay); k++ {
		if !isSep(rune(hay[k])) {
			return false
		}
	}
	return true
}

// matchSxx reports whether hay contains a standalone "s<digits>" token (e.g.
// "s01"), the conventional season marker in scene release names.
func matchSxx(hay string) bool {
	for i := 0; i < len(hay); i++ {
		if hay[i] != 's' {
			continue
		}
		if i != 0 && !isSep(rune(hay[i-1])) {
			continue
		}
		j := i + 1
		for j < len(hay) && hay[j] >= '0' && hay[j] <= '9' {
			j++
		}
		if j == i+1 { // no digits followed
			continue
		}
		if j == len(hay) || isSep(rune(hay[j])) {
			return true
		}
	}
	return false
}

// hasStandaloneToken reports whether tok appears in hay delimited by ASCII
// separators (space, dot, dash, underscore) or string boundaries. hay is
// assumed already normalized to lowercase with separators preserved.
func hasStandaloneToken(hay, tok string) bool {
	start := 0
	for {
		i := strings.Index(hay[start:], tok)
		if i < 0 {
			return false
		}
		i += start
		leftOK := i == 0 || isSep(rune(hay[i-1]))
		end := i + len(tok)
		rightOK := end == len(hay) || isSep(rune(hay[end]))
		if leftOK && rightOK {
			return true
		}
		start = i + len(tok)
		if start >= len(hay) {
			return false
		}
	}
}

func isSep(r rune) bool {
	switch r {
	case ' ', '.', '-', '_', '[', ']', '(', ')', '/', '\\':
		return true
	}
	return false
}

func containsAny(hay string, markers []string) bool {
	for _, m := range markers {
		if strings.Contains(hay, m) {
			return true
		}
	}
	return false
}

// pick returns the highest-priority hint among the matched set.
func pick(hits map[string]bool) string {
	if len(hits) == 0 {
		return HintNone
	}
	got := make([]string, 0, len(hits))
	for h := range hits {
		got = append(got, h)
	}
	sort.Slice(got, func(i, j int) bool { return priority[got[i]] < priority[got[j]] })
	return got[0]
}

// normalize lowercases and collapses CJK fullwidth separators so ASCII markers
// match regardless of case and the input's separator soup. It deliberately
// keeps CJK characters intact so CJK markers match by substring.
func normalize(s string) string {
	s = strings.ToLower(s)
	// Map a few fullwidth ASCII separators to their halfwidth forms so token
	// boundaries are recognized consistently.
	s = strings.NewReplacer(
		"　", " ",
		"．", ".",
		"（", "(",
		"）", ")",
		"［", "[",
		"］", "]",
	).Replace(s)
	return s
}
