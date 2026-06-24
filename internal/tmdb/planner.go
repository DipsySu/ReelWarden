// Package tmdb contains deterministic TMDB integration helpers. This file is
// intentionally a query planner only: it never performs network I/O and never
// calls AI. The real adapter can execute these requests later.
package tmdb

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/reelwarden/reelwarden/internal/store"
)

const (
	EndpointFindID      = "/find/{external_id}"
	EndpointMovieDetail = "/movie/{tmdb_id}"
	EndpointTVDetail    = "/tv/{tmdb_id}"
	EndpointSearchMovie = "/search/movie"
	EndpointSearchTV    = "/search/tv"
)

// Options are the locale and safety knobs that map directly to documented TMDB
// request parameters. Empty fields are omitted.
type Options struct {
	Language     string
	Region       string
	IncludeAdult bool
}

// Request is one planned TMDB API request. PathTemplate keeps IDs out of string
// concatenation until the adapter executes it; PathParams and Query are already
// sanitized, deterministic values.
type Request struct {
	PathTemplate string            `json:"path_template"`
	PathParams   map[string]string `json:"path_params,omitempty"`
	Query        map[string]string `json:"query,omitempty"`
	Reason       string            `json:"reason"`
	Rank         int               `json:"rank"`
}

// Plan returns TMDB requests ordered most-constrained first:
//  1. exact external ID lookup from local data (TMDB detail or /find);
//  2. title search constrained by media type + release/air year + locale.
//
// It accepts store.QueryHypothesis so the resolver can pass R0/R1/R4 hypotheses
// directly. ExternalIDs are local-only facts extracted from filenames/NFO; no
// provider-returned content is ever accepted here.
func Plan(h store.QueryHypothesis, opts Options) []Request {
	var out []Request
	out = append(out, exactIDRequests(h, opts)...)
	out = append(out, titleSearchRequests(h, opts)...)
	for i := range out {
		out[i].Rank = i + 1
	}
	return out
}

func exactIDRequests(h store.QueryHypothesis, opts Options) []Request {
	if len(h.ExternalIDs) == 0 {
		return nil
	}
	var out []Request
	ids := cleanExternalIDs(h.ExternalIDs)

	if id := ids["tmdb"]; id != "" {
		for _, endpoint := range tmdbDetailEndpoints(h.MediaType) {
			out = append(out, Request{
				PathTemplate: endpoint,
				PathParams:   map[string]string{"tmdb_id": id},
				Query:        localeQuery(opts, false),
				Reason:       "exact_tmdb_id",
			})
		}
	}

	for _, spec := range []struct {
		key            string
		externalSource string
	}{
		{"imdb", "imdb_id"},
		{"tvdb", "tvdb_id"},
		{"wikidata", "wikidata_id"},
	} {
		if id := ids[spec.key]; id != "" {
			q := localeQuery(opts, false)
			q["external_source"] = spec.externalSource
			out = append(out, Request{
				PathTemplate: EndpointFindID,
				PathParams:   map[string]string{"external_id": id},
				Query:        q,
				Reason:       "exact_" + spec.key + "_id",
			})
		}
	}
	return out
}

func titleSearchRequests(h store.QueryHypothesis, opts Options) []Request {
	title := strings.TrimSpace(h.Title)
	if title == "" {
		return nil
	}
	var out []Request
	for _, endpoint := range searchEndpoints(h.MediaType) {
		q := localeQuery(opts, true)
		q["query"] = title
		q["page"] = "1"
		q["include_adult"] = strconv.FormatBool(opts.IncludeAdult)
		if h.Year != 0 {
			switch endpoint {
			case EndpointSearchMovie:
				q["primary_release_year"] = strconv.Itoa(h.Year)
				q["year"] = strconv.Itoa(h.Year)
				if opts.Region != "" {
					q["region"] = opts.Region
				}
			case EndpointSearchTV:
				q["first_air_date_year"] = strconv.Itoa(h.Year)
				q["year"] = strconv.Itoa(h.Year)
			}
		} else if endpoint == EndpointSearchMovie && opts.Region != "" {
			q["region"] = opts.Region
		}
		out = append(out, Request{
			PathTemplate: endpoint,
			Query:        q,
			Reason:       "title_year_media_type",
		})
	}
	return out
}

func localeQuery(opts Options, includeRegion bool) map[string]string {
	q := map[string]string{}
	if opts.Language != "" {
		q["language"] = opts.Language
	}
	if includeRegion && opts.Region != "" {
		q["region"] = opts.Region
	}
	return q
}

func tmdbDetailEndpoints(mediaType string) []string {
	switch mediaType {
	case "tv", "tv_liveaction", "ova", "special":
		return []string{EndpointTVDetail, EndpointMovieDetail}
	case "movie":
		return []string{EndpointMovieDetail}
	default:
		return []string{EndpointMovieDetail, EndpointTVDetail}
	}
}

func searchEndpoints(mediaType string) []string {
	switch mediaType {
	case "tv", "tv_liveaction", "ova", "special":
		return []string{EndpointSearchTV, EndpointSearchMovie}
	case "movie":
		return []string{EndpointSearchMovie}
	default:
		return []string{EndpointSearchMovie, EndpointSearchTV}
	}
}

func cleanExternalIDs(ids map[string]string) map[string]string {
	out := map[string]string{}
	type pair struct{ key, value string }
	pairs := make([]pair, 0, len(ids))
	for k, v := range ids {
		pairs = append(pairs, pair{
			key:   strings.ToLower(strings.TrimSpace(k)),
			value: strings.TrimSpace(v),
		})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].key < pairs[j].key })
	for _, p := range pairs {
		k, v := p.key, p.value
		if v == "" {
			continue
		}
		switch k {
		case "tmdb", "tvdb":
			if numericIDRE.MatchString(v) {
				out[k] = v
			}
		case "imdb":
			v = strings.ToLower(v)
			if imdbIDRE.MatchString(v) {
				out[k] = v
			}
		case "wikidata":
			v = strings.ToUpper(v)
			if wikidataIDRE.MatchString(v) {
				out[k] = v
			}
		}
	}
	return out
}

var (
	numericIDRE  = regexp.MustCompile(`^[0-9]+$`)
	imdbIDRE     = regexp.MustCompile(`^tt[0-9]+$`)
	wikidataIDRE = regexp.MustCompile(`^Q[0-9]+$`)
)
