// Package nfo discovers and parses sidecar Kodi/Jellyfin-style .nfo files (R1,
// authority §14.2, §7.2). NFO content is LOCAL UNTRUSTED data: it is parsed
// only for local fields (the <uniqueid> provider IDs the user already stored
// locally, and the local <title>). It performs no network access and never
// passes content to the Provider. Malformed XML is tolerated: Parse returns
// ok=false and never panics.
package nfo

import (
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Provider names mirror the externalid package / §13 provider IDs.
const (
	ProviderTMDB = "tmdb"
	ProviderIMDB = "imdb"
	ProviderTVDB = "tvdb"
)

// UniqueID is one <uniqueid type="..."> entry detected in an NFO.
type UniqueID struct {
	Provider string `json:"provider"` // tmdb | imdb | tvdb | <other, lower-cased>
	ID       string `json:"id"`       // raw value
	Default  bool   `json:"default"`  // the entry marked default="true"
}

// Info is the local, untrusted data extracted from an NFO sidecar. It carries
// only fields the user authored locally; no provider-derived content.
type Info struct {
	Path      string     `json:"path"`            // the .nfo file that was parsed
	Title     string     `json:"title,omitempty"` // local <title> (display-candidate, untrusted)
	UniqueIDs []UniqueID `json:"unique_ids"`      // detected <uniqueid> entries
	Year      int        `json:"year,omitempty"`  // local <year>, 0 if absent/invalid
	OK        bool       `json:"ok"`              // false on malformed/unparseable XML
}

// nfoBaseNames are the conventional sidecar names checked first, in order.
var nfoBaseNames = []string{"movie.nfo"}

// Discover returns the path of the sidecar .nfo for a media file, if one
// exists. Resolution order (authority/Kodi convention):
//  1. <mediafile-without-ext>.nfo
//  2. movie.nfo in the same directory
//  3. the single *.nfo in the same directory (only if exactly one exists)
//
// ok is false when no sidecar is found. mediaPath is an absolute or relative
// path to the media file itself.
func Discover(mediaPath string) (string, bool) {
	dir := filepath.Dir(mediaPath)
	stem := strings.TrimSuffix(filepath.Base(mediaPath), filepath.Ext(mediaPath))

	if p := dir + string(filepath.Separator) + stem + ".nfo"; fileExists(p) {
		return p, true
	}
	for _, name := range nfoBaseNames {
		if p := filepath.Join(dir, name); fileExists(p) {
			return p, true
		}
	}
	if matches, _ := filepath.Glob(filepath.Join(dir, "*.nfo")); len(matches) == 1 {
		return matches[0], true
	}
	return "", false
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// xmlRoot is a permissive view of an NFO root element. Kodi <movie>/<tvshow>
// share these fields; we parse defensively and ignore everything else.
type xmlRoot struct {
	Title     string        `xml:"title"`
	Year      string        `xml:"year"`
	UniqueIDs []xmlUniqueID `xml:"uniqueid"`
	// Legacy single-id forms still seen in the wild.
	ID   string `xml:"id"`
	IMDB string `xml:"imdbid"`
	TMDB string `xml:"tmdbid"`
	TVDB string `xml:"tvdbid"`
}

type xmlUniqueID struct {
	Type    string `xml:"type,attr"`
	Default string `xml:"default,attr"`
	Value   string `xml:",chardata"`
}

// ParseFile reads and parses an NFO sidecar at path. It never panics; on a read
// error or malformed XML it returns Info{Path: path, OK: false}.
func ParseFile(path string) Info {
	data, err := os.ReadFile(path)
	if err != nil {
		return Info{Path: path, OK: false}
	}
	info := Parse(data)
	info.Path = path
	return info
}

// Parse parses NFO bytes (treated as untrusted). It tolerates malformed XML by
// returning OK=false rather than panicking or erroring.
func Parse(data []byte) Info {
	var root xmlRoot
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	dec.Strict = false // tolerate quirky NFO markup
	dec.CharsetReader = passthroughCharset
	if err := dec.Decode(&root); err != nil {
		return Info{OK: false}
	}

	info := Info{
		OK:    true,
		Title: strings.TrimSpace(root.Title),
		Year:  atoiSafe(strings.TrimSpace(root.Year)),
	}

	seen := map[string]bool{}
	addUID := func(provider, id string, def bool) {
		provider = strings.ToLower(strings.TrimSpace(provider))
		id = strings.TrimSpace(id)
		if provider == "" || id == "" {
			return
		}
		key := provider + "/" + id
		if seen[key] {
			return
		}
		seen[key] = true
		info.UniqueIDs = append(info.UniqueIDs, UniqueID{Provider: provider, ID: id, Default: def})
	}

	for _, u := range root.UniqueIDs {
		addUID(u.Type, u.Value, strings.EqualFold(strings.TrimSpace(u.Default), "true"))
	}
	// Legacy single-element forms (lower priority than <uniqueid>).
	addUID(ProviderIMDB, root.IMDB, false)
	addUID(ProviderTMDB, root.TMDB, false)
	addUID(ProviderTVDB, root.TVDB, false)
	if root.ID != "" {
		// A bare <id> is conventionally the IMDb id when it looks like one.
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(root.ID)), "tt") {
			addUID(ProviderIMDB, root.ID, false)
		} else {
			addUID(ProviderTMDB, root.ID, false)
		}
	}

	// Stable order: default first, then by provider, then id.
	sort.SliceStable(info.UniqueIDs, func(i, j int) bool {
		a, b := info.UniqueIDs[i], info.UniqueIDs[j]
		if a.Default != b.Default {
			return a.Default
		}
		if a.Provider != b.Provider {
			return a.Provider < b.Provider
		}
		return a.ID < b.ID
	})
	return info
}

// passthroughCharset lets the decoder accept declared encodings (e.g. UTF-8,
// or unknown labels) without failing; bytes are passed through unchanged.
func passthroughCharset(_ string, input io.Reader) (io.Reader, error) {
	return input, nil
}

func atoiSafe(s string) int {
	n := 0
	if s == "" {
		return 0
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}
