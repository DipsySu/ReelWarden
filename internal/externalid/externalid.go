// Package externalid extracts explicit, embedded provider IDs from file and
// directory names (R1, authority §14.2). It consumes LOCAL UNTRUSTED data only
// (the path string) and performs no provider calls. Recognized forms:
//
//	[tmdbid-1234]   {tmdb-1234}
//	[imdbid-tt123]  {imdb-tt123}
//	{tvdb-123}      [tvdbid-123]
//
// Matching is deterministic; the resolver decides the final match (§14.1).
package externalid

import (
	"path/filepath"
	"regexp"
	"strings"
)

// Provider names returned in Match.Provider. These mirror the §13 provider IDs.
const (
	ProviderTMDB = "tmdb"
	ProviderIMDB = "imdb"
	ProviderTVDB = "tvdb"
)

// Source describes which path component an ID was found in.
const (
	SourceFileName = "file_name"
	SourceDirName  = "dir_name"
)

// Match is a single embedded external ID detected in a path component.
type Match struct {
	Provider string `json:"provider"` // tmdb | imdb | tvdb
	ID       string `json:"id"`       // raw id, e.g. "1234" or "tt0133093"
	Source   string `json:"source"`   // file_name | dir_name
}

// idRE matches the bracketed/braced ID tokens. The provider key tolerates an
// optional "id" suffix (tmdbid / tmdb). The value runs to the closing bracket.
var idRE = regexp.MustCompile(`(?i)[\[{]\s*(tmdb|imdb|tvdb)(?:id)?\s*[-:=]\s*([^\]}\s]+)\s*[\]}]`)

// Parse extracts every embedded external ID from a relative path. The file name
// (basename) is scanned first, then the immediate parent directory name, so the
// most file-specific IDs lead. Duplicate (provider,id) pairs are de-duplicated,
// preferring the file-name source. Invalid ID payloads are skipped (never
// panics). nil is returned when nothing valid is found.
func Parse(rel string) []Match {
	base := filepath.Base(rel)
	parent := filepath.Base(filepath.Dir(rel))

	var out []Match
	seen := map[string]bool{}
	add := func(text, source string) {
		for _, m := range scan(text) {
			m.Source = source
			key := m.Provider + "/" + m.ID
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, m)
		}
	}
	add(base, SourceFileName)
	if parent != "" && parent != "." && parent != string(filepath.Separator) {
		add(parent, SourceDirName)
	}
	return out
}

// scan returns the normalized, validated matches found in a single string.
func scan(text string) []Match {
	var out []Match
	for _, sm := range idRE.FindAllStringSubmatch(text, -1) {
		provider := strings.ToLower(sm[1])
		id := normalizeID(provider, sm[2])
		if id == "" {
			continue // malformed payload -> skip, do not fabricate
		}
		out = append(out, Match{Provider: provider, ID: id})
	}
	return out
}

var (
	imdbRE    = regexp.MustCompile(`^tt[0-9]+$`)
	numericRE = regexp.MustCompile(`^[0-9]+$`)
)

// normalizeID validates a raw ID for its provider and returns the canonical
// form, or "" if it is not a valid ID for that provider. IMDb IDs are
// lower-cased "tt"+digits; TMDB/TVDB IDs are positive integers.
func normalizeID(provider, raw string) string {
	raw = strings.TrimSpace(raw)
	switch provider {
	case ProviderIMDB:
		lowered := strings.ToLower(raw)
		if imdbRE.MatchString(lowered) {
			return lowered
		}
	case ProviderTMDB, ProviderTVDB:
		if numericRE.MatchString(raw) {
			return raw
		}
	}
	return ""
}
