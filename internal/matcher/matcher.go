// Package matcher is the R3 deterministic scorer of the resolver ladder
// (authority §14.3-14.7, docs/design/resolver-pipeline.md rung R3). Given a
// parsed local identity and the structured fields of provider candidates, it
//
//  1. applies hard constraints (§14.3): media-type mismatch, illegal year,
//     user-excluded candidate, external-ID conflict -> filter or strong demote;
//  2. scores the title evidence group (§14.4) by taking the MAX of the correlated
//     title signals {normalized exact, original exact, alias exact, fuzzy} -- it
//     MUST NOT sum them;
//  3. adds capped auxiliary evidence (§14.5): year exact, year +-1, runtime close,
//     parent-dir consistent, edition consistent, local NFO external ID;
//  4. applies conflict penalties (§14.6), kept visible in the evidence;
//  5. emits rank_score in [0,1] + a §14.7 ScoreBand and §14.8-shaped Evidence.
//
// This is a LEAF package. It performs NO provider I/O and NO LLM calls: it scores
// already-fetched structured fields. rank_score is an ordering heuristic, NOT a
// probability (§14.1). The resolver's Integrate stage wires this scorer in; this
// package never imports internal/resolver.
package matcher

import (
	"math"
	"sort"
	"strings"
	"unicode"

	"github.com/reelwarden/reelwarden/internal/store"
)

// ResolverVersion identifies this scorer's evidence/score contract (stored on
// candidate_matches.resolver_version, §9.4).
const ResolverVersion = "matcher/r3-v0.1.1"

// Media-type constants used for the §14.3 hard constraint and §14.6 penalty.
// The empty hint ("") means "unknown" and never triggers a media-type conflict.
const (
	MediaMovie = "movie"
)

// movieLikeHints are local MediaTypeHint values (§9.2) that a "movie" provider
// candidate is compatible with. v0.1.1 only organizes movies (§12.5), so a
// non-movie hint against a movie candidate is a hard media-type mismatch.
var movieLikeHints = map[string]bool{
	"":              true, // unknown -> no constraint
	"movie":         true,
	"film":          true,
	"theatrical":    true,
	"剧场版":           true,
	"tv_liveaction": true, // live-action adaptations are still organized as movies in v0.1.1
}

// Scoring weights. Contributions are fractions of rank_score in [0,1]; the title
// group dominates, auxiliary groups are capped well below it so no stack of weak
// auxiliary signals can substitute for a title match (§14.5 "per-group caps").
const (
	wTitleMax     = 0.62 // max contribution of the single strongest title signal (§14.4)
	wExternalID   = 0.34 // local NFO / filename external-ID exact match (§14.2/§14.5)
	wYearExact    = 0.26 // year exact (§14.5)
	wYearOff1     = 0.12 // year off-by-one (§14.5)
	wRuntimeClose = 0.07 // runtime within tolerance (§14.5)
	wParentDir    = 0.05 // parent-dir title consistency (§14.5)
	wEdition      = 0.03 // edition consistency (§14.5)

	// Auxiliary group cap (§14.5): the summed auxiliary contribution (everything
	// except the title group and the external-ID signal) is clamped to this.
	auxGroupCap = 0.36

	// Conflict penalties (§14.6). Negative contributions, kept visible.
	pYearFar       = 0.45 // |year delta| > 2
	pRuntimeFar    = 0.30 // runtime grossly inconsistent
	pTitleConflict = 0.30 // title core tokens clearly disjoint despite a query hit
	pUserRejected  = 1.00 // user previously rejected this candidate
	// External-ID conflict is handled as a §14.3 hard constraint (floor), not a
	// soft penalty, so no penalty weight is needed for it here.

	// hardDemote is the rank_score floor applied to a candidate that fails a hard
	// constraint but is not filtered out entirely (§14.3): it must not be rescued
	// by weighted evidence, so it is demoted below the low band.
	hardDemote = 0.0

	runtimeCloseMinutes = 10 // |file - provider| <= this -> "runtime_close"
	runtimeFarMinutes   = 45 // |file - provider| >= this -> hard runtime conflict
	yearMinValid        = 1900
)

// ProviderCandidate is the structured-field projection of a provider item
// (§9.3 ProviderItem) that the scorer reads. It carries NO raw provider payload
// and NO display-only fields beyond what scoring needs. The Integrate stage maps
// its provider DTO / store.Candidate into this shape.
//
// TODO(integrate): the resolver owns the provider DTO. When the metadata provider
// interface (§13.1 MovieCandidate / MovieMetadata) lands, map it -> ProviderCandidate
// here (or in the resolver wiring) and feed ScoreCandidates from R1/R3 fetch output.
type ProviderCandidate struct {
	// Identity / provenance.
	Provider       string // e.g. "tmdb", "local_nfo", "mock"
	ProviderItemID string // stable provider id; used for external-ID match/conflict
	MediaType      string // provider media type; "" if unknown

	// Structured comparison fields (§9.3).
	Title          string   // primary localized title
	OriginalTitle  string   // original-language title
	Aliases        []string // alternative titles
	Year           int      // 0 if unknown
	RuntimeMinutes int      // 0 if unknown
	Edition        string   // edition label if the provider distinguishes one

	// ExternalIDs maps an id namespace (e.g. "tmdb", "imdb", "tvdb") to its value.
	// Used for §14.2 exact-ID match and §14.6 external-ID conflict.
	ExternalIDs map[string]string
}

// Local is the deterministic, locally-derived view scored against each candidate.
// Everything here originates from local untrusted data (file name, parent dir,
// local NFO non-provider fields) and the media probe -- never from the provider.
type Local struct {
	Identity store.ParsedIdentity // R0/R2 output: normalized title, year, hints, ...
	// RuntimeMinutes from the media probe (ffprobe duration), 0 if unprobed.
	RuntimeMinutes int
	// ExternalIDs extracted locally (filename tokens / local NFO <uniqueid>), §14.2.
	ExternalIDs map[string]string
	// ExcludedItemIDs are provider item ids the user explicitly excluded (§14.3).
	ExcludedItemIDs map[string]bool
	// RejectedItemIDs are provider item ids the user previously rejected (§14.6).
	RejectedItemIDs map[string]bool
}

// SelectedEvidence is the single chosen signal of a group (§14.8 selected_evidence).
type SelectedEvidence struct {
	Type         string  `json:"type"`
	Contribution float64 `json:"contribution"`
	Detail       string  `json:"detail"`
}

// EvidenceGroup is one §14.8 group: the selected (strongest) signal plus the
// names of correlated signals that were considered but discarded (not summed).
type EvidenceGroup struct {
	Group                       string           `json:"group"`
	SelectedEvidence            SelectedEvidence `json:"selected_evidence"`
	DiscardedCorrelatedEvidence []string         `json:"discarded_correlated_evidence,omitempty"`
}

// Penalty is a §14.6 conflict penalty, kept visible in the evidence.
type Penalty struct {
	Type         string  `json:"type"`
	Contribution float64 `json:"contribution"` // negative
	Detail       string  `json:"detail"`
}

// Evidence is the §14.8 evidence document for one scored candidate.
type Evidence struct {
	RankScore float64         `json:"rank_score"`
	ScoreBand store.ScoreBand `json:"score_band"`
	Groups    []EvidenceGroup `json:"groups"`
	Penalties []Penalty       `json:"penalties"`
	Warnings  []string        `json:"warnings"`
}

// ScoreResult is one candidate's deterministic score: the §14.8 evidence plus the
// resolved rank_score and band (§14.7). HardFiltered candidates are reported with
// rank_score 0 / low band and a warning, never silently dropped (§14.3 wants the
// demotion visible). Rank is the 1-based position after sorting (set by ScoreCandidates).
type ScoreResult struct {
	Provider       string
	ProviderItemID string
	RankScore      float64
	ScoreBand      store.ScoreBand
	Rank           int
	Evidence       Evidence
	HardFiltered   bool // failed a hard constraint and was demoted to the floor (§14.3)
}

// ScoreCandidate scores a single provider candidate against the local identity,
// emitting §14.8 evidence and a §14.7 band. It applies §14.3-14.6 in order.
func ScoreCandidate(local Local, c ProviderCandidate) ScoreResult {
	res := ScoreResult{
		Provider:       c.Provider,
		ProviderItemID: c.ProviderItemID,
		Evidence:       Evidence{Groups: []EvidenceGroup{}, Penalties: []Penalty{}, Warnings: []string{}},
	}

	// --- §14.3 hard constraints: filter / strong demote, do not rely on weights. ---
	if hard, warn := hardConstraintFail(local, c); hard {
		res.HardFiltered = true
		res.RankScore = hardDemote
		res.ScoreBand = store.BandFor(hardDemote)
		res.Evidence.Warnings = append(res.Evidence.Warnings, warn)
		res.Evidence.RankScore = res.RankScore
		res.Evidence.ScoreBand = res.ScoreBand
		// Still surface the title comparison so a human can see why it was demoted.
		if g, ok := titleGroup(local.Identity, c); ok {
			res.Evidence.Groups = append(res.Evidence.Groups, g)
		}
		return res
	}

	score := 0.0

	// --- §14.2/§14.5 external ID: a local id that exactly matches a candidate id
	// is the single strongest non-title signal. It is NOT part of the title group
	// (uncorrelated), so it is added, but capped on its own. ---
	if g, ok := externalIDGroup(local, c); ok {
		score += g.SelectedEvidence.Contribution
		res.Evidence.Groups = append(res.Evidence.Groups, g)
	}

	// --- §14.4 title evidence group: MAX of correlated title signals, never sum. ---
	if g, ok := titleGroup(local.Identity, c); ok {
		score += g.SelectedEvidence.Contribution
		res.Evidence.Groups = append(res.Evidence.Groups, g)
	}

	// --- §14.5 auxiliary evidence group, summed then clamped to auxGroupCap. ---
	aux, auxGroups := auxiliaryGroups(local, c)
	res.Evidence.Groups = append(res.Evidence.Groups, auxGroups...)
	score += aux

	// --- §14.6 conflict penalties, visible in evidence. ---
	penalty, penalties := conflictPenalties(local, c)
	res.Evidence.Penalties = append(res.Evidence.Penalties, penalties...)
	score -= penalty

	score = clamp01(score)
	res.RankScore = score
	res.ScoreBand = store.BandFor(score)
	res.Evidence.RankScore = score
	res.Evidence.ScoreBand = res.ScoreBand
	return res
}

// ScoreCandidates scores every candidate, sorts by rank_score descending (stable
// on provider item id for determinism), and assigns 1-based ranks. Hard-filtered
// candidates sort last.
func ScoreCandidates(local Local, candidates []ProviderCandidate) []ScoreResult {
	out := make([]ScoreResult, 0, len(candidates))
	for _, c := range candidates {
		out = append(out, ScoreCandidate(local, c))
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].RankScore != out[j].RankScore {
			return out[i].RankScore > out[j].RankScore
		}
		return out[i].ProviderItemID < out[j].ProviderItemID
	})
	for i := range out {
		out[i].Rank = i + 1
	}
	return out
}

// hardConstraintFail reports §14.3 hard-constraint failures: media-type mismatch,
// illegal candidate year, user-excluded candidate, external-ID conflict. A failing
// candidate is demoted to the floor rather than rescued by weighted evidence.
func hardConstraintFail(local Local, c ProviderCandidate) (bool, string) {
	if c.ProviderItemID != "" && local.ExcludedItemIDs[c.ProviderItemID] {
		return true, "EXCLUDED_CANDIDATE: user excluded this candidate"
	}
	// Media type: v0.1.1 organizes movies; a non-movie local hint vs a movie
	// candidate (or vice-versa) is incompatible.
	if c.MediaType == MediaMovie && !movieLikeHints[local.Identity.MediaTypeHint] {
		return true, "MEDIA_TYPE_MISMATCH: local hint '" + local.Identity.MediaTypeHint + "' incompatible with movie candidate"
	}
	if c.MediaType != "" && c.MediaType != MediaMovie && local.Identity.MediaTypeHint == MediaMovie {
		return true, "MEDIA_TYPE_MISMATCH: movie hint vs non-movie candidate '" + c.MediaType + "'"
	}
	// Illegal year: a candidate year outside the plausible window is a data error.
	if c.Year != 0 && (c.Year < yearMinValid) {
		return true, "ILLEGAL_YEAR: candidate year out of range"
	}
	// External-ID conflict: local and candidate disagree on the same namespace.
	if externalIDConflict(local.ExternalIDs, c.ExternalIDs) {
		return true, "EXTERNAL_ID_CONFLICT: local id disagrees with candidate id in a shared namespace"
	}
	return false, ""
}

// externalIDConflict reports whether any shared id namespace has differing values.
func externalIDConflict(localIDs, candIDs map[string]string) bool {
	for ns, lv := range localIDs {
		if lv == "" {
			continue
		}
		if cv, ok := candIDs[ns]; ok && cv != "" && !strings.EqualFold(cv, lv) {
			return true
		}
	}
	return false
}

// externalIDGroup emits the external-ID evidence group when a local id exactly
// matches the candidate (§14.2). Conflicts are handled as a hard constraint, so
// here we only ever produce a positive match.
func externalIDGroup(local Local, c ProviderCandidate) (EvidenceGroup, bool) {
	for ns, lv := range local.ExternalIDs {
		if lv == "" {
			continue
		}
		if cv, ok := c.ExternalIDs[ns]; ok && cv != "" && strings.EqualFold(cv, lv) {
			return EvidenceGroup{
				Group: "external_id",
				SelectedEvidence: SelectedEvidence{
					Type:         "external_id_exact",
					Contribution: wExternalID,
					Detail:       ns + ":" + lv + " == " + ns + ":" + cv,
				},
			}, true
		}
	}
	return EvidenceGroup{}, false
}

// titleGroup builds the §14.4 title group: it evaluates the four correlated title
// signals and selects the MAX as the single contribution. The other (lower)
// signals that fired are recorded as discarded_correlated_evidence -- never summed.
func titleGroup(id store.ParsedIdentity, c ProviderCandidate) (EvidenceGroup, bool) {
	type sig struct {
		typ     string
		score   float64
		detail  string
		present bool
	}
	localNorm := normalizeTitle(id.NormalizedTitle)
	if localNorm == "" {
		localNorm = normalizeTitle(id.RawTitle)
	}
	if localNorm == "" {
		return EvidenceGroup{}, false
	}

	sigs := []sig{}

	// 1. normalized title exact.
	if n := normalizeTitle(c.Title); n != "" {
		if n == localNorm {
			sigs = append(sigs, sig{"normalized_title_exact", 1.00, id.RawTitle + " == " + c.Title, true})
		} else {
			sigs = append(sigs, sig{"fuzzy_title_similarity", titleSimilarity(localNorm, n), id.RawTitle + " ~ " + c.Title, true})
		}
	}
	// also test local comparison keys (simplified/traditional folds) against the title.
	for _, ck := range id.ComparisonKeys {
		nk := normalizeTitle(ck)
		if nk != "" && nk == normalizeTitle(c.Title) {
			sigs = append(sigs, sig{"comparison_key_exact", 0.98, ck + " == " + c.Title, true})
		}
	}
	// 2. original title exact.
	if n := normalizeTitle(c.OriginalTitle); n != "" && n == localNorm {
		sigs = append(sigs, sig{"original_title_exact", 0.99, id.RawTitle + " == " + c.OriginalTitle, true})
	}
	// 3. alias exact.
	for _, al := range c.Aliases {
		if n := normalizeTitle(al); n != "" && n == localNorm {
			sigs = append(sigs, sig{"alias_exact", 0.97, id.RawTitle + " == " + al, true})
			break
		}
	}
	// 4. fuzzy against original title (only contributes via max).
	if n := normalizeTitle(c.OriginalTitle); n != "" && n != localNorm {
		sigs = append(sigs, sig{"fuzzy_title_similarity", titleSimilarity(localNorm, n), id.RawTitle + " ~ " + c.OriginalTitle, true})
	}

	if len(sigs) == 0 {
		return EvidenceGroup{}, false
	}

	// MAX, not sum (§14.4). Tie-break by signal strength order already encoded in score.
	best := sigs[0]
	for _, s := range sigs[1:] {
		if s.score > best.score {
			best = s
		}
	}
	discarded := []string{}
	seen := map[string]bool{}
	for _, s := range sigs {
		if s.typ == best.typ {
			continue
		}
		if !seen[s.typ] {
			discarded = append(discarded, s.typ)
			seen[s.typ] = true
		}
	}
	return EvidenceGroup{
		Group: "title",
		SelectedEvidence: SelectedEvidence{
			Type:         best.typ,
			Contribution: round4(best.score * wTitleMax),
			Detail:       best.detail,
		},
		DiscardedCorrelatedEvidence: discarded,
	}, true
}

// auxiliaryGroups builds the §14.5 auxiliary evidence groups and returns their
// summed contribution clamped to auxGroupCap. Each group independently selects its
// strongest signal; the year group takes max(exact, off-by-one) so the two
// correlated year signals are not summed.
func auxiliaryGroups(local Local, c ProviderCandidate) (float64, []EvidenceGroup) {
	groups := []EvidenceGroup{}
	sum := 0.0
	id := local.Identity

	// Year group: exact vs off-by-one are correlated -> select the stronger.
	if id.Year != 0 && c.Year != 0 {
		delta := id.Year - c.Year
		if delta < 0 {
			delta = -delta
		}
		switch {
		case delta == 0:
			g := EvidenceGroup{Group: "year", SelectedEvidence: SelectedEvidence{
				Type: "release_year_exact", Contribution: wYearExact,
				Detail: itoa(id.Year) + " == " + itoa(c.Year)}}
			if delta == 0 {
				g.DiscardedCorrelatedEvidence = nil
			}
			groups = append(groups, g)
			sum += wYearExact
		case delta == 1:
			groups = append(groups, EvidenceGroup{Group: "year", SelectedEvidence: SelectedEvidence{
				Type: "release_year_off_by_one", Contribution: wYearOff1,
				Detail: itoa(id.Year) + " ~ " + itoa(c.Year)}})
			sum += wYearOff1
		}
	}

	// Runtime group.
	if local.RuntimeMinutes != 0 && c.RuntimeMinutes != 0 {
		d := local.RuntimeMinutes - c.RuntimeMinutes
		if d < 0 {
			d = -d
		}
		if d <= runtimeCloseMinutes {
			groups = append(groups, EvidenceGroup{Group: "runtime", SelectedEvidence: SelectedEvidence{
				Type: "runtime_close", Contribution: wRuntimeClose,
				Detail: "file=" + itoa(local.RuntimeMinutes) + "m, provider=" + itoa(c.RuntimeMinutes) + "m"}})
			sum += wRuntimeClose
		}
	}

	// Parent-dir consistency: parent dir name shares the candidate title core.
	if id.ParentDirName != "" {
		pn := normalizeTitle(id.ParentDirName)
		cn := normalizeTitle(c.Title)
		if pn != "" && cn != "" && (pn == cn || titleSimilarity(pn, cn) >= 0.85) {
			groups = append(groups, EvidenceGroup{Group: "parent_dir", SelectedEvidence: SelectedEvidence{
				Type: "parent_dir_consistent", Contribution: wParentDir,
				Detail: id.ParentDirName + " ~ " + c.Title}})
			sum += wParentDir
		}
	}

	// Edition consistency.
	if id.Edition != "" && c.Edition != "" && strings.EqualFold(strings.TrimSpace(id.Edition), strings.TrimSpace(c.Edition)) {
		groups = append(groups, EvidenceGroup{Group: "edition", SelectedEvidence: SelectedEvidence{
			Type: "edition_consistent", Contribution: wEdition,
			Detail: id.Edition + " == " + c.Edition}})
		sum += wEdition
	}

	// §14.5 per-group cap on the auxiliary group as a whole.
	if sum > auxGroupCap {
		// Scale each contribution proportionally so the visible evidence still sums
		// to the clamped total (keeps §14.8 contributions honest).
		factor := auxGroupCap / sum
		for i := range groups {
			groups[i].SelectedEvidence.Contribution = round4(groups[i].SelectedEvidence.Contribution * factor)
		}
		sum = auxGroupCap
	}
	return round4(sum), groups
}

// conflictPenalties applies §14.6 penalties and returns the total (positive
// magnitude to subtract) plus the visible penalty records (negative contributions).
func conflictPenalties(local Local, c ProviderCandidate) (float64, []Penalty) {
	penalties := []Penalty{}
	total := 0.0
	id := local.Identity

	if c.ProviderItemID != "" && local.RejectedItemIDs[c.ProviderItemID] {
		penalties = append(penalties, Penalty{Type: "user_previously_rejected", Contribution: -pUserRejected,
			Detail: "user previously rejected " + c.ProviderItemID})
		total += pUserRejected
	}
	// Year far apart (> 2).
	if id.Year != 0 && c.Year != 0 {
		d := id.Year - c.Year
		if d < 0 {
			d = -d
		}
		if d > 2 {
			penalties = append(penalties, Penalty{Type: "year_conflict", Contribution: -pYearFar,
				Detail: itoa(id.Year) + " vs " + itoa(c.Year) + " (delta " + itoa(d) + ")"})
			total += pYearFar
		}
	}
	// Runtime grossly inconsistent.
	if local.RuntimeMinutes != 0 && c.RuntimeMinutes != 0 {
		d := local.RuntimeMinutes - c.RuntimeMinutes
		if d < 0 {
			d = -d
		}
		if d >= runtimeFarMinutes {
			penalties = append(penalties, Penalty{Type: "runtime_conflict", Contribution: -pRuntimeFar,
				Detail: "file=" + itoa(local.RuntimeMinutes) + "m vs provider=" + itoa(c.RuntimeMinutes) + "m"})
			total += pRuntimeFar
		}
	}
	// Title core-token conflict: the candidate matched the query but its title core
	// tokens are disjoint from the local title core tokens.
	if titleCoreDisjoint(id, c) {
		penalties = append(penalties, Penalty{Type: "title_core_conflict", Contribution: -pTitleConflict,
			Detail: "local and candidate title core tokens are disjoint"})
		total += pTitleConflict
	}
	return total, penalties
}

// titleCoreDisjoint reports whether the local and candidate title share no core
// token at all (after normalization). Empty local title -> not a conflict.
func titleCoreDisjoint(id store.ParsedIdentity, c ProviderCandidate) bool {
	local := tokenSet(normalizeTitle(firstNonEmpty(id.NormalizedTitle, id.RawTitle)))
	if len(local) == 0 {
		return false
	}
	cand := tokenSet(normalizeTitle(c.Title))
	for _, al := range append([]string{c.OriginalTitle}, c.Aliases...) {
		for t := range tokenSet(normalizeTitle(al)) {
			cand[t] = true
		}
	}
	if len(cand) == 0 {
		return false
	}
	for t := range local {
		if cand[t] {
			return false
		}
	}
	return true
}

// --- normalization & similarity helpers (§12.3 comparison-only normalization) ---

// normalizeTitle applies the comparison-only normalization used for scoring:
// Unicode case fold, fullwidth->halfwidth, separator/punctuation flattening,
// whitespace collapse. It must never be used as a display title (§12.3).
func normalizeTitle(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range s {
		r = foldWidth(r)
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
		case unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r):
			b.WriteRune(r)
		default:
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// foldWidth maps fullwidth ASCII variants to their halfwidth equivalents (§12.3).
func foldWidth(r rune) rune {
	if r >= 0xFF01 && r <= 0xFF5E {
		return r - 0xFEE0
	}
	if r == 0x3000 { // ideographic space
		return ' '
	}
	return r
}

// tokenSet splits a normalized title into a set of tokens. CJK runs are also split
// into individual characters so single-character overlap counts as shared.
func tokenSet(norm string) map[string]bool {
	out := map[string]bool{}
	for _, f := range strings.Fields(norm) {
		hasCJK := false
		for _, r := range f {
			if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r) {
				hasCJK = true
				out[string(r)] = true
			}
		}
		if !hasCJK {
			out[f] = true
		}
	}
	return out
}

// titleSimilarity returns a [0,1] similarity. It combines a token Jaccard with a
// character-bigram Dice coefficient, taking the max so reorderings and minor edits
// both score well. This is a heuristic fuzzy signal (§14.4), not a probability.
func titleSimilarity(a, b string) float64 {
	if a == "" || b == "" {
		return 0
	}
	if a == b {
		return 1
	}
	return math.Max(jaccard(tokenSet(a), tokenSet(b)), diceBigram(a, b))
}

func jaccard(a, b map[string]bool) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	inter := 0
	for k := range a {
		if b[k] {
			inter++
		}
	}
	union := len(a) + len(b) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

func diceBigram(a, b string) float64 {
	ba, bb := bigrams(a), bigrams(b)
	if len(ba) == 0 || len(bb) == 0 {
		return 0
	}
	inter := 0
	for g, n := range ba {
		if m, ok := bb[g]; ok {
			inter += min(n, m)
		}
	}
	ta, tb := 0, 0
	for _, n := range ba {
		ta += n
	}
	for _, n := range bb {
		tb += n
	}
	return 2 * float64(inter) / float64(ta+tb)
}

func bigrams(s string) map[string]int {
	rs := []rune(strings.ReplaceAll(s, " ", ""))
	out := map[string]int{}
	for i := 0; i+1 < len(rs); i++ {
		out[string(rs[i:i+2])]++
	}
	if len(rs) == 1 {
		out[string(rs)]++
	}
	return out
}

func clamp01(f float64) float64 {
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return round4(f)
}

func round4(f float64) float64 { return math.Round(f*10000) / 10000 }

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
