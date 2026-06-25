package matcher

import (
	"testing"

	"github.com/reelwarden/reelwarden/internal/store"
)

func localID(title string, year int) Local {
	return Local{Identity: store.ParsedIdentity{
		RawTitle:        title,
		NormalizedTitle: title,
		Year:            year,
		MediaTypeHint:   "",
	}}
}

func findGroup(ev Evidence, group string) (EvidenceGroup, bool) {
	for _, g := range ev.Groups {
		if g.Group == group {
			return g, true
		}
	}
	return EvidenceGroup{}, false
}

// §14.4: correlated title signals must take the MAX, never the sum. A candidate
// whose normalized title, original title and an alias ALL match exactly must score
// the title group at exactly one signal's contribution (the max), and the others
// must appear as discarded_correlated_evidence.
func TestTitleGroupMaxNotSum(t *testing.T) {
	c := ProviderCandidate{
		Provider:       "mock",
		ProviderItemID: "1",
		MediaType:      MediaMovie,
		Title:          "Dune",
		OriginalTitle:  "Dune",
		Aliases:        []string{"Dune", "Dune Part One"},
		Year:           2021,
	}
	res := ScoreCandidate(localID("Dune", 2021), c)

	g, ok := findGroup(res.Evidence, "title")
	if !ok {
		t.Fatal("no title group emitted")
	}
	if g.SelectedEvidence.Contribution != round4(wTitleMax) {
		t.Fatalf("title contribution = %v, want exactly the single max %v (signals were summed?)",
			g.SelectedEvidence.Contribution, round4(wTitleMax))
	}
	if len(g.DiscardedCorrelatedEvidence) == 0 {
		t.Fatal("expected discarded correlated title evidence to be recorded (§14.8)")
	}

	// Hard upper bound: title(max) + year exact must be far below the sum of all
	// three title signals. Prove summing would have overshot.
	summed := wTitleMax * 3
	if res.RankScore >= summed {
		t.Fatalf("rank_score %v >= summed title weight %v: title signals appear to be summed", res.RankScore, summed)
	}
}

// The fuzzy signal must never be added on top of an exact title match: an exact
// normalized match plus a present fuzzy signal yields exactly the max, with fuzzy
// discarded.
func TestExactBeatsFuzzyNoStacking(t *testing.T) {
	c := ProviderCandidate{
		ProviderItemID: "1", MediaType: MediaMovie,
		Title:         "The Matrix",
		OriginalTitle: "The Matrix Reloaded", // fuzzy, lower than exact
		Year:          1999,
	}
	res := ScoreCandidate(localID("The Matrix", 1999), c)
	g, _ := findGroup(res.Evidence, "title")
	if g.SelectedEvidence.Type != "normalized_title_exact" {
		t.Fatalf("selected title signal = %q, want normalized_title_exact", g.SelectedEvidence.Type)
	}
	if g.SelectedEvidence.Contribution != round4(wTitleMax) {
		t.Fatalf("title contribution = %v, want %v", g.SelectedEvidence.Contribution, round4(wTitleMax))
	}
	found := false
	for _, d := range g.DiscardedCorrelatedEvidence {
		if d == "fuzzy_title_similarity" {
			found = true
		}
	}
	if !found {
		t.Fatalf("fuzzy signal should be discarded, got %v", g.DiscardedCorrelatedEvidence)
	}
}

// §14.7 band thresholds: high (>=0.95), medium (0.80<=x<0.95), low (<0.80).
func TestBandThresholds(t *testing.T) {
	// HIGH: exact title + exact year + runtime close + parent dir + external id.
	high := Local{
		Identity: store.ParsedIdentity{
			RawTitle: "Dune", NormalizedTitle: "Dune", Year: 2021,
			ParentDirName: "Dune", MediaTypeHint: "movie",
		},
		RuntimeMinutes: 155,
		ExternalIDs:    map[string]string{"tmdb": "438631"},
	}
	hc := ProviderCandidate{
		ProviderItemID: "438631", MediaType: MediaMovie, Title: "Dune",
		OriginalTitle: "Dune", Year: 2021, RuntimeMinutes: 155,
		ExternalIDs: map[string]string{"tmdb": "438631"},
	}
	hr := ScoreCandidate(high, hc)
	if hr.ScoreBand != store.BandHigh {
		t.Fatalf("high case: band=%s rank=%v, want high (>=0.95)", hr.ScoreBand, hr.RankScore)
	}
	if hr.RankScore < store.BandHighThreshold {
		t.Fatalf("high case rank_score %v < %v", hr.RankScore, store.BandHighThreshold)
	}

	// MEDIUM: exact title + exact year, no other corroboration.
	med := localID("Dune", 2021)
	mc := ProviderCandidate{ProviderItemID: "1", MediaType: MediaMovie, Title: "Dune", Year: 2021}
	mr := ScoreCandidate(med, mc)
	if mr.ScoreBand != store.BandMedium {
		t.Fatalf("medium case: band=%s rank=%v, want medium (0.80..0.95)", mr.ScoreBand, mr.RankScore)
	}
	if mr.RankScore < store.BandMediumThreshold || mr.RankScore >= store.BandHighThreshold {
		t.Fatalf("medium case rank_score %v out of [0.80,0.95)", mr.RankScore)
	}

	// LOW: weak fuzzy title, no year corroboration.
	low := localID("Some Obscure Film", 0)
	lc := ProviderCandidate{ProviderItemID: "1", MediaType: MediaMovie, Title: "Totally Different Movie"}
	lr := ScoreCandidate(low, lc)
	if lr.ScoreBand != store.BandLow {
		t.Fatalf("low case: band=%s rank=%v, want low (<0.80)", lr.ScoreBand, lr.RankScore)
	}
	if lr.RankScore >= store.BandMediumThreshold {
		t.Fatalf("low case rank_score %v >= %v", lr.RankScore, store.BandMediumThreshold)
	}
}

// §14.3 hard constraint: media-type mismatch demotes to the floor and is visible.
func TestHardConstraintMediaTypeMismatch(t *testing.T) {
	l := Local{Identity: store.ParsedIdentity{
		RawTitle: "Dune", NormalizedTitle: "Dune", Year: 2021, MediaTypeHint: "movie",
	}}
	c := ProviderCandidate{ProviderItemID: "1", MediaType: "tv", Title: "Dune", Year: 2021}
	res := ScoreCandidate(l, c)
	if !res.HardFiltered {
		t.Fatal("expected hard-filtered on media-type mismatch")
	}
	if res.RankScore != hardDemote || res.ScoreBand != store.BandLow {
		t.Fatalf("expected floor score/low band, got rank=%v band=%s", res.RankScore, res.ScoreBand)
	}
	if len(res.Evidence.Warnings) == 0 {
		t.Fatal("hard constraint must be visible as a warning (§14.3)")
	}
}

// §14.3 hard constraint: external-ID conflict in a shared namespace demotes.
func TestHardConstraintExternalIDConflict(t *testing.T) {
	l := Local{
		Identity:    store.ParsedIdentity{RawTitle: "Dune", NormalizedTitle: "Dune", Year: 2021},
		ExternalIDs: map[string]string{"tmdb": "111"},
	}
	c := ProviderCandidate{ProviderItemID: "x", MediaType: MediaMovie, Title: "Dune", Year: 2021,
		ExternalIDs: map[string]string{"tmdb": "222"}}
	res := ScoreCandidate(l, c)
	if !res.HardFiltered || res.RankScore != hardDemote {
		t.Fatalf("expected hard-filtered floor on external-id conflict, got filtered=%v rank=%v", res.HardFiltered, res.RankScore)
	}
}

// §14.2/§14.5 external ID exact match is a strong uncorrelated signal that pushes
// even a weak-title candidate into a high band, and is emitted as its own group.
func TestExternalIDExactMatch(t *testing.T) {
	l := Local{
		Identity:    store.ParsedIdentity{RawTitle: "Dune", NormalizedTitle: "Dune", Year: 2021},
		ExternalIDs: map[string]string{"tmdb": "438631"},
	}
	c := ProviderCandidate{ProviderItemID: "438631", MediaType: MediaMovie, Title: "Dune",
		Year: 2021, ExternalIDs: map[string]string{"tmdb": "438631"}}
	res := ScoreCandidate(l, c)
	if _, ok := findGroup(res.Evidence, "external_id"); !ok {
		t.Fatal("expected external_id evidence group")
	}
	if res.ScoreBand != store.BandHigh {
		t.Fatalf("external-id exact + title + year should be high, got band=%s rank=%v", res.ScoreBand, res.RankScore)
	}
}

// §14.6 conflict penalties are subtracted AND visible in the evidence.
func TestConflictPenaltyVisible(t *testing.T) {
	l := localID("Dune", 2021)
	// Year far apart (delta 5 > 2) -> penalty, even though title is exact.
	c := ProviderCandidate{ProviderItemID: "1", MediaType: MediaMovie, Title: "Dune", Year: 2016}
	res := ScoreCandidate(l, c)
	if len(res.Evidence.Penalties) == 0 {
		t.Fatal("expected a visible year_conflict penalty (§14.6)")
	}
	var found bool
	for _, p := range res.Evidence.Penalties {
		if p.Type == "year_conflict" {
			found = true
			if p.Contribution >= 0 {
				t.Fatalf("penalty contribution must be negative, got %v", p.Contribution)
			}
		}
	}
	if !found {
		t.Fatal("expected year_conflict penalty type")
	}
	// Title-exact alone (0.62) minus the year-far penalty must land below medium.
	if res.ScoreBand == store.BandHigh {
		t.Fatalf("year-conflicted candidate should not be high, got band=%s rank=%v", res.ScoreBand, res.RankScore)
	}
}

func TestUserRejectedPenalty(t *testing.T) {
	l := localID("Dune", 2021)
	l.RejectedItemIDs = map[string]bool{"1": true}
	c := ProviderCandidate{ProviderItemID: "1", MediaType: MediaMovie, Title: "Dune", Year: 2021}
	res := ScoreCandidate(l, c)
	if res.ScoreBand != store.BandLow {
		t.Fatalf("previously-rejected candidate must be demoted to low, got band=%s rank=%v", res.ScoreBand, res.RankScore)
	}
	var seen bool
	for _, p := range res.Evidence.Penalties {
		if p.Type == "user_previously_rejected" {
			seen = true
		}
	}
	if !seen {
		t.Fatal("expected user_previously_rejected penalty to be visible")
	}
}

// §14.5: the auxiliary group is capped; a pile of weak auxiliary signals cannot
// exceed auxGroupCap.
func TestAuxiliaryGroupCap(t *testing.T) {
	l := Local{
		Identity: store.ParsedIdentity{
			RawTitle: "Dune", NormalizedTitle: "Dune", Year: 2021,
			ParentDirName: "Dune", Edition: "Extended",
		},
		RuntimeMinutes: 155,
	}
	c := ProviderCandidate{
		ProviderItemID: "1", MediaType: MediaMovie, Title: "Dune", Year: 2021,
		RuntimeMinutes: 155, Edition: "Extended",
	}
	aux, _ := auxiliaryGroups(l, c)
	if aux > auxGroupCap+1e-9 {
		t.Fatalf("auxiliary sum %v exceeds cap %v", aux, auxGroupCap)
	}
}

// ScoreCandidates ranks by rank_score and hard-filtered candidates sort last.
func TestRankingOrder(t *testing.T) {
	l := localID("Dune", 2021)
	cands := []ProviderCandidate{
		{ProviderItemID: "weak", MediaType: MediaMovie, Title: "Dunes of Arrakis"},
		{ProviderItemID: "strong", MediaType: MediaMovie, Title: "Dune", Year: 2021},
		{ProviderItemID: "wrongtype", MediaType: "tv", Title: "Dune", Year: 2021},
	}
	// give the local a movie hint so wrongtype hard-fails
	l.Identity.MediaTypeHint = "movie"
	res := ScoreCandidates(l, cands)
	if res[0].ProviderItemID != "strong" {
		t.Fatalf("expected strong candidate ranked first, got %s", res[0].ProviderItemID)
	}
	if res[0].Rank != 1 {
		t.Fatalf("ranks not assigned: %+v", res)
	}
	if !res[len(res)-1].HardFiltered {
		t.Fatalf("hard-filtered candidate should sort last, got %s", res[len(res)-1].ProviderItemID)
	}
}

// §14.8 evidence shape: rank_score, score_band, groups with selected_evidence are
// all present on a scored candidate.
func TestEvidenceShape(t *testing.T) {
	res := ScoreCandidate(localID("Dune", 2021),
		ProviderCandidate{ProviderItemID: "1", MediaType: MediaMovie, Title: "Dune", Year: 2021})
	if res.Evidence.RankScore != res.RankScore {
		t.Fatalf("evidence.rank_score %v != result rank_score %v", res.Evidence.RankScore, res.RankScore)
	}
	if res.Evidence.ScoreBand != res.ScoreBand {
		t.Fatal("evidence.score_band mismatch")
	}
	if len(res.Evidence.Groups) == 0 {
		t.Fatal("expected at least one evidence group")
	}
	for _, g := range res.Evidence.Groups {
		if g.Group == "" || g.SelectedEvidence.Type == "" {
			t.Fatalf("malformed evidence group: %+v", g)
		}
	}
}

// CJK normalized-title exact match scores like ASCII exact.
func TestCJKTitleExact(t *testing.T) {
	l := localID("千与千寻", 2001)
	c := ProviderCandidate{ProviderItemID: "1", MediaType: MediaMovie, Title: "千与千寻", Year: 2001}
	res := ScoreCandidate(l, c)
	g, ok := findGroup(res.Evidence, "title")
	if !ok || g.SelectedEvidence.Type != "normalized_title_exact" {
		t.Fatalf("expected CJK normalized exact, got %+v", g)
	}
	if res.ScoreBand != store.BandMedium && res.ScoreBand != store.BandHigh {
		t.Fatalf("CJK exact + year should be at least medium, got %s rank=%v", res.ScoreBand, res.RankScore)
	}
}

// Bridge: float rank_score -> int store.Candidate, band preserved from float.
func TestToStoreCandidate(t *testing.T) {
	res := ScoreCandidate(localID("Dune", 2021),
		ProviderCandidate{Provider: "mock", ProviderItemID: "1", MediaType: MediaMovie, Title: "Dune", Year: 2021})
	sc := ToStoreCandidate("asset_1", res)
	if sc.ScoreBand != res.ScoreBand {
		t.Fatalf("bridge band %s != scorer band %s", sc.ScoreBand, res.ScoreBand)
	}
	if sc.AssetID != "asset_1" || sc.Provider != "mock" || sc.ProviderID != "1" {
		t.Fatalf("bridge identity fields wrong: %+v", sc)
	}
	if sc.Score <= 0 || sc.Score > 100 {
		t.Fatalf("scaled score out of range: %d", sc.Score)
	}
	if len(sc.Evidence) == 0 {
		t.Fatal("bridge dropped evidence")
	}
}

// Regression (bridge.go): ToStoreCandidate must carry the candidate Title and Year
// through ScoreResult into store.Candidate. Before the fix these were dropped, so a
// scanner-persisted candidate came back Title="" Year=0 and naming.JellyfinPath then
// renamed the file to "Unknown Movie". A scored candidate must round-trip to a
// store.Candidate with the non-empty Title and the correct Year.
func TestToStoreCandidateCarriesTitleAndYear(t *testing.T) {
	const wantTitle, wantYear = "V for Vendetta", 2005
	res := ScoreCandidate(
		localID("V for Vendetta", 2005),
		ProviderCandidate{Provider: "mock", ProviderItemID: "752", MediaType: MediaMovie, Title: wantTitle, Year: wantYear},
	)
	// The score result itself must expose the passthrough fields.
	if res.Title != wantTitle || res.Year != wantYear {
		t.Fatalf("ScoreResult dropped title/year: title=%q year=%d", res.Title, res.Year)
	}
	sc := ToStoreCandidate("asset_1", res)
	if sc.Title != wantTitle {
		t.Fatalf("store.Candidate.Title = %q, want %q (title dropped -> rename would be 'Unknown Movie')", sc.Title, wantTitle)
	}
	if sc.Year != wantYear {
		t.Fatalf("store.Candidate.Year = %d, want %d", sc.Year, wantYear)
	}
}

// Regression (matcher.go externalIDGroup): when more than one local external id
// matches the candidate, the emitted evidence detail must be deterministic. Go map
// iteration is randomized, so selecting the matched namespace by ranging the map made
// the persisted evidence detail vary across runs. The group must always select the
// lexicographically-first matching namespace.
func TestExternalIDGroupDeterministicSelection(t *testing.T) {
	local := Local{
		Identity:    store.ParsedIdentity{RawTitle: "Dune", NormalizedTitle: "Dune", Year: 2021},
		ExternalIDs: map[string]string{"tmdb": "438631", "imdb": "tt1160419", "tvdb": "999"},
	}
	c := ProviderCandidate{
		ProviderItemID: "438631", MediaType: MediaMovie, Title: "Dune", Year: 2021,
		// All three namespaces match, so the un-pinned implementation could pick any.
		ExternalIDs: map[string]string{"tmdb": "438631", "imdb": "tt1160419", "tvdb": "999"},
	}
	g, ok := externalIDGroup(local, c)
	if !ok {
		t.Fatal("expected an external_id match")
	}
	// "imdb" sorts before "tmdb" and "tvdb", so it must be the selected detail.
	const wantDetail = "imdb:tt1160419 == imdb:tt1160419"
	if g.SelectedEvidence.Detail != wantDetail {
		t.Fatalf("selected detail = %q, want %q (non-deterministic namespace selection)", g.SelectedEvidence.Detail, wantDetail)
	}
	// Stability: many runs must all agree on the same detail.
	for i := 0; i < 200; i++ {
		gi, _ := externalIDGroup(local, c)
		if gi.SelectedEvidence.Detail != wantDetail {
			t.Fatalf("run %d selected %q, want stable %q", i, gi.SelectedEvidence.Detail, wantDetail)
		}
	}
}

// Regression (normalizer divergence): the matcher previously re-implemented title
// normalization, omitting roman numerals / CJK punctuation / simp-trad folding, so
// the provider side and the local id.NormalizedTitle diverged and exact-match recall
// broke. Both sides now share parser.NormalizeTitle. "V for Vendetta" (local) and
// "V for Vendetta" (candidate) must normalize identically and hit the exact-title
// signal, and a roman-numeral spelling difference must still fold to exact.
func TestSharedNormalizerExactTitle(t *testing.T) {
	// Exact-title signal across the V-for-Vendetta case (matches the prompt repro).
	res := ScoreCandidate(
		localID("V for Vendetta", 2005),
		ProviderCandidate{ProviderItemID: "1", MediaType: MediaMovie, Title: "V for Vendetta", Year: 2005},
	)
	g, ok := findGroup(res.Evidence, "title")
	if !ok {
		t.Fatal("no title group emitted")
	}
	if g.SelectedEvidence.Type != "normalized_title_exact" {
		t.Fatalf("title signal = %q, want normalized_title_exact (normalizers diverged?)", g.SelectedEvidence.Type)
	}
	if g.SelectedEvidence.Contribution != round4(wTitleMax) {
		t.Fatalf("exact title contribution = %v, want %v", g.SelectedEvidence.Contribution, round4(wTitleMax))
	}

	// Roman-numeral fold: the old matcher normalizer left "II" intact, so "Rocky II"
	// (local) vs "Rocky 2" (provider) would have been only a fuzzy match. The shared
	// parser normalizer rewrites both to "rocky 2" -> exact.
	roman := ScoreCandidate(
		localID("Rocky II", 1979),
		ProviderCandidate{ProviderItemID: "2", MediaType: MediaMovie, Title: "Rocky 2", Year: 1979},
	)
	rg, ok := findGroup(roman.Evidence, "title")
	if !ok {
		t.Fatal("no title group emitted for roman-numeral case")
	}
	if rg.SelectedEvidence.Type != "normalized_title_exact" {
		t.Fatalf("roman-numeral title signal = %q, want normalized_title_exact (roman numerals not folded by matcher)", rg.SelectedEvidence.Type)
	}
}
