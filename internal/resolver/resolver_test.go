package resolver

import (
	"context"
	"errors"
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
