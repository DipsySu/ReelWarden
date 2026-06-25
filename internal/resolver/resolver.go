// Package resolver runs the per-file confidence-routed escalation ladder R0..R5
// (authority §14.9, docs/design/resolver-pipeline.md). It accumulates
// store.Evidence, routes by store.ScoreBand, and returns a result that supports
// unresolved + multi-hypothesis outputs.
//
// The Integrate stage wires the concrete leaf rung implementations (parser,
// externalid, nfo, mediainfo, mediatype, matcher, airepair) into the ladder via
// the standardRungs builder below. To keep import cycles out, the leaf packages
// never import internal/resolver; this package imports them.
//
// Compliance boundary (non-negotiable, §0.2/§7.1/§7.2/§14.1):
//   - All provider queries and all scoring/selection are deterministic. The
//     resolver issues provider queries via the injected ProviderQuery func; the
//     AI rung (R4) never receives provider candidates and never queries the
//     provider -- it only consumes local untrusted signals and emits hypotheses.
//   - The ladder NEVER auto-confirms (§14.7): a resolved result is still a
//     human-confirm preselection; unresolved is a valid output (§12.6).
package resolver

import (
	"context"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/reelwarden/reelwarden/internal/airepair"
	"github.com/reelwarden/reelwarden/internal/externalid"
	"github.com/reelwarden/reelwarden/internal/matcher"
	"github.com/reelwarden/reelwarden/internal/mediainfo"
	"github.com/reelwarden/reelwarden/internal/mediatype"
	"github.com/reelwarden/reelwarden/internal/nfo"
	"github.com/reelwarden/reelwarden/internal/parser"
	"github.com/reelwarden/reelwarden/internal/store"
	"github.com/reelwarden/reelwarden/internal/tmdb"
)

// Exit is a rung's escalation decision.
type Exit int

const (
	// Continue: this rung did its work; climb to the next rung.
	Continue Exit = iota
	// Stop: a satisfying band was reached; terminate the ladder with success.
	Stop
	// Escalate: explicit hand-off upward (e.g. R3 low -> R4); equivalent to
	// Continue for ladder progression but records intent in evidence.
	Escalate
)

// ProviderQuery issues a deterministic provider search for one hypothesis and
// returns the structured-field candidates (§13.1 DTO -> §9.3 projection). It is
// the ONLY door to the provider in the ladder; AI rungs never receive it (§0.2,
// §14.1). It must be fail-soft: a provider/network error returns (nil, err) and
// the resolver isolates it so the local scan is never blocked (§11.5/§0.2).
//
// The Integrate caller (scanner/api) injects a concrete implementation (mock,
// local NFO, or the deterministic TMDB adapter). nil is allowed: with no
// provider the ladder degrades to unresolved with hypotheses (R5).
type ProviderQuery func(ctx context.Context, h store.QueryHypothesis) ([]matcher.ProviderCandidate, error)

// LLMClient is re-exported from internal/airepair so callers (scanner/api) can
// inject a local model for the R4 rung without importing airepair directly. It
// is the LOCAL-only repair client: it never receives provider data (§7.2).
type LLMClient = airepair.LLMClient

// Input is the immutable per-file context handed to every rung. It carries the
// asset, the local file-system context the rungs read (paths, parent dir), and
// the injected deterministic provider door + optional local AI client. Rungs
// that need provider data receive it via Query, never by calling a provider
// themselves; AI rungs receive only the local signals (§7.1/§7.2).
type Input struct {
	Asset store.MediaAsset

	// AbsPath is the absolute path to the media file, used by the local probe
	// (mediainfo) and sidecar NFO discovery (nfo). It is never sent to the AI.
	AbsPath string
	// RelPath is the library-root-relative path (slash-form) used for filename
	// external-ID extraction and as a local AI signal.
	RelPath string
	// ParentDirName is the immediate parent directory name (local untrusted).
	ParentDirName string

	// Query is the deterministic provider door (R1/R3). nil disables provider
	// lookups (the ladder then degrades to unresolved hypotheses).
	Query ProviderQuery
	// LLM is the optional local model for R4 filename repair. nil disables R4.
	// It receives LOCAL untrusted signals only (§7.2) and emits hypotheses only.
	LLM LLMClient
	// Ctx bounds provider/AI calls; defaults to context.Background when nil.
	Ctx context.Context

	// Candidates pre-seeds candidates already fetched by deterministic code. The
	// standard ladder normally fetches via Query, but a caller may pre-populate.
	Candidates []matcher.ProviderCandidate

	// ExcludedItemIDs / RejectedItemIDs feed §14.3/§14.6 (user excluded/rejected).
	ExcludedItemIDs map[string]bool
	RejectedItemIDs map[string]bool
}

func (in Input) ctx() context.Context {
	if in.Ctx != nil {
		return in.Ctx
	}
	return context.Background()
}

// State threads through the ladder: the evolving identity, accumulated evidence,
// the best candidate/band seen so far, and the rung that produced it.
type State struct {
	Identity   store.ParsedIdentity
	Evidence   []store.Evidence
	Candidates []store.Candidate
	Best       *store.Candidate
	RankScore  float64
	Band       store.ScoreBand
	LastRung   string

	// localExternalIDs are provider IDs extracted from the file name / NFO (§14.2),
	// keyed by namespace ("tmdb"/"imdb"/"tvdb"). Used by R3 scoring.
	localExternalIDs map[string]string
	// runtimeMinutes is the ffprobe-derived runtime (R2), 0 if unprobed.
	runtimeMinutes int
	// scored marks that R3 has already produced a banded result so R4 knows to
	// re-run scoring after appending AI hypotheses.
	scored bool

	// scoredCandidates retains the deterministic ProviderCandidate set scored so
	// far, keyed by provider+provider-id. R4 merges its AI-hypothesis candidates
	// into this set so the human-review list never shrinks (§12.6) and the best is
	// taken across R3 + R4.
	scoredCandidates map[string]matcher.ProviderCandidate
}

// invalidateScore drops the cached banded result so the next scoreViaProvider
// call re-scores from scratch. It is used when a later rung changes an input that
// the cached score depended on (e.g. R2 refining MediaTypeHint after R1 scored):
// the stale band/Best must not survive into R3. The accumulated candidate set is
// kept so re-scoring never loses a candidate from the human-review list (§12.6).
func (st *State) invalidateScore() {
	st.scored = false
	st.Best = nil
	st.RankScore = 0
	st.Band = store.BandLow
}

// Rung is a single step of the R0..R5 ladder. It inspects/updates State, may
// append Evidence, and returns an Exit decision. Concrete rungs are implemented
// below and call the leaf packages; they MUST honor the compliance boundary
// (AI rungs consume local untrusted data only and emit hypotheses only).
type Rung interface {
	// ID is the stable rung label, e.g. "R0".."R5".
	ID() string
	// Run advances the state for this rung and reports how the ladder should proceed.
	Run(in Input, st *State) (Exit, error)
}

// Result is the terminal output of the ladder.
type Result struct {
	AssetID    string
	Identity   store.ParsedIdentity
	Evidence   []store.Evidence
	Candidates []store.Candidate // ranked; empty when unresolved with no candidates
	Best       *store.Candidate  // preselected best (still human-confirm); nil if unresolved
	RankScore  float64
	Band       store.ScoreBand
	State      string                  // "resolved" | "unresolved"
	Hypotheses []store.QueryHypothesis // surfaced for human review when unresolved (R5)
	StoppedAt  string                  // rung ID where the ladder stopped
}

// Resolve runs the supplied rungs in order (expected R0..R5), accumulating
// evidence and routing by band. A file climbs only as far as needed: a rung
// returning Stop terminates with a resolved result; otherwise the ladder runs to
// the end and emits an unresolved + multi-hypothesis result for human review.
//
// Confidence is the router (§14.9): rungs decide Stop vs Escalate by band, not by
// any presumed file "kind". Passing rungs in a different order is allowed but the
// caller owns ordering semantics.
func Resolve(in Input, rungs []Rung) (Result, error) {
	st := State{
		Identity:         in.Asset.ToIdentitySeed(),
		Band:             store.BandLow,
		localExternalIDs: map[string]string{},
	}

	stoppedAt := ""
	for _, r := range rungs {
		exit, err := r.Run(in, &st)
		st.LastRung = r.ID()
		if err != nil {
			// §0.2/§11.5: a rung failure (probe, provider, AI) must never break
			// the ladder. We record nothing fatal and keep climbing; the worst
			// case is an unresolved result for human review.
			st.Evidence = append(st.Evidence, store.Evidence{
				Group: "resolver", Code: "RUNG_DEGRADED",
				Message: r.ID() + ": " + err.Error(),
			})
			continue
		}
		if exit == Stop {
			stoppedAt = r.ID()
			break
		}
	}

	res := Result{
		AssetID:    in.Asset.ID,
		Identity:   st.Identity,
		Evidence:   st.Evidence,
		Candidates: st.Candidates, // full ranked list for human review (kept even when unresolved)
		RankScore:  st.RankScore,
		Band:       st.Band,
		Hypotheses: st.Identity.Hypotheses,
		StoppedAt:  stoppedAt,
	}
	// §14.7: high/medium with a best candidate is "resolved" (still human-confirm).
	// Anything else is unresolved -- uncertainty is a valid output (§12.6). We
	// never fabricate confidence and never auto-confirm.
	//
	// Best is the preselected match. It MUST be nil unless the result is resolved:
	// an unresolved (low-band) result that still carried a non-nil score-0 Best
	// would falsely advertise a preselection the bands never justified. Only a
	// resolved high/medium result surfaces a Best (still human-confirm).
	if st.Best != nil && (st.Band == store.BandHigh || st.Band == store.BandMedium) {
		res.State = "resolved"
		res.Best = st.Best
	} else {
		res.State = "unresolved"
		res.Identity.State = "unresolved"
		res.Best = nil
	}
	return res, nil
}

// StandardRungs builds the default R0..R5 ladder wired to the leaf packages.
// The caller injects the provider door and optional local AI via Input; this
// keeps the resolver deterministic and the AI strictly local-only.
func StandardRungs() []Rung {
	return []Rung{
		rungR0{}, rungR1{}, rungR2{}, rungR3{}, rungR4{}, rungR5{},
	}
}

// ResolveAsset is the high-level convenience the Integrate stage (scanner/api)
// calls: it runs the standard ladder for one asset and returns the Result.
func ResolveAsset(in Input) Result {
	res, _ := Resolve(in, StandardRungs())
	return res
}

// --- R0: input preservation + §12.3 title normalization ----------------------

type rungR0 struct{}

func (rungR0) ID() string { return "R0" }

func (rungR0) Run(in Input, st *State) (Exit, error) {
	// The parser owns §12.1 preservation + §12.3 normalization + title-aware year
	// (§12.4) + tag/edition/group extraction. It is deterministic and local-only.
	id := parser.Parse(in.RelPath, in.ParentDirName)
	id.MediaAssetID = in.Asset.ID
	st.Identity = id
	return Continue, nil
}

// --- R1: explicit external IDs (filename) + local NFO IDs ---------------------

type rungR1 struct{}

func (rungR1) ID() string { return "R1" }

func (rungR1) Run(in Input, st *State) (Exit, error) {
	// Filename-embedded IDs ([tmdbid-123], {imdb-tt..}). Local untrusted only.
	for _, m := range externalid.Parse(in.RelPath) {
		if st.localExternalIDs[m.Provider] == "" {
			st.localExternalIDs[m.Provider] = m.ID
		}
	}

	// Sidecar NFO IDs + local title/year (§14.2/§7.2). Discovery uses the absolute
	// path; parsing is fail-soft (malformed XML -> OK=false).
	if in.AbsPath != "" {
		if path, ok := nfo.Discover(in.AbsPath); ok {
			info := nfo.ParseFile(path)
			if info.OK {
				for _, u := range info.UniqueIDs {
					if st.localExternalIDs[u.Provider] == "" {
						st.localExternalIDs[u.Provider] = u.ID
					}
				}
				// A local NFO title/year is a local hint when the filename lacked one.
				// R0 left Hypotheses nil for an empty filename, so recovering the title
				// here is not enough: R3 issues queries from Hypotheses, so we must
				// rebuild them (re-normalize + re-derive comparison keys) or the
				// provider would never be queried for this file (§14.9 R1 -> R3).
				recoveredTitle := false
				if st.Identity.RawTitle == "" && info.Title != "" {
					st.Identity.RawTitle = info.Title
					recoveredTitle = true
				}
				if st.Identity.Year == 0 && info.Year != 0 {
					st.Identity.Year = info.Year
				}
				if recoveredTitle {
					rebuildHypothesesFromTitle(&st.Identity)
				}
			}
		}
	}

	if len(st.localExternalIDs) == 0 {
		return Continue, nil
	}

	st.Evidence = append(st.Evidence, store.Evidence{
		Group: "external_id", Code: "LOCAL_EXTERNAL_ID_FOUND",
		Message: "local external IDs detected; querying provider by ID (§14.2)",
	})
	st.Identity.Hypotheses = idConstrainedHypotheses(st.Identity, st.localExternalIDs)

	// §14.2: an explicit ID query expects a high-confidence hit. Build an
	// ID-constrained hypothesis and prepend it so it leads. The deterministic
	// query door resolves it; AI never does.
	if in.Query == nil {
		return Continue, nil // no provider door: fall through to R2/R3 hypotheses.
	}
	cands := scoreViaProvider(in, st, st.Identity.Hypotheses)
	if cands == 0 {
		return Continue, nil
	}
	// §14.2 ID short-circuit: an exact local<->candidate external-ID match is
	// AUTHORITATIVE. scoreViaProvider promotes such a candidate to BandHigh even
	// when its title is garbled (the soft external-ID weight alone cannot reach the
	// 0.95 threshold), so a [tmdbid-603] file with a mangled title still resolves on
	// the ID. §14.7 still holds: this is resolved/needs_review, never "confirmed".
	if st.Best != nil && st.Band == store.BandHigh {
		return Stop, nil
	}
	return Continue, nil
}

// --- R2: local structured signals (probe runtime + media-type token) ----------

type rungR2 struct{}

func (rungR2) ID() string { return "R2" }

func (rungR2) Run(in Input, st *State) (Exit, error) {
	// ffprobe runtime (fail-soft: absent/timeout -> OK=false, never errors). A
	// probe failure must NEVER block the scan (§0.2/§11.5).
	if in.AbsPath != "" {
		info := mediainfo.ProbeContext(in.ctx(), in.AbsPath)
		if info.OK {
			st.runtimeMinutes = info.RuntimeMinutes
			st.Evidence = append(st.Evidence, store.Evidence{
				Group: "probe", Code: "PROBE_OK",
				Message: "ffprobe runtime/resolution extracted",
			})
		} else {
			st.Evidence = append(st.Evidence, store.Evidence{
				Group: "probe", Code: "PROBE_UNAVAILABLE",
				Message: "ffprobe unavailable or failed; continuing without runtime",
			})
		}
	}

	// Refine the media-type hint from local tokens (file name + parent dir). The
	// parser set a filename-only hint; the dedicated extractor is authoritative
	// for R2. Only overwrite when it produced a concrete hint.
	//
	// If R1 already scored (st.scored) and R2 now CHANGES the hint, the cached
	// band/Best were produced with the pre-R2 hint and never saw the refined §14.3
	// media-type hard constraint. R3's "if !st.scored" guard would otherwise reuse
	// that stale result and skip re-scoring, so invalidate the cache here to force
	// R3 to re-score the candidates against the refined hint.
	if h := mediatype.Hint(st.Identity.RawTitle+" "+in.RelPath, in.ParentDirName); h != mediatype.HintNone {
		if st.scored && h != st.Identity.MediaTypeHint {
			st.invalidateScore()
		}
		st.Identity.MediaTypeHint = h
	}
	return Continue, nil
}

// --- R3: deterministic scoring of structured provider fields (§14.3-14.7) ------

type rungR3 struct{}

func (rungR3) ID() string { return "R3" }

func (rungR3) Run(in Input, st *State) (Exit, error) {
	// Build the deterministic TMDB query plan from the FULL ParsedIdentity now that
	// every pre-API signal is assembled (R1 external IDs, R0 NormalizedTitle/Year,
	// R2-refined MediaTypeHint, ordered Hypotheses). internal/tmdb is a pure planner
	// (ID-first, then constrained title search); it is NEVER called by AI and its
	// output is NEVER sent to AI (§7.1/§7.2). This materializes the ID-first /
	// constrained-title ordering so the door consumes ALL signals deterministically.
	recordTMDBPlan(in, st)

	// Fetch candidates if R1 did not already (or if the cache was invalidated, e.g.
	// R2 refined the media-type hint), then score deterministically. Provider
	// failures are isolated (§0.2).
	if !st.scored {
		_ = scoreViaProvider(in, st, st.Identity.Hypotheses)
	}

	switch st.Band {
	case store.BandHigh, store.BandMedium:
		return Stop, nil // band reached; stop (still human-confirm, §14.7).
	default:
		return Escalate, nil // low -> hand off to R4 local AI repair.
	}
}

// recordTMDBPlan builds the deterministic TMDB request plan from the full parsed
// identity + local external IDs and records it as explainable evidence (§14.8).
// The plan is ID-first (local tmdb -> detail, local imdb/tvdb/wikidata -> /find),
// then constrained title search per hypothesis. It is built deterministically and
// is never exposed to AI.
//
// The live provider door remains the injected ProviderQuery (offline mock today).
// TODO(adapter): when the real TMDB HTTP adapter lands (§13.1), execute these
// RequestPlans in order via the metadata gateway (proxy/cache/retry, §13.3-13.6)
// and project the responses into matcher.ProviderCandidate, replacing the
// per-hypothesis door with plan execution. The plan stays deterministic and
// explainable; nothing here calls the provider or the AI.
func recordTMDBPlan(in Input, st *State) {
	plans := tmdb.Plan(tmdb.PlanInput{
		Identity:    st.Identity,
		ExternalIDs: st.localExternalIDs,
	}, tmdb.Options{})
	if len(plans) == 0 {
		return
	}
	idFirst := plans[0].Path == tmdb.PathFindID ||
		plans[0].Path == tmdb.PathMovieDetail ||
		plans[0].Path == tmdb.PathTVDetail
	lead := "title-search"
	if idFirst {
		lead = "external-id"
	}
	st.Evidence = append(st.Evidence, store.Evidence{
		Group: "provider", Code: "TMDB_QUERY_PLAN",
		Message: "deterministic TMDB query plan built from all pre-API signals (" +
			lead + "-first; " + strconv.Itoa(len(plans)) + " request(s)); door executes it, AI never sees it",
	})
}

// --- R4: local AI filename repair (LOCAL untrusted signals ONLY) ---------------

type rungR4 struct{}

func (rungR4) ID() string { return "R4" }

func (rungR4) Run(in Input, st *State) (Exit, error) {
	if in.LLM == nil || in.Query == nil {
		return Escalate, nil // no local AI or no provider door -> R5.
	}
	// COMPLIANCE: the AI receives LOCAL untrusted signals ONLY (file name, parent
	// dir, relative dir). It never sees provider candidates and never queries the
	// provider; it emits hypotheses for deterministic re-scoring (§7.2/§14.1).
	sig := airepair.Signals{
		RawFileName:   filepathBase(in.RelPath),
		ParentDirName: in.ParentDirName,
		RelativeDir:   filepathDir(in.RelPath),
	}
	hyps, err := airepair.RepairFilename(in.ctx(), in.LLM, sig)
	if err != nil {
		return Escalate, err // degraded; ladder records it and escalates to R5.
	}
	if len(hyps) == 0 {
		return Escalate, nil
	}
	st.Identity.Hypotheses = append(st.Identity.Hypotheses, hyps...)
	st.Evidence = append(st.Evidence, store.Evidence{
		Group: "ai_repair", Code: "AI_HYPOTHESES_ADDED",
		Message: "local AI proposed repaired hypotheses (local-only); re-scoring deterministically",
	})

	// Re-run the deterministic query + scoring with ONLY the AI-augmented
	// hypotheses, but scoreViaProvider scores the MERGED candidate set: R3's
	// candidates are retained in st.scoredCandidates and the AI-hypothesis hits are
	// added to it (deduped by provider+provider-id). The human-review list must not
	// shrink to only the AI hits and the best is taken across R3 + R4 (§12.6). We
	// reset only the cached band/Best, never the accumulated candidate set.
	st.invalidateScore()
	_ = scoreViaProvider(in, st, hyps)
	if st.Best != nil && (st.Band == store.BandHigh || st.Band == store.BandMedium) {
		return Stop, nil
	}
	return Escalate, nil
}

// --- R5: unresolved + ranked hypotheses for human review (§12.6) ---------------

type rungR5 struct{}

func (rungR5) ID() string { return "R5" }

func (rungR5) Run(in Input, st *State) (Exit, error) {
	// Terminal. If a banded best somehow survived (it would have stopped earlier),
	// keep it; otherwise mark unresolved. Never fabricate confidence (§12.6/§14.7).
	st.Identity.State = "unresolved"
	st.Evidence = append(st.Evidence, store.Evidence{
		Group: "resolver", Code: "UNRESOLVED",
		Message: "no candidate reached a confident band; surfacing hypotheses for human review",
	})
	return Continue, nil
}

// --- shared deterministic helpers ---------------------------------------------

// scoreViaProvider issues the deterministic provider query for the supplied
// hypotheses, MERGES the returned candidates into the accumulated set
// (deduplicated by provider + provider-item-id), scores the WHOLE set with the
// matcher (§14.3-14.7), and records the §14.8-shaped store.Candidate list and the
// best banded result. It returns the total number of scored candidates.
//
// Merging (not replacing) is load-bearing: R4 re-scores with AI-appended
// hypotheses, and the human-review candidate list must NOT shrink to only the
// AI-hypothesis hits -- it must keep R3's candidates and pick the best across
// both (§12.6). Provider failures are swallowed into a degraded-evidence note
// (§0.2) and treated as zero new candidates so the ladder simply escalates.
func scoreViaProvider(in Input, st *State, hyps []store.QueryHypothesis) int {
	if st.scoredCandidates == nil {
		st.scoredCandidates = map[string]matcher.ProviderCandidate{}
		// Seed with any pre-populated candidates the caller supplied.
		for _, c := range in.Candidates {
			st.scoredCandidates[c.Provider+"/"+c.ProviderItemID] = c
		}
	}
	if in.Query != nil {
		for _, h := range hyps {
			got, err := in.Query(in.ctx(), h)
			if err != nil {
				st.Evidence = append(st.Evidence, store.Evidence{
					Group: "provider", Code: "META_PROVIDER_UNAVAILABLE",
					Message: "provider query failed for hypothesis '" + h.Title + "'; isolated (§0.2): " + err.Error(),
				})
				continue
			}
			for _, c := range got {
				key := c.Provider + "/" + c.ProviderItemID
				if _, ok := st.scoredCandidates[key]; ok {
					continue
				}
				st.scoredCandidates[key] = c
			}
		}
	}
	cands := mergedCandidates(st)
	if len(cands) == 0 {
		return 0
	}

	local := matcher.Local{
		Identity:        st.Identity,
		RuntimeMinutes:  st.runtimeMinutes,
		ExternalIDs:     st.localExternalIDs,
		ExcludedItemIDs: in.ExcludedItemIDs,
		RejectedItemIDs: in.RejectedItemIDs,
	}
	results := matcher.ScoreCandidates(local, cands)

	// §14.2 ID short-circuit: an exact local<->candidate external-ID match is
	// AUTHORITATIVE. The matcher models it as a soft weight (it can never alone
	// reach the 0.95 high threshold), so a file with a correct [tmdbid-...] but a
	// garbled title would stay low and never short-circuit. Promote a non-filtered
	// candidate carrying an authoritative external-ID match to the high band so R1
	// can Stop on the ID alone. §14.7 is preserved: the ladder still only
	// preselects (resolved/needs_review) and never auto-confirms.
	promoteAuthoritativeIDMatch(st.localExternalIDs, results)

	// Re-sort after any promotion so the authoritative match leads, then map to the
	// store shape. ScoreCandidates already sorted; a promotion may reorder. The
	// order mirrors the matcher's contract (rank_score desc, stable on provider
	// item id for determinism).
	sortResults(results)
	st.Candidates = matcher.ToStoreCandidates(in.Asset.ID, results)
	st.scored = true

	// Best banded candidate is the top-ranked one. It is a preselection only --
	// never auto-confirmed (§14.7).
	if len(results) > 0 {
		st.RankScore = results[0].RankScore
		st.Band = results[0].ScoreBand
		if len(st.Candidates) > 0 {
			best := st.Candidates[0]
			st.Best = &best
		}
	}
	return len(cands)
}

// mergedCandidates returns the accumulated candidate set in a deterministic order
// (by provider/provider-item-id key) so scoring/ranking is stable across runs.
func mergedCandidates(st *State) []matcher.ProviderCandidate {
	keys := make([]string, 0, len(st.scoredCandidates))
	for k := range st.scoredCandidates {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]matcher.ProviderCandidate, 0, len(keys))
	for _, k := range keys {
		out = append(out, st.scoredCandidates[k])
	}
	return out
}

// promoteAuthoritativeIDMatch raises a candidate to the high band when a local
// external ID exactly matches one of the candidate's external IDs (§14.2). The
// candidate must not have failed a hard constraint (a media-type mismatch or an
// ID conflict still wins -- an authoritative match is only authoritative when it
// is also consistent). The promotion is recorded as visible evidence (§14.8).
func promoteAuthoritativeIDMatch(localIDs map[string]string, results []matcher.ScoreResult) {
	if len(localIDs) == 0 {
		return
	}
	for i := range results {
		r := &results[i]
		if r.HardFiltered {
			continue
		}
		if !hasAuthoritativeIDMatch(r.Evidence) {
			continue
		}
		if r.RankScore < store.BandHighThreshold {
			r.RankScore = store.BandHighThreshold
		}
		r.ScoreBand = store.BandHigh
		r.Evidence.RankScore = r.RankScore
		r.Evidence.ScoreBand = store.BandHigh
		r.Evidence.Warnings = append(r.Evidence.Warnings,
			"AUTHORITATIVE_EXTERNAL_ID: exact local external-ID match treated as authoritative (§14.2); preselection only, still human-confirm (§14.7)")
	}
}

// hasAuthoritativeIDMatch reports whether a scored candidate's evidence contains
// an exact external-ID match group (matcher group "external_id", type
// "external_id_exact"). External-ID conflicts are a hard constraint upstream, so
// the presence of this group means a clean, exact match.
func hasAuthoritativeIDMatch(ev matcher.Evidence) bool {
	for _, g := range ev.Groups {
		if g.Group == "external_id" && g.SelectedEvidence.Type == "external_id_exact" {
			return true
		}
	}
	return false
}

// sortResults re-orders scored candidates by rank_score descending (stable on
// provider item id for determinism) and re-assigns 1-based ranks. It mirrors the
// matcher's own ordering so a post-scoring promotion (authoritative ID match)
// surfaces the promoted candidate first.
func sortResults(results []matcher.ScoreResult) {
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].RankScore != results[j].RankScore {
			return results[i].RankScore > results[j].RankScore
		}
		return results[i].ProviderItemID < results[j].ProviderItemID
	})
	for i := range results {
		results[i].Rank = i + 1
	}
}

// idConstrainedHypotheses prepends the local external IDs found in R1 to every
// existing hypothesis so deterministic provider code can prefer exact ID lookup
// (TMDB /find or detail endpoints) before title search. If parsing produced no
// title hypothesis, an ID-only hypothesis is still valid (§14.2).
func idConstrainedHypotheses(id store.ParsedIdentity, ids map[string]string) []store.QueryHypothesis {
	clean := cleanExternalIDs(ids)
	if len(clean) == 0 {
		return id.Hypotheses
	}
	base := id.Hypotheses
	if len(base) == 0 {
		base = []store.QueryHypothesis{{Source: "local_external_id"}}
	}
	out := make([]store.QueryHypothesis, 0, len(base))
	for _, h := range base {
		h.ExternalIDs = copyExternalIDs(clean)
		if h.Source == "" {
			h.Source = "local_external_id"
		}
		out = append(out, h)
	}
	return out
}

// rebuildHypothesesFromTitle re-derives the comparison fields and the query
// hypotheses from a freshly recovered RawTitle (R1 recovered it from a sidecar
// NFO when the filename had none). R0 builds Hypotheses from the parser, but for
// an empty filename it left them nil; recovering the title is useless unless the
// hypotheses are rebuilt, because R3 issues provider queries from Hypotheses.
//
// It re-runs the §12.3 normalizer (the exported parser.NormalizeTitle, the same
// chain R0 uses) to refresh NormalizedTitle + ComparisonKeys, then emits the same
// ordered hypotheses the parser would (title+year, normalized, comparison keys,
// parent-dir), so R3 has a query to issue. The display RawTitle is never
// overwritten (§12.3).
func rebuildHypothesesFromTitle(id *store.ParsedIdentity) {
	title := strings.TrimSpace(id.RawTitle)
	if title == "" {
		id.Hypotheses = nil
		return
	}
	normalized, compKeys := parser.NormalizeTitle(title)
	id.NormalizedTitle = normalized
	id.ComparisonKeys = compKeys

	var hyps []store.QueryHypothesis
	seen := map[string]bool{}
	add := func(t string, year int, src string) {
		t = strings.TrimSpace(t)
		if t == "" {
			return
		}
		key := strings.ToLower(t) + "|" + itoaYear(year)
		if seen[key] {
			return
		}
		seen[key] = true
		hyps = append(hyps, store.QueryHypothesis{
			Title:     t,
			Year:      year,
			MediaType: id.MediaTypeHint,
			Source:    src,
		})
	}
	// Most constrained first (§12.6), mirroring parser.buildHypotheses.
	add(id.RawTitle, id.Year, "rule")
	if normalized != "" && !strings.EqualFold(normalized, id.RawTitle) {
		add(normalized, id.Year, "rule")
	}
	for _, k := range compKeys {
		add(k, id.Year, "comparison_key")
	}
	if p := strings.TrimSpace(id.ParentDirName); p != "" && !strings.EqualFold(p, id.RawTitle) {
		add(p, id.Year, "parent_dir")
	}
	id.Hypotheses = hyps
	if id.State == "unresolved" {
		id.State = "parsed" // a recovered title makes this parseable again.
	}
}

func itoaYear(y int) string {
	if y == 0 {
		return ""
	}
	return strconv.Itoa(y)
}

func cleanExternalIDs(ids map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range ids {
		if k == "" || v == "" {
			continue
		}
		out[k] = v
	}
	return out
}

func copyExternalIDs(ids map[string]string) map[string]string {
	out := make(map[string]string, len(ids))
	for k, v := range ids {
		out[k] = v
	}
	return out
}

// filepathBase / filepathDir operate on the slash-form RelPath (the scanner
// stores RelativePath via filepath.ToSlash), so the slash-aware path package is
// used rather than filepath. They feed the LOCAL-only AI signals (R4).
func filepathBase(rel string) string {
	if rel == "" {
		return ""
	}
	return path.Base(rel)
}

func filepathDir(rel string) string {
	if rel == "" {
		return ""
	}
	d := path.Dir(rel)
	if d == "." || d == "/" {
		return ""
	}
	return d
}
