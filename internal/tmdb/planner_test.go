package tmdb

import (
	"reflect"
	"testing"

	"github.com/reelwarden/reelwarden/internal/store"
)

// identity is a tiny helper to build a PlanInput from a single hypothesis +
// optional top-level external IDs, mirroring the most common resolver call.
func single(mediaTypeHint string, h store.QueryHypothesis, ids map[string]string) PlanInput {
	return PlanInput{
		Identity: store.ParsedIdentity{
			MediaTypeHint: mediaTypeHint,
			Hypotheses:    []store.QueryHypothesis{h},
		},
		ExternalIDs: ids,
	}
}

// Step 1, branch: local tmdb id + movie hint -> single /movie/{id} detail.
func TestPlanTMDBIDMovieDetail(t *testing.T) {
	got := Plan(single("movie", store.QueryHypothesis{}, map[string]string{"tmdb": "603"}),
		Options{Language: "en-US"})
	if len(got) != 1 {
		t.Fatalf("expected exactly one movie detail plan, got %+v", got)
	}
	p := got[0]
	if p.Method != MethodGet || p.Path != PathMovieDetail {
		t.Fatalf("path = %s %s, want GET %s", p.Method, p.Path, PathMovieDetail)
	}
	if p.PathParams["tmdb_id"] != "603" {
		t.Fatalf("tmdb_id = %q", p.PathParams["tmdb_id"])
	}
	if p.Query["language"] != "en-US" {
		t.Fatalf("language = %q", p.Query["language"])
	}
	if p.Reason != "exact_tmdb_id" || p.Rank != 1 {
		t.Fatalf("metadata wrong: %+v", p)
	}
}

// Step 1, branch: local tmdb id + tv hint -> single /tv/{id} detail.
// Regression: the previous planner fabricated a movie-detail fallback for a
// tv hint. The deterministic plan emits exactly one detail call chosen by hint.
func TestPlanTMDBIDTVDetailNoFabricatedFallback(t *testing.T) {
	got := Plan(single("tv", store.QueryHypothesis{}, map[string]string{"tmdb": "1396"}),
		Options{Language: "en-US"})
	if len(got) != 1 {
		t.Fatalf("tv hint must yield exactly one tv detail plan (no opposite-type fallback), got %+v", got)
	}
	if got[0].Path != PathTVDetail {
		t.Fatalf("path = %s, want %s", got[0].Path, PathTVDetail)
	}
	if got[0].PathParams["tmdb_id"] != "1396" {
		t.Fatalf("tmdb_id = %q", got[0].PathParams["tmdb_id"])
	}
}

// Step 1, branch: local imdb id -> /find/{external_id} with external_source.
func TestPlanFindByIMDB(t *testing.T) {
	got := Plan(PlanInput{
		Identity: store.ParsedIdentity{
			MediaTypeHint: "movie",
			Hypotheses: []store.QueryHypothesis{
				{Title: "The Matrix", Year: 1999, MediaType: "movie"},
			},
		},
		ExternalIDs: map[string]string{"imdb": "tt0133093"},
	}, Options{Language: "zh-CN", Region: "CN"})

	if len(got) < 2 {
		t.Fatalf("expected /find then title fallback, got %+v", got)
	}
	first := got[0]
	if first.Method != MethodGet || first.Path != PathFindID {
		t.Fatalf("first = %s %s, want GET %s", first.Method, first.Path, PathFindID)
	}
	if first.PathParams["external_id"] != "tt0133093" {
		t.Fatalf("external_id = %q", first.PathParams["external_id"])
	}
	if first.Query["external_source"] != "imdb_id" {
		t.Fatalf("external_source = %q", first.Query["external_source"])
	}
	if first.Reason != "exact_imdb_id" || first.Rank != 1 {
		t.Fatalf("first metadata wrong: %+v", first)
	}
	// /find takes language but no region.
	if _, hasRegion := first.Query["region"]; hasRegion {
		t.Fatalf("/find must not carry region: %+v", first.Query)
	}
	if got[1].Path != PathSearchMovie {
		t.Fatalf("fallback = %s, want movie search", got[1].Path)
	}
}

// Step 2, branch: movie-like hypothesis -> /search/movie with constrained params.
func TestPlanMovieTitleSearchWithYear(t *testing.T) {
	got := Plan(single("movie", store.QueryHypothesis{Title: "Dune", Year: 2021}, nil),
		Options{Language: "zh-CN", Region: "CN"})
	if len(got) != 1 {
		t.Fatalf("expected one movie search, got %+v", got)
	}
	want := map[string]string{
		"query":                "Dune",
		"page":                 "1",
		"include_adult":        "false",
		"language":             "zh-CN",
		"region":               "CN",
		"primary_release_year": "2021",
		"year":                 "2021",
	}
	if !reflect.DeepEqual(got[0].Query, want) {
		t.Fatalf("query = %+v, want %+v", got[0].Query, want)
	}
	if got[0].Method != MethodGet || got[0].Path != PathSearchMovie {
		t.Fatalf("got %s %s", got[0].Method, got[0].Path)
	}
}

// Step 2, branch: tv-like hypothesis -> /search/tv with first_air_date_year.
func TestPlanTVTitleSearch(t *testing.T) {
	got := Plan(single("tv", store.QueryHypothesis{Title: "Breaking Bad", Year: 2008}, nil),
		Options{Language: "en-US", Region: "US"})
	if len(got) != 1 {
		t.Fatalf("expected one tv search, got %+v", got)
	}
	want := map[string]string{
		"query":               "Breaking Bad",
		"page":                "1",
		"include_adult":       "false",
		"language":            "en-US",
		"first_air_date_year": "2008",
		"year":                "2008",
	}
	if !reflect.DeepEqual(got[0].Query, want) {
		t.Fatalf("query = %+v, want %+v", got[0].Query, want)
	}
	// tv search takes no region per the plan.
	if _, hasRegion := got[0].Query["region"]; hasRegion {
		t.Fatalf("tv search must not carry region: %+v", got[0].Query)
	}
}

// Step 2, branch: unknown media type -> movie search first, tv search second.
func TestPlanUnknownTypeSearchesMovieThenTV(t *testing.T) {
	got := Plan(single("", store.QueryHypothesis{Title: "2046", Year: 2004}, nil), Options{})
	if len(got) != 2 {
		t.Fatalf("expected movie then tv search, got %+v", got)
	}
	if got[0].Path != PathSearchMovie || got[1].Path != PathSearchTV {
		t.Fatalf("expected movie then tv, got %s then %s", got[0].Path, got[1].Path)
	}
	if got[0].Rank != 1 || got[1].Rank != 2 {
		t.Fatalf("ranks wrong: %+v", got)
	}
}

// Regression: multiple hypotheses (most-constrained first) each produce a
// search, in order, deduplicated. Exercises the plural-hypotheses contract the
// single-hypothesis planner could not express.
func TestPlanMultipleHypothesesOrderedAndDeduped(t *testing.T) {
	in := PlanInput{
		Identity: store.ParsedIdentity{
			MediaTypeHint: "movie",
			Hypotheses: []store.QueryHypothesis{
				{Title: "Hero", Year: 2002, MediaType: "movie", Source: "rule"},
				{Title: "Ying Xiong", Year: 2002, MediaType: "movie", Source: "romanized"},
				{Title: "Hero", Year: 2002, MediaType: "movie", Source: "comparison_key"}, // dup of #1
			},
		},
	}
	got := Plan(in, Options{Language: "en-US"})
	if len(got) != 2 {
		t.Fatalf("expected two deduped searches, got %+v", got)
	}
	if got[0].Query["query"] != "Hero" || got[1].Query["query"] != "Ying Xiong" {
		t.Fatalf("order/dedupe wrong: %+v", got)
	}
}

// Regression: invalid external IDs are skipped, leaving only the title search.
func TestPlanSkipsInvalidExternalIDs(t *testing.T) {
	got := Plan(PlanInput{
		Identity: store.ParsedIdentity{
			MediaTypeHint: "movie",
			Hypotheses:    []store.QueryHypothesis{{Title: "Dune", Year: 2021, MediaType: "movie"}},
		},
		ExternalIDs: map[string]string{
			"tmdb": "not-numeric",
			"imdb": "nm0000000",
		},
	}, Options{})
	if len(got) != 1 {
		t.Fatalf("invalid IDs should be skipped, leaving only title search: %+v", got)
	}
	if got[0].Path != PathSearchMovie {
		t.Fatalf("got %+v", got)
	}
}

// Hypothesis-carried external IDs are honored even when PlanInput.ExternalIDs
// is empty, and per-hypothesis MediaType overrides the identity hint.
func TestPlanHypothesisExternalIDsAndMediaTypeOverride(t *testing.T) {
	in := PlanInput{
		Identity: store.ParsedIdentity{
			MediaTypeHint: "movie", // overridden by the hypothesis below
			Hypotheses: []store.QueryHypothesis{
				{
					Title:       "Sherlock",
					Year:        2010,
					MediaType:   "tv",
					ExternalIDs: map[string]string{"tvdb": "176941"},
					Source:      "rule",
				},
			},
		},
	}
	got := Plan(in, Options{Language: "en-US"})
	if len(got) != 2 {
		t.Fatalf("expected /find then tv search, got %+v", got)
	}
	if got[0].Path != PathFindID || got[0].Query["external_source"] != "tvdb_id" {
		t.Fatalf("first should be tvdb find: %+v", got[0])
	}
	if got[0].PathParams["external_id"] != "176941" {
		t.Fatalf("external_id = %q", got[0].PathParams["external_id"])
	}
	if got[1].Path != PathSearchTV {
		t.Fatalf("hypothesis MediaType=tv should route to tv search, got %s", got[1].Path)
	}
}
