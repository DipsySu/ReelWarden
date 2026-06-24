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
	{"Director's Cut", reTag(`director'?s[ ]?cut|dc`)},
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
		if cand != "" && !looksLikeTechnical(cand) {
			return cand, strings.TrimSpace(rest[m[1]:])
		}
	}
	if m := trailingGroupRE.FindStringSubmatchIndex(rest); m != nil {
		cand := strings.TrimSpace(rest[m[2]:m[3]])
		if cand != "" && len(cand) >= 2 && !looksLikeTechnical(cand) && !yearOnlyRE.MatchString(cand) {
			return cand, strings.TrimSpace(rest[:m[0]])
		}
	}
	return "", rest
}

var yearOnlyRE = regexp.MustCompile(`^(?:19|20)[0-9]{2}$`)

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
