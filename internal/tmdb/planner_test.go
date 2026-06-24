package tmdb

import (
	"reflect"
	"testing"

	"github.com/reelwarden/reelwarden/internal/store"
)

func TestPlanPrefersExternalIDLookup(t *testing.T) {
	got := Plan(store.QueryHypothesis{
		Title:     "The Matrix",
		Year:      1999,
		MediaType: "movie",
		ExternalIDs: map[string]string{
			"imdb": "tt0133093",
		},
	}, Options{Language: "zh-CN", Region: "CN"})

	if len(got) < 2 {
		t.Fatalf("expected exact ID lookup plus title fallback, got %+v", got)
	}
	first := got[0]
	if first.PathTemplate != EndpointFindID {
		t.Fatalf("first path = %s, want %s", first.PathTemplate, EndpointFindID)
	}
	if first.PathParams["external_id"] != "tt0133093" {
		t.Fatalf("external_id = %q", first.PathParams["external_id"])
	}
	if first.Query["external_source"] != "imdb_id" {
		t.Fatalf("external_source = %q", first.Query["external_source"])
	}
	if first.Reason != "exact_imdb_id" || first.Rank != 1 {
		t.Fatalf("first request metadata wrong: %+v", first)
	}
	if got[1].PathTemplate != EndpointSearchMovie {
		t.Fatalf("fallback path = %s, want movie search", got[1].PathTemplate)
	}
}

func TestPlanMovieSearchUsesConstrainedParams(t *testing.T) {
	got := Plan(store.QueryHypothesis{
		Title:     "Dune",
		Year:      2021,
		MediaType: "movie",
	}, Options{Language: "zh-CN", Region: "CN"})
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
}

func TestPlanTVSearchUsesAirDateParams(t *testing.T) {
	got := Plan(store.QueryHypothesis{
		Title:     "Breaking Bad",
		Year:      2008,
		MediaType: "tv",
	}, Options{Language: "en-US"})
	if len(got) != 2 {
		t.Fatalf("expected tv search plus movie fallback, got %+v", got)
	}
	if got[0].PathTemplate != EndpointSearchTV {
		t.Fatalf("first path = %s, want %s", got[0].PathTemplate, EndpointSearchTV)
	}
	if got[0].Query["first_air_date_year"] != "2008" || got[0].Query["year"] != "2008" {
		t.Fatalf("tv year params missing: %+v", got[0].Query)
	}
}

func TestPlanTMDBIDUsesMediaTypedDetail(t *testing.T) {
	got := Plan(store.QueryHypothesis{
		MediaType:   "tv",
		ExternalIDs: map[string]string{"tmdb": "1396"},
	}, Options{Language: "en-US"})
	if len(got) != 2 {
		t.Fatalf("expected tv detail plus movie fallback for tv-like hint, got %+v", got)
	}
	if got[0].PathTemplate != EndpointTVDetail {
		t.Fatalf("first path = %s, want %s", got[0].PathTemplate, EndpointTVDetail)
	}
	if got[0].PathParams["tmdb_id"] != "1396" {
		t.Fatalf("tmdb_id = %q", got[0].PathParams["tmdb_id"])
	}
}

func TestPlanUnknownTypeSearchesMovieThenTV(t *testing.T) {
	got := Plan(store.QueryHypothesis{Title: "2046", Year: 2004}, Options{})
	if len(got) != 2 {
		t.Fatalf("expected movie and tv searches, got %+v", got)
	}
	if got[0].PathTemplate != EndpointSearchMovie || got[1].PathTemplate != EndpointSearchTV {
		t.Fatalf("expected movie then tv search, got %+v", got)
	}
}

func TestPlanSkipsInvalidExternalIDs(t *testing.T) {
	got := Plan(store.QueryHypothesis{
		Title:     "Dune",
		Year:      2021,
		MediaType: "movie",
		ExternalIDs: map[string]string{
			"tmdb": "not-numeric",
			"imdb": "nm0000000",
		},
	}, Options{})
	if len(got) != 1 {
		t.Fatalf("invalid IDs should be skipped, leaving only title search: %+v", got)
	}
	if got[0].PathTemplate != EndpointSearchMovie {
		t.Fatalf("got %+v", got)
	}
}
