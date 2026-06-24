// Package parser implements ReelWarden's file-name parsing (authority §12). It owns
// the R0 "input preservation + title normalization" and the R3-input extraction
// (release group, edition, technical tags, title-aware year) of the resolver ladder
// (§14.9). It is a LEAF package: it depends only on internal/store for the shared
// ParsedIdentity / QueryHypothesis types and never imports internal/resolver.
package parser

import (
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/reelwarden/reelwarden/internal/store"
)

// Result is the legacy, minimal parse shape consumed by the scanner. It is kept for
// backward compatibility; Parse returns the full store.ParsedIdentity (§9.2).
type Result struct {
	Title string
	Year  int
	Tags  []string
}

// fourDigitRE finds any 4-digit run together with the byte before/after so we can
// reason about token boundaries (title-aware year, §12.4).
var fourDigitRE = regexp.MustCompile(`[0-9]{4}`)

// yearLo / yearHi bound a plausible release year (§12.4: 1900 .. current+2).
const yearLo = 1900

func yearHi() int { return time.Now().UTC().Year() + 2 }

// ParsePath is the legacy entry point used by the scanner. It returns the cleaned
// display-ish title, the title-aware year, and the stripped technical tags. It is a
// thin adapter over Parse so both code paths share one implementation.
func ParsePath(rel string) Result {
	id := Parse(rel, "")
	return Result{Title: id.RawTitle, Year: id.Year, Tags: id.TechnicalTags}
}

// Parse runs the full §12.1/§12.2/§12.3/§12.4 pipeline on a relative path and an
// optional parent directory name (local untrusted context, §7.2). It preserves the
// raw/display title and never lets normalization overwrite it (§12.3). The returned
// ParsedIdentity carries NormalizedTitle + ComparisonKeys for matching, the title-
// aware Year, Edition, ReleaseGroup, the full TechnicalTags set, and at least one
// QueryHypothesis ordered most-constrained first (§12.6).
//
// TODO(integrate): the resolver stage sets MediaAssetID/ID/State transitions and may
// append AI-repair hypotheses (R4); this function only produces the deterministic R0
// baseline. MediaTypeHint here is a filename-only signal; R2 refines it via probe.
func Parse(rel, parentDir string) store.ParsedIdentity {
	base := filepath.Base(filepath.ToSlash(rel))
	stem := strings.TrimSuffix(base, filepath.Ext(base))

	// §12.1: preserve inputs. parentDir is kept verbatim as a local signal.
	parent := ""
	if parentDir != "" {
		parent = parentDir
	} else if dir := filepath.Dir(filepath.ToSlash(rel)); dir != "." && dir != "/" && dir != "" {
		parent = filepath.Base(dir)
	}

	// 1) Release group first (operates on the raw stem, before separators collapse).
	group, work := extractReleaseGroup(stem)

	// 2) Collapse separators into a single working string for tag/year detection.
	//    This is a *working* copy; the raw title is reconstructed separately so we
	//    never lose original characters (§12.1/§12.3).
	work = separatorsToSpaces(work)
	work = collapseSpaces(work)

	// 3) Edition (identity, §9.2) and the full §12.2 technical-tag set.
	edition, work := extractEdition(work)
	tags, work := extractTechnicalTags(work)
	work = collapseSpaces(work)

	// 4) Title-aware year (§12.4). Removes the year token only when it is safely a
	//    year and not a title digit (§12.6).
	year, titleRegion := extractYear(work)

	rawTitle := strings.TrimSpace(titleRegion)
	if rawTitle == "" {
		// Year/tags ate everything; fall back to the pre-year working string so we
		// never emit an empty title for a file that clearly had one.
		rawTitle = strings.TrimSpace(work)
	}

	normalized, compKeys := NormalizeTitle(rawTitle)

	id := store.ParsedIdentity{
		RawTitle:        rawTitle,
		NormalizedTitle: normalized,
		ComparisonKeys:  compKeys,
		Year:            year,
		Edition:         edition,
		ReleaseGroup:    group,
		TechnicalTags:   tags,
		MediaTypeHint:   mediaTypeHint(stem, parent),
		ParentDirName:   parent,
		ParserVersion:   ParserVersion,
		State:           "parsed",
	}
	id.Hypotheses = buildHypotheses(id)
	id.Confidence = heuristicConfidence(id)
	if rawTitle == "" {
		id.State = "unresolved"
	}
	return id
}

// separatorsToSpaces converts the §12.3 separators to spaces on the raw (pre-fold)
// stem so the title region survives with its original casing/characters.
func separatorsToSpaces(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '.', '_', '[', ']', '(', ')', '{', '}', '|', '/', '\\':
			return ' '
		}
		return r
	}, s)
}

// extractYear implements the title-aware year rule (§12.4): prefer a standalone
// 4-digit token in [1900, currentYear+2]. It deliberately does NOT just take the
// last 4-digit number, and it will not strip a digit run that is fused to title text
// (§12.6, e.g. "2046", "Tron 2.0"). When several plausible years exist, the latest
// one that is a clean standalone token is chosen and removed from the title region.
// Year detection is width-aware: fullwidth digits (２０２１) are recognized while the
// surrounding title text keeps its original characters (§12.1).
func extractYear(work string) (year int, titleRegion string) {
	folded := foldDigitsToASCII(work) // same byte layout as work (1:1 rune folding kept index-safe below)
	type cand struct {
		val        int
		start, end int
	}
	var cands []cand
	for _, loc := range fourDigitRE.FindAllStringIndex(folded, -1) {
		v := atoi(folded[loc[0]:loc[1]])
		if v < yearLo || v > yearHi() {
			continue
		}
		if !standaloneToken(folded, loc[0], loc[1]) {
			continue // digits fused to title text -> part of the title (§12.6)
		}
		cands = append(cands, cand{val: v, start: loc[0], end: loc[1]})
	}
	if len(cands) == 0 {
		return 0, work
	}
	work = folded // strip from the folded copy; ASCII year tokens never carry title meaning
	// Prefer the last standalone year token (years usually trail the title), but
	// only when removing it still leaves a non-empty title. If stripping it would
	// empty the title, the number probably *is* the title (e.g. "2012", "1917").
	for i := len(cands) - 1; i >= 0; i-- {
		c := cands[i]
		remainder := strings.TrimSpace(work[:c.start] + " " + work[c.end:])
		if remainder != "" {
			return c.val, remainder
		}
	}
	// Every candidate is the whole title (numeric-only title): keep it as title,
	// emit no year (do not fabricate; §12.6).
	return 0, work
}

// foldDigitsToASCII maps fullwidth digits (U+FF10..U+FF19) to ASCII so year
// detection sees them, leaving every other rune (including fullwidth letters)
// untouched. Used only for the year pass; the display title region keeps its
// original letters.
func foldDigitsToASCII(s string) string {
	hasFW := false
	for _, r := range s {
		if r >= 0xFF10 && r <= 0xFF19 {
			hasFW = true
			break
		}
	}
	if !hasFW {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r >= 0xFF10 && r <= 0xFF19 {
			b.WriteRune(r - 0xFEE0)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// standaloneToken reports whether the [start,end) digit run is bounded by
// non-alphanumeric, non-CJK characters (or string edges). A run touching letters or
// Han/Kana is considered part of the title, not a year.
func standaloneToken(s string, start, end int) bool {
	if start > 0 {
		prev := prevRune(s, start)
		if isTitleAdjacent(prev) {
			return false
		}
	}
	if end < len(s) {
		next := nextRune(s, end)
		if isTitleAdjacent(next) {
			return false
		}
	}
	return true
}

// isTitleAdjacent reports whether r, if directly touching a 4-digit run, means the
// digits belong to the title rather than being a standalone year.
func isTitleAdjacent(r rune) bool {
	if r == ' ' || r == 0 {
		return false
	}
	if r >= '0' && r <= '9' {
		return true // part of a longer number
	}
	if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
		return true
	}
	if r >= 0x2E80 { // CJK and beyond
		return true
	}
	return false
}

func prevRune(s string, i int) rune {
	r := []rune(s[:i])
	if len(r) == 0 {
		return 0
	}
	return r[len(r)-1]
}

func nextRune(s string, i int) rune {
	for _, r := range s[i:] {
		return r
	}
	return 0
}

// mediaTypeHint derives a filename-only media-type hint (§12.5 scope is movies; the
// hint is advisory and R2 refines it). It looks for explicit type tokens in the stem
// or parent dir. Returns "" when unknown.
func mediaTypeHint(stem, parent string) string {
	hay := strings.ToLower(stem + " " + parent)
	switch {
	case containsAny(hay, "剧场版", "劇場版", "movie"):
		return "movie"
	case containsAny(hay, "真人", "实写", "實寫", "live action", "live-action"):
		return "tv_liveaction"
	case containsAny(hay, "ova", "oad"):
		return "ova"
	case containsAny(hay, " sp ", "special", "特别篇", "特別篇"):
		return "special"
	case seasonEpisodeRE.MatchString(stem):
		return "tv"
	}
	return ""
}

var seasonEpisodeRE = regexp.MustCompile(`(?i)\bS[0-9]{1,2}E[0-9]{1,3}\b`)

func containsAny(hay string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(hay, n) {
			return true
		}
	}
	return false
}

// buildHypotheses emits constrained provider-query hypotheses ordered most-
// constrained first (§12.6). Deterministic code issues these queries; AI never does
// (§14.1). The rule-based hypotheses come first; comparison-key variants follow as
// recall fallbacks.
func buildHypotheses(id store.ParsedIdentity) []store.QueryHypothesis {
	if id.RawTitle == "" {
		return nil
	}
	var hyps []store.QueryHypothesis
	seen := map[string]bool{}
	addHyp := func(title string, year int, src string) {
		title = strings.TrimSpace(title)
		if title == "" {
			return
		}
		key := strings.ToLower(title) + "|" + itoa(year)
		if seen[key] {
			return
		}
		seen[key] = true
		hyps = append(hyps, store.QueryHypothesis{
			Title:     title,
			Year:      year,
			MediaType: id.MediaTypeHint,
			Source:    src,
		})
	}
	// Most constrained: title + year.
	addHyp(id.RawTitle, id.Year, "rule")
	if id.NormalizedTitle != "" && !strings.EqualFold(id.NormalizedTitle, id.RawTitle) {
		addHyp(id.NormalizedTitle, id.Year, "rule")
	}
	// Comparison-key variants (simp/trad fold, spaceless) as recall fallbacks.
	for _, k := range id.ComparisonKeys {
		addHyp(k, id.Year, "comparison_key")
	}
	// Parent-dir signal: only when it differs and looks title-like.
	if p := strings.TrimSpace(id.ParentDirName); p != "" && !strings.EqualFold(p, id.RawTitle) {
		addHyp(p, id.Year, "parent_dir")
	}
	return hyps
}

// heuristicConfidence is an ordering heuristic, NOT a probability (§9.2). It rewards
// a clean title + year and penalizes empty/very short titles.
func heuristicConfidence(id store.ParsedIdentity) float64 {
	if id.RawTitle == "" {
		return 0
	}
	c := 0.5
	if id.Year != 0 {
		c += 0.25
	}
	if utf8Len(id.RawTitle) >= 2 {
		c += 0.15
	}
	if len(id.TechnicalTags) > 0 {
		c += 0.05 // technical tags imply a parseable release name
	}
	if c > 0.95 {
		c = 0.95 // never fabricate high confidence at the parser stage (§12.6)
	}
	return c
}

// atoi parses a non-negative integer prefix; it stops at the first non-digit.
func atoi(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return n
		}
		n = n*10 + int(r-'0')
	}
	return n
}
