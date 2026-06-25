package parser

import (
	"regexp"
	"strings"
)

// edition.go covers the §12.2 technical-tag set, release-group extraction, and
// edition extraction, plus the title-aware year logic (§12.4). These run on the
// extension-stripped file name and feed the R0/R3-input fields of ParsedIdentity.

// ParserVersion identifies the parsing rule set for ParsedIdentity.ParserVersion.
const ParserVersion = "parser/0.1.1"

// --- §12.2 technical tags (full set) ---

// technicalTagPatterns recognizes the complete §12.2 tag set plus the very common
// scene variants that map onto the same concepts (so they are stripped, not kept as
// title). Each pattern is matched case-insensitively against word-ish boundaries.
// Order matters: multi-word tags (e.g. "Dolby Vision", "Directors Cut") must be
// listed before their single-word relatives so the longer match wins.
var technicalTagSpecs = []struct {
	name string         // canonical tag recorded in TechnicalTags
	re   *regexp.Regexp // matcher against the working (separator-spaced) string
}{
	// Resolution
	{"2160p", reTag(`2160p|\b4k\b|\buhd\b`)},
	{"1080p", reTag(`1080[pi]`)},
	{"720p", reTag(`720p`)},
	{"576p", reTag(`576[pi]`)},
	{"480p", reTag(`480[pi]`)},
	// HDR / color (multi-word first)
	{"Dolby Vision", reTag(`dolby[ ]?vision`)},
	{"HDR", reTag(`hdr(?:10)?(?:\+|plus)?`)},
	{"DV", reTag(`\bdv\b`)},
	// Source
	{"REMUX", reTag(`remux`)},
	{"BluRay", reTag(`blu[ -]?ray|\bbdrip\b|\bbrrip\b|\bbd\b`)},
	{"WEB-DL", reTag(`web[ -]?dl`)},
	{"WEBRip", reTag(`web[ -]?rip|\bweb\b`)},
	{"HDTV", reTag(`hdtv`)},
	{"DVDRip", reTag(`dvd[ -]?rip|\bdvd\b`)},
	// Video codecs (multi-word / dotted first)
	{"H.265", reTag(`h[ .]?265`)},
	{"H.264", reTag(`h[ .]?264`)},
	{"HEVC", reTag(`hevc`)},
	{"x265", reTag(`x[ ]?265`)},
	{"x264", reTag(`x[ ]?264`)},
	{"AV1", reTag(`\bav1\b`)},
	{"VC-1", reTag(`vc[ -]?1`)},
	// Audio (codec optionally glued to a channel layout, e.g. DDP5.1, DTS5.1)
	{"TrueHD", reTag(`true[ ]?hd(?:[ ]?[57][ .]1)?`)},
	{"Atmos", reTag(`atmos`)},
	{"DTS", reTag(`dts(?:[ -]?hd)?(?:[ -]?ma)?(?:[ ]?[57][ .]1)?`)},
	{"DDP", reTag(`(?:ddp|dd\+|e[ -]?ac[ -]?3|eac3)(?:[ ]?[57][ .]1)?`)},
	{"AC3", reTag(`(?:ac[ -]?3|dd)(?:[ ]?[57][ .]1)?`)},
	{"AAC", reTag(`aac(?:[ ]?[57][ .]1)?`)},
	{"FLAC", reTag(`flac`)},
	// Channel layouts (standalone, when not glued to a codec above)
	{"7.1", reTag(`7[ .]1`)},
	{"5.1", reTag(`5[ .]1`)},
	// Release qualifiers
	{"PROPER", reTag(`proper`)},
	{"REPACK", reTag(`repack`)},
	{"UNCUT", reTag(`uncut`)},
	{"REMASTERED", reTag(`remaster(?:ed)?`)},
	{"HYBRID", reTag(`hybrid`)},
	{"10bit", reTag(`10[ -]?bit`)},
	{"IMAX", reTag(`imax`)},
}

func reTag(body string) *regexp.Regexp {
	return regexp.MustCompile(`(?i)(?:^|[ ])(?:` + body + `)(?:[ ]|$)`)
}

// --- §9.2 editions (kept distinct from technical tags) ---

// editionSpecs map recognized edition markers to their canonical edition label.
// Editions are part of identity (§9.2), so they are recorded in Edition and NOT in
// TechnicalTags. Multi-word forms come first.
var editionSpecs = []struct {
	name string
	re   *regexp.Regexp
}{
	// The spelled-out "director's cut" form is unambiguous anywhere. The bare "dc"
	// abbreviation is only treated as an edition in a real edition context (it must
	// follow title text, never be the leading title token), so a *leading* "DC"
	// stays in the title (e.g. "DC League of Super-Pets"). See dcAbbrevRE.
	{"Director's Cut", reTag(`director'?s[ ]?cut`)},
	{"Director's Cut", dcAbbrevRE},
	{"Extended Edition", reTag(`extended(?:[ ](?:cut|edition|version))?`)},
	{"Theatrical Cut", reTag(`theatrical(?:[ ]cut)?`)},
	{"Unrated", reTag(`unrated`)},
	{"Special Edition", reTag(`special[ ]edition`)},
	{"Ultimate Edition", reTag(`ultimate[ ]edition`)},
	{"Final Cut", reTag(`final[ ]cut`)},
	{"Criterion Collection", reTag(`criterion(?:[ ]collection)?`)},
	{"IMAX Edition", reTag(`imax[ ]edition`)},
	{"Anniversary Edition", reTag(`(?:\d+th[ ])?anniversary(?:[ ]edition)?`)},
}

// dcAbbrevRE matches the bare "dc" Director's-Cut abbreviation only in a real
// edition context: it requires a literal leading space, so the token can never be
// the leading title token. This keeps a *leading* "DC" as a title token (e.g.
// "DC.League.of.Super-Pets", working string "DC League of Super Pets ...") while
// still recognizing the trailing edition form "Movie Title DC 2009". The required
// leading space is reinserted by removeMatch so the preceding token survives.
var dcAbbrevRE = regexp.MustCompile(`(?i)[ ](?:dc)(?:[ ]|$)`)

// --- release group ---

// trailingGroupRE captures a trailing scene group, typically "-GROUP" at the very
// end of the name (e.g. "...x265-RARBG") or a bracketed "[GROUP]" suffix. Groups are
// alnum-ish (optionally with @), no spaces.
var trailingGroupRE = regexp.MustCompile(`[-\[]([A-Za-z0-9]+(?:@[A-Za-z0-9]+)?)\]?$`)

// bracketGroupRE captures a leading bracketed group like "[GROUP] Title" used by
// many fansub/anime releases.
var bracketGroupRE = regexp.MustCompile(`^\[([^\]]{1,40})\]`)

// extractEdition finds an edition marker in the working string, returns the
// canonical label and the string with that marker removed.
func extractEdition(work string) (edition, rest string) {
	rest = work
	for _, spec := range editionSpecs {
		if loc := spec.re.FindStringIndex(rest); loc != nil {
			edition = spec.name
			rest = removeMatch(rest, loc)
			break // first (highest-priority) edition wins
		}
	}
	return edition, rest
}

// extractTechnicalTags strips every §12.2 tag (looping until stable so repeated
// tags like multiple audio tracks all go) and returns the ordered unique tag list
// plus the cleaned string.
func extractTechnicalTags(work string) (tags []string, rest string) {
	rest = work
	seen := map[string]bool{}
	for _, spec := range technicalTagSpecs {
		for {
			loc := spec.re.FindStringIndex(rest)
			if loc == nil {
				break
			}
			if !seen[spec.name] {
				seen[spec.name] = true
				tags = append(tags, spec.name)
			}
			rest = removeMatch(rest, loc)
		}
	}
	return tags, rest
}

// extractReleaseGroup pulls a leading [bracket] group or a trailing -GROUP token.
// It only treats a trailing token as a group when it is not itself a technical tag
// or a bare year, to avoid eating real title words.
func extractReleaseGroup(raw string) (group, rest string) {
	rest = strings.TrimSpace(raw)
	if m := bracketGroupRE.FindStringSubmatchIndex(rest); m != nil {
		cand := strings.TrimSpace(rest[m[2]:m[3]])
		// Mirror the trailing-group guards: a leading "[2021]" is a year, not a
		// release group, and a single character is too short to be a real group.
		// Without these guards "[2021] The Northman" would yield group "2021" and
		// lose the year (the year pass never sees the bracketed token).
		if cand != "" && len(cand) >= 2 && !looksLikeTechnical(cand) && !yearOnlyRE.MatchString(cand) {
			return cand, strings.TrimSpace(rest[m[1]:])
		}
	}
	if m := trailingGroupRE.FindStringSubmatchIndex(rest); m != nil {
		cand := strings.TrimSpace(rest[m[2]:m[3]])
		prefix := strings.TrimSpace(rest[:m[0]])
		// A trailing hyphen segment is ambiguous: it can be a scene "-GROUP" suffix
		// OR the second half of a hyphenated title ("Spider-Man", "X-Men",
		// "Mission-Impossible"). Only treat a hyphen segment as a release group when
		// the prefix carries real release markers (a standalone year or a
		// technical/edition tag). A bracketed "[GROUP]" container is unambiguous, so
		// it is exempt from this requirement.
		hyphenSep := m[0] < len(rest) && rest[m[0]] == '-'
		if cand != "" && len(cand) >= 2 && !looksLikeTechnical(cand) && !yearOnlyRE.MatchString(cand) {
			if !hyphenSep || hasReleaseMarkers(prefix) {
				return cand, prefix
			}
		}
	}
	return "", rest
}

var yearOnlyRE = regexp.MustCompile(`^(?:19|20)[0-9]{2}$`)

// hasReleaseMarkers reports whether s carries a hallmark of a real scene/release
// name: a standalone 4-digit release year, or any recognized technical/edition
// tag. Plain hyphenated titles ("Spider-Man", "Mission-Impossible") have neither,
// so a trailing hyphen segment after such a prefix is title text, not a group.
func hasReleaseMarkers(s string) bool {
	probe := collapseSpaces(separatorsToSpaces(s))
	if probe == "" {
		return false
	}
	for _, f := range strings.Fields(probe) {
		if yearOnlyRE.MatchString(f) {
			return true
		}
		// Scan field-by-field as well as whole-string so a tag glued among title
		// words is still detected.
		if looksLikeTechnical(f) {
			return true
		}
	}
	return looksLikeTechnical(probe)
}

func looksLikeTechnical(s string) bool {
	probe := " " + strings.ToLower(s) + " "
	for _, spec := range technicalTagSpecs {
		if spec.re.MatchString(probe) {
			return true
		}
	}
	for _, spec := range editionSpecs {
		if spec.re.MatchString(probe) {
			return true
		}
	}
	return false
}

// --- helpers ---

// removeMatch deletes [loc[0],loc[1]) but preserves one surrounding space so token
// boundaries stay intact for subsequent matches.
func removeMatch(s string, loc []int) string {
	left := s[:loc[0]]
	right := s[loc[1]:]
	// reTag consumes a leading and trailing separator space; reinsert a single
	// space so adjacent tokens do not fuse (e.g. "a x264 b" -> "a b").
	joined := strings.TrimRight(left, " ") + " " + strings.TrimLeft(right, " ")
	return strings.TrimSpace(joined)
}
