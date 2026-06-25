package resolver

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/reelwarden/reelwarden/internal/matcher"
	"github.com/reelwarden/reelwarden/internal/store"
)

// fakeQuery returns one exact-title candidate for the given title/year so the
// deterministic R3 scorer can reach a band without any network or ffprobe.
func fakeQuery(title string, year int) ProviderQuery {
	return func(_ context.Context, h store.QueryHypothesis) ([]matcher.ProviderCandidate, error) {
		if h.Title == "" {
			return nil, nil
		}
		return []matcher.ProviderCandidate{{
			Provider:       "mock",
			ProviderItemID: "mock:1",
			MediaType:      matcher.MediaMovie,
			Title:          title,
			Year:           year,
		}}, nil
	}
}

func TestResolve_CleanTitleResolves(t *testing.T) {
	a := store.MediaAsset{ID: "asset_1"}
	out := ResolveAsset(Input{
		Asset:   a,
		RelPath: "Dune (2021)/Dune.2021.2160p.BluRay.x265.mkv",
		Query:   fakeQuery("Dune", 2021),
	})
	if out.State != "resolved" {
		t.Fatalf("expected resolved, got %q (band=%s score=%.3f stopped=%s)", out.State, out.Band, out.RankScore, out.StoppedAt)
	}
	if out.Best == nil {
		t.Fatal("expected a preselected best candidate")
	}
	if out.Band != store.BandHigh && out.Band != store.BandMedium {
		t.Fatalf("expected high/medium band, got %s", out.Band)
	}
	// §14.7: resolved is a preselection only; the identity must not be confirmed.
	if out.Identity.RawTitle != "Dune" {
		t.Fatalf("expected raw title 'Dune', got %q", out.Identity.RawTitle)
	}
}

func TestResolve_NoProviderIsUnresolved(t *testing.T) {
	// No provider door -> no candidates -> unresolved with hypotheses (§12.6).
	out := ResolveAsset(Input{
		Asset:   store.MediaAsset{ID: "asset_2"},
		RelPath: "Some Obscure Film (1999).mkv",
	})
	if out.State != "unresolved" {
		t.Fatalf("expected unresolved, got %q", out.State)
	}
	if len(out.Hypotheses) == 0 {
		t.Fatal("expected hypotheses surfaced for human review")
	}
	if out.StoppedAt != "" {
		t.Fatalf("expected ladder to run to the end (no Stop), stopped at %q", out.StoppedAt)
	}
}

func TestResolve_ProviderFailureIsolated(t *testing.T) {
	// A provider error must never break the resolve; the file ends unresolved
	// with a degraded-evidence note (§0.2/§11.5).
	failing := func(_ context.Context, _ store.QueryHypothesis) ([]matcher.ProviderCandidate, error) {
		return nil, errors.New("META_PROVIDER_UNAVAILABLE: boom")
	}
	out := ResolveAsset(Input{
		Asset:   store.MediaAsset{ID: "asset_3"},
		RelPath: "Heat (1995).mkv",
		Query:   failing,
	})
	if out.State != "unresolved" {
		t.Fatalf("expected unresolved on provider failure, got %q", out.State)
	}
	foundDegraded := false
	for _, e := range out.Evidence {
		if e.Code == "META_PROVIDER_UNAVAILABLE" {
			foundDegraded = true
		}
	}
	if !foundDegraded {
		t.Fatal("expected an isolated provider-failure evidence note")
	}
}

func TestResolve_LocalExternalIDReachesProviderQuery(t *testing.T) {
	var sawID bool
	query := func(_ context.Context, h store.QueryHypothesis) ([]matcher.ProviderCandidate, error) {
		if h.ExternalIDs["imdb"] == "tt0133093" {
			sawID = true
		}
		return []matcher.ProviderCandidate{{
			Provider:       "mock",
			ProviderItemID: "603",
			MediaType:      matcher.MediaMovie,
			Title:          "The Matrix",
			Year:           1999,
			ExternalIDs:    map[string]string{"imdb": "tt0133093"},
		}}, nil
	}
	out := ResolveAsset(Input{
		Asset:   store.MediaAsset{ID: "asset_id"},
		RelPath: "The.Matrix.1999.[imdbid-tt0133093].mkv",
		Query:   query,
	})
	if !sawID {
		t.Fatal("expected local external ID to be forwarded on the provider hypothesis")
	}
	if out.State != "resolved" {
		t.Fatalf("expected resolved by ID-constrained provider result, got %q", out.State)
	}
}

// errLLM always errors, exercising the R4 degraded path without a real model.
type errLLM struct{}

func (errLLM) Complete(context.Context, string) (string, error) {
	return "", errors.New("model unavailable")
}

func TestResolve_AIRepairFailureStillUnresolved(t *testing.T) {
	// Low band (provider returns a non-matching title) + a failing local AI must
	// degrade gracefully to unresolved, never panic or block (§0.2).
	out := ResolveAsset(Input{
		Asset:   store.MediaAsset{ID: "asset_4"},
		RelPath: "garbled (2010).mkv",
		Query:   fakeQuery("Totally Different Movie", 1980),
		LLM:     errLLM{},
	})
	if out.State != "unresolved" {
		t.Fatalf("expected unresolved, got %q (band=%s)", out.State, out.Band)
	}
}

func TestResolve_NeverAutoConfirms(t *testing.T) {
	// Even a perfect match never sets a confirmed match_state in the resolver.
	out := ResolveAsset(Input{
		Asset:   store.MediaAsset{ID: "asset_5"},
		RelPath: "Dune (2021).mkv",
		Query:   fakeQuery("Dune", 2021),
	})
	if out.Identity.State == "confirmed" {
		t.Fatal("resolver must never confirm (§14.7)")
	}
}

// --- regression: resolver.go:191 -- unresolved must carry Best==nil -----------

// nonMatchQuery returns one unrelated low-scoring candidate so the ladder reaches
// the low band (unresolved) but still has a scored candidate in State.Best.
func nonMatchQuery() ProviderQuery {
	return func(_ context.Context, h store.QueryHypothesis) ([]matcher.ProviderCandidate, error) {
		if h.Title == "" {
			return nil, nil
		}
		return []matcher.ProviderCandidate{{
			Provider:       "mock",
			ProviderItemID: "mock:zzz",
			MediaType:      matcher.MediaMovie,
			Title:          "Completely Unrelated Title",
			Year:           1971,
		}}, nil
	}
}

func TestResolve_UnresolvedHasNilBest(t *testing.T) {
	// A single unrelated low-scoring candidate must yield State "unresolved" AND a
	// nil Best: an unresolved result must never advertise a score-0 preselection
	// (resolver.go:191). The candidate is still kept in the human-review list.
	out := ResolveAsset(Input{
		Asset:   store.MediaAsset{ID: "asset_lowband"},
		RelPath: "Some Specific Movie (2008).mkv",
		Query:   nonMatchQuery(),
	})
	if out.State != "unresolved" {
		t.Fatalf("expected unresolved for a single unrelated candidate, got %q (band=%s score=%.3f)", out.State, out.Band, out.RankScore)
	}
	if out.Best != nil {
		t.Fatalf("unresolved result must have Best==nil, got %+v", *out.Best)
	}
	if len(out.Candidates) == 0 {
		t.Fatal("the unrelated candidate must still be surfaced for human review")
	}
}

// --- regression: resolver.go:267 -- R1 rebuilds Hypotheses from NFO title ------

func TestResolve_R1RebuildsHypothesesFromNFO(t *testing.T) {
	// An empty-name file (all technical tags, no parseable title) plus a valid NFO
	// <title>: R0 left Hypotheses nil, so without rebuilding them R3 would issue no
	// provider query. After recovering RawTitle from the NFO, Hypotheses must be
	// rebuilt so a provider query is issued (resolver.go:267).
	dir := t.TempDir()
	media := filepath.Join(dir, "1080p.BluRay.x265.mkv") // parses to an empty title
	if err := os.WriteFile(media, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	nfoPath := filepath.Join(dir, "1080p.BluRay.x265.nfo")
	nfoXML := `<movie><title>Blade Runner</title><year>1982</year></movie>`
	if err := os.WriteFile(nfoPath, []byte(nfoXML), 0o600); err != nil {
		t.Fatal(err)
	}

	var queriedTitles []string
	query := func(_ context.Context, h store.QueryHypothesis) ([]matcher.ProviderCandidate, error) {
		if h.Title == "" {
			return nil, nil
		}
		queriedTitles = append(queriedTitles, h.Title)
		return []matcher.ProviderCandidate{{
			Provider:       "mock",
			ProviderItemID: "mock:br",
			MediaType:      matcher.MediaMovie,
			Title:          "Blade Runner",
			Year:           1982,
		}}, nil
	}

	out := ResolveAsset(Input{
		Asset:   store.MediaAsset{ID: "asset_nfotitle"},
		AbsPath: media,
		RelPath: "1080p.BluRay.x265.mkv",
		Query:   query,
	})

	if out.Identity.RawTitle != "Blade Runner" {
		t.Fatalf("expected RawTitle recovered from NFO, got %q", out.Identity.RawTitle)
	}
	if len(out.Identity.Hypotheses) == 0 {
		t.Fatal("expected rebuilt hypotheses after recovering the NFO title")
	}
	if len(queriedTitles) == 0 {
		t.Fatal("expected a provider query to be issued from the rebuilt hypotheses")
	}
	if out.State != "resolved" {
		t.Fatalf("expected resolved via the rebuilt-hypothesis query, got %q (band=%s)", out.State, out.Band)
	}
}

// --- regression: resolver.go:345 -- R2 invalidates a stale R1 score ------------

// idNoMatchQuery answers an external-ID-carrying file with a movie candidate that
// matches the title/year (medium band) but does NOT carry the local tmdb id, so
// no authoritative-ID promotion fires and R1 scores without stopping. This lets
// R2 run and refine the media-type hint.
func idNoMatchQuery() ProviderQuery {
	return func(_ context.Context, h store.QueryHypothesis) ([]matcher.ProviderCandidate, error) {
		if h.Title == "" {
			return nil, nil
		}
		return []matcher.ProviderCandidate{{
			Provider:       "mock",
			ProviderItemID: "mock:tvshow",
			MediaType:      matcher.MediaMovie, // movie candidate
			Title:          "Detective Story",
			Year:           2014,
		}}, nil
	}
}

func TestResolve_R2RefinedHintReScores(t *testing.T) {
	// R1 scores a movie candidate (st.scored=true). R2 then refines MediaTypeHint to
	// "tv" from the S01E01 token. R3's "if !st.scored" guard would otherwise reuse
	// the stale pre-R2 movie band; instead R2 must invalidate the cached score so R3
	// re-applies the §14.3 media-type hard constraint -> the movie candidate is
	// hard-filtered -> unresolved (resolver.go:345).
	out := ResolveAsset(Input{
		Asset: store.MediaAsset{ID: "asset_r2stale"},
		// Carries an external ID (so R1 scores) AND a TV token (so R2 refines the
		// hint to "tv"). The candidate is a movie that does not carry the id.
		RelPath: "Detective Story S01E01 (2014) [tmdbid-999].mkv",
		Query:   idNoMatchQuery(),
	})
	if out.State != "unresolved" {
		t.Fatalf("expected unresolved after R2 refines hint to tv and R3 re-applies the media-type constraint, got %q (band=%s score=%.3f stopped=%s)",
			out.State, out.Band, out.RankScore, out.StoppedAt)
	}
	if out.Identity.MediaTypeHint != "tv" {
		t.Fatalf("expected R2 to refine the media-type hint to tv, got %q", out.Identity.MediaTypeHint)
	}
}

// --- regression: resolver.go:460 -- R4 merges R3 + R4 candidates ---------------

// splitQuery returns the R3 candidate for the parsed (garbled) title and a second,
// different candidate for the AI-repaired title. The human-review list after R4
// must contain BOTH, never shrink to only the AI hit.
func splitQuery(garbledTitle, repairedTitle string) ProviderQuery {
	return func(_ context.Context, h store.QueryHypothesis) ([]matcher.ProviderCandidate, error) {
		switch h.Title {
		case garbledTitle:
			return []matcher.ProviderCandidate{{
				Provider: "mock", ProviderItemID: "r3:cand",
				MediaType: matcher.MediaMovie, Title: "R3 Weak Candidate", Year: 1990,
			}}, nil
		case repairedTitle:
			return []matcher.ProviderCandidate{{
				Provider: "mock", ProviderItemID: "r4:cand",
				MediaType: matcher.MediaMovie, Title: repairedTitle, Year: 2011,
			}}, nil
		default:
			return nil, nil
		}
	}
}

// repairLLM emits one repaired hypothesis with the given title (local-only).
type repairLLM struct{ repaired string }

func (m repairLLM) Complete(context.Context, string) (string, error) {
	return `{"hypotheses":[{"title":"` + m.repaired + `","media_type_hint":"movie"}]}`, nil
}

func TestResolve_R4MergesCandidates(t *testing.T) {
	// R3 produced a (weak) candidate from the garbled title; R4's AI hypothesis adds
	// a different candidate. After R4 re-scores, the human-review candidate list must
	// contain BOTH provider items, not just the AI hit (resolver.go:460).
	garbled := "garbled"
	repaired := "Drive"
	out := ResolveAsset(Input{
		Asset:   store.MediaAsset{ID: "asset_r4merge"},
		RelPath: "garbled (2011).mkv",
		Query:   splitQuery(garbled, repaired),
		LLM:     repairLLM{repaired: repaired},
	})

	gotR3, gotR4 := false, false
	for _, c := range out.Candidates {
		switch c.ProviderID {
		case "r3:cand":
			gotR3 = true
		case "r4:cand":
			gotR4 = true
		}
	}
	if !gotR3 {
		t.Fatalf("R3's candidate was dropped after R4 re-score; list must not shrink. got %d candidates", len(out.Candidates))
	}
	if !gotR4 {
		t.Fatalf("R4's AI-hypothesis candidate is missing. got %d candidates", len(out.Candidates))
	}
}

// --- regression: resolver.go:286 -- exact external-ID match is authoritative ---

// garbledIDQuery returns a candidate whose TITLE does not match the (garbled)
// filename at all, but which carries the SAME tmdb id embedded in the filename.
// Title/year evidence alone keeps it well below the high threshold; only treating
// the exact ID match as authoritative can reach BandHigh.
func garbledIDQuery() ProviderQuery {
	return func(_ context.Context, h store.QueryHypothesis) ([]matcher.ProviderCandidate, error) {
		return []matcher.ProviderCandidate{{
			Provider:       "mock",
			ProviderItemID: "603",
			MediaType:      matcher.MediaMovie,
			Title:          "The Matrix", // deliberately unrelated to the garbled file name
			Year:           1999,
			ExternalIDs:    map[string]string{"tmdb": "603"},
		}}, nil
	}
}

func TestResolve_ExactExternalIDIsAuthoritative(t *testing.T) {
	// A [tmdbid-603] file with a garbled title must resolve via the ID alone: the
	// soft external-ID weight cannot reach 0.95, so without the §14.2 ID
	// short-circuit the file would stay low/unresolved (resolver.go:286). With the
	// fix the exact local<->candidate ID match is authoritative -> BandHigh -> Stop.
	out := ResolveAsset(Input{
		Asset:   store.MediaAsset{ID: "asset_authid"},
		RelPath: "低zhi商犯罪.乱码标题 [tmdbid-603].mkv",
		Query:   garbledIDQuery(),
	})
	if out.State != "resolved" {
		t.Fatalf("expected resolved via authoritative external-ID match, got %q (band=%s score=%.3f stopped=%s)",
			out.State, out.Band, out.RankScore, out.StoppedAt)
	}
	if out.Band != store.BandHigh {
		t.Fatalf("expected BandHigh from the authoritative ID match, got %s", out.Band)
	}
	if out.StoppedAt != "R1" {
		t.Fatalf("expected the ID short-circuit to Stop at R1, stopped at %q", out.StoppedAt)
	}
	if out.Best == nil || out.Best.ProviderID != "603" {
		t.Fatalf("expected the ID-matched candidate (603) preselected, got %+v", out.Best)
	}
	// §14.7: still a preselection, never auto-confirmed.
	if out.Identity.State == "confirmed" {
		t.Fatal("authoritative ID match must still be human-confirm, never confirmed (§14.7)")
	}
}
