// Package tmdb is the deterministic TMDB query planner (resolver-pipeline.md,
// "TMDB deterministic query plan"). It is a PURE planner: it emits ordered
// request plans (method, path, query params) and performs NO HTTP and makes NO
// network/provider/AI calls. The real adapter executes these plans later.
//
// Compliance boundary: this package is never called by AI and its output is
// never sent to AI (authority §7.1/§7.2). It consumes store.ParsedIdentity and
// local-only external IDs; it never imports internal/resolver and never accepts
// provider-returned content.
package tmdb

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/reelwarden/reelwarden/internal/store"
)

const (
	// MethodGet is the only HTTP method the deterministic plan emits.
	MethodGet = "GET"

	PathFindID      = "/find/{external_id}"
	PathMovieDetail = "/movie/{tmdb_id}"
	PathTVDetail    = "/tv/{tmdb_id}"
	PathSearchMovie = "/search/movie"
	PathSearchTV    = "/search/tv"
)

// Options are the locale and safety knobs that map directly to documented TMDB
// request parameters. Empty fields are omitted from the emitted query.
type Options struct {
	Language string // maps to ?language
	Region   string // maps to ?region (movie search / find only per the plan)
}

// PlanInput is the small local input struct the planner consumes. It is built
// from store types only (never internal/resolver). Identity supplies the
// MediaTypeHint and the ordered Hypotheses (most-constrained first, §12.6);
// ExternalIDs are local-only exact IDs detected in filenames/NFO (R1).
//
// ExternalIDs may be supplied here and/or carried per-hypothesis
// (QueryHypothesis.ExternalIDs); both are merged before planning. All IDs are
// local untrusted facts — no provider content is ever accepted.
type PlanInput struct {
	Identity    store.ParsedIdentity
	ExternalIDs map[string]string
}

// RequestPlan is one planned TMDB API request. Path keeps IDs out of string
// concatenation (PathParams) until the adapter executes it; Query holds already
// deterministic, sanitized values. Method is always GET. Rank is the 1-based
// position in the most-authoritative-first ordering.
type RequestPlan struct {
	Method     string            `json:"method"`
	Path       string            `json:"path"`
	PathParams map[string]string `json:"path_params,omitempty"`
	Query      map[string]string `json:"query,omitempty"`
	Reason     string            `json:"reason"`
	Rank       int               `json:"rank"`
}

// Plan returns the ordered TMDB request plans, most-authoritative first:
//  1. local exact IDs first (resolver-pipeline.md step 1):
//     local tmdb ID -> /movie/{id} or /tv/{id} chosen by MediaTypeHint;
//     local imdb/tvdb/wikidata ID -> /find/{external_id} with external_source;
//  2. title fallback only when needed (step 2), one search per hypothesis,
//     constrained by media type + release/air year + locale.
//
// The result is deterministic: identical inputs always yield identical,
// duplicate-free output.
func Plan(in PlanInput, opts Options) []RequestPlan {
	var out []RequestPlan
	out = append(out, exactIDRequests(in, opts)...)
	out = append(out, titleSearchRequests(in, opts)...)

	out = dedupe(out)
	for i := range out {
		out[i].Rank = i + 1
	}
	return out
}

// exactIDRequests implements step 1. The tmdb-id detail endpoint is chosen
// strictly by MediaTypeHint; it never fabricates an opposite-type detail call
// for an explicit hint. /find requests are emitted in a deterministic order.
func exactIDRequests(in PlanInput, opts Options) []RequestPlan {
	ids := cleanExternalIDs(mergeExternalIDs(in))
	if len(ids) == 0 {
		return nil
	}
	var out []RequestPlan

	if id := ids["tmdb"]; id != "" {
		for _, path := range tmdbDetailPaths(in.Identity.MediaTypeHint) {
			out = append(out, RequestPlan{
				Method:     MethodGet,
				Path:       path,
				PathParams: map[string]string{"tmdb_id": id},
				Query:      localeQuery(opts),
				Reason:     "exact_tmdb_id",
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
			q := localeQuery(opts)
			q["external_source"] = spec.externalSource
			out = append(out, RequestPlan{
				Method:     MethodGet,
				Path:       PathFindID,
				PathParams: map[string]string{"external_id": id},
				Query:      q,
				Reason:     "exact_" + spec.key + "_id",
			})
		}
	}
	return out
}

// titleSearchRequests implements step 2. It walks every hypothesis (already
// ordered most-constrained first) and emits a search per media-type branch.
func titleSearchRequests(in PlanInput, opts Options) []RequestPlan {
	var out []RequestPlan
	for _, h := range in.Identity.Hypotheses {
		title := strings.TrimSpace(h.Title)
		if title == "" {
			continue
		}
		// A hypothesis may carry its own media type; fall back to the
		// identity-level hint when it does not.
		mediaType := h.MediaType
		if mediaType == "" {
			mediaType = in.Identity.MediaTypeHint
		}
		for _, path := range searchPaths(mediaType) {
			out = append(out, searchRequest(path, title, h.Year, opts))
		}
	}
	return out
}

func searchRequest(path, title string, year int, opts Options) RequestPlan {
	q := map[string]string{
		"query":         title,
		"include_adult": "false",
		"page":          "1",
	}
	if opts.Language != "" {
		q["language"] = opts.Language
	}
	switch path {
	case PathSearchMovie:
		if opts.Region != "" {
			q["region"] = opts.Region
		}
		if year != 0 {
			q["primary_release_year"] = strconv.Itoa(year)
			q["year"] = strconv.Itoa(year)
		}
	case PathSearchTV:
		if year != 0 {
			q["first_air_date_year"] = strconv.Itoa(year)
			q["year"] = strconv.Itoa(year)
		}
	}
	return RequestPlan{
		Method: MethodGet,
		Path:   path,
		Query:  q,
		Reason: "title_year_media_type",
	}
}

// localeQuery builds the locale query shared by ID lookups (language only; the
// /find and detail endpoints take no region).
func localeQuery(opts Options) map[string]string {
	q := map[string]string{}
	if opts.Language != "" {
		q["language"] = opts.Language
	}
	return q
}

// tmdbDetailPaths picks the detail endpoint for a local tmdb id by MediaTypeHint
// (resolver-pipeline.md step 1: "/movie/{id} or /tv/{id} based on MediaTypeHint").
// Anime/special live-action subtypes are tv-like. An unknown hint cannot be
// resolved to one endpoint deterministically, so it falls back to the
// unknown-type ordering (movie detail first, then tv detail).
func tmdbDetailPaths(mediaType string) []string {
	switch mediaType {
	case "movie":
		return []string{PathMovieDetail}
	case "tv", "tv_liveaction", "ova", "special":
		return []string{PathTVDetail}
	default:
		return []string{PathMovieDetail, PathTVDetail}
	}
}

// searchPaths picks the title-search endpoint(s) by media type (step 2):
// movie-like -> movie search; tv-like -> tv search; unknown -> movie then tv.
func searchPaths(mediaType string) []string {
	switch mediaType {
	case "movie":
		return []string{PathSearchMovie}
	case "tv", "tv_liveaction", "ova", "special":
		return []string{PathSearchTV}
	default:
		return []string{PathSearchMovie, PathSearchTV}
	}
}

// mergeExternalIDs combines top-level PlanInput.ExternalIDs with any IDs carried
// on the hypotheses. Top-level IDs win on conflict; hypotheses are consulted in
// their most-constrained-first order.
func mergeExternalIDs(in PlanInput) map[string]string {
	merged := map[string]string{}
	for _, h := range in.Identity.Hypotheses {
		for k, v := range h.ExternalIDs {
			if _, ok := merged[k]; !ok {
				merged[k] = v
			}
		}
	}
	for k, v := range in.ExternalIDs {
		merged[k] = v
	}
	return merged
}

// cleanExternalIDs lowercases keys, trims values, validates each ID against its
// documented shape, and returns only well-formed local IDs.
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

// dedupe removes structurally identical plans while preserving first-seen order
// so the most-authoritative request is kept.
func dedupe(plans []RequestPlan) []RequestPlan {
	seen := map[string]struct{}{}
	out := plans[:0]
	for _, p := range plans {
		key := planKey(p)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, p)
	}
	return out
}

func planKey(p RequestPlan) string {
	var b strings.Builder
	b.WriteString(p.Method)
	b.WriteByte('|')
	b.WriteString(p.Path)
	b.WriteByte('|')
	writeSortedMap(&b, p.PathParams)
	b.WriteByte('|')
	writeSortedMap(&b, p.Query)
	return b.String()
}

func writeSortedMap(b *strings.Builder, m map[string]string) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(m[k])
		b.WriteByte(';')
	}
}

var (
	numericIDRE  = regexp.MustCompile(`^[0-9]+$`)
	imdbIDRE     = regexp.MustCompile(`^tt[0-9]+$`)
	wikidataIDRE = regexp.MustCompile(`^Q[0-9]+$`)
)
