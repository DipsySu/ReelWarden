package nfo

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseUniqueIDs(t *testing.T) {
	data := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<movie>
  <title>The Matrix</title>
  <year>1999</year>
  <uniqueid type="tmdb" default="true">603</uniqueid>
  <uniqueid type="imdb">tt0133093</uniqueid>
</movie>`)
	got := Parse(data)
	if !got.OK {
		t.Fatalf("expected OK")
	}
	if got.Title != "The Matrix" {
		t.Fatalf("title = %q", got.Title)
	}
	if got.Year != 1999 {
		t.Fatalf("year = %d", got.Year)
	}
	want := []UniqueID{
		{Provider: "tmdb", ID: "603", Default: true},
		{Provider: "imdb", ID: "tt0133093", Default: false},
	}
	if !reflect.DeepEqual(got.UniqueIDs, want) {
		t.Fatalf("uniqueids = %#v, want %#v", got.UniqueIDs, want)
	}
}

func TestParseLegacyForms(t *testing.T) {
	data := []byte(`<movie>
  <title>Old NFO</title>
  <imdbid>tt0111161</imdbid>
  <tmdbid>278</tmdbid>
</movie>`)
	got := Parse(data)
	if !got.OK {
		t.Fatalf("expected OK")
	}
	want := []UniqueID{
		{Provider: "imdb", ID: "tt0111161"},
		{Provider: "tmdb", ID: "278"},
	}
	if !reflect.DeepEqual(got.UniqueIDs, want) {
		t.Fatalf("uniqueids = %#v, want %#v", got.UniqueIDs, want)
	}
}

func TestParseMalformedReturnsNotOK(t *testing.T) {
	cases := [][]byte{
		[]byte(`<movie><title>Broken`), // unterminated
		[]byte(`not xml at all {{{`),   // garbage
		[]byte(``),                     // empty
		[]byte(`<<<>>>`),               // junk tokens
	}
	for i, data := range cases {
		got := Parse(data)
		if got.OK {
			t.Fatalf("case %d: expected OK=false for malformed input %q", i, data)
		}
		if len(got.UniqueIDs) != 0 {
			t.Fatalf("case %d: expected no ids", i)
		}
	}
}

func TestParseDedupe(t *testing.T) {
	data := []byte(`<movie>
  <uniqueid type="tmdb">603</uniqueid>
  <uniqueid type="tmdb">603</uniqueid>
  <tmdbid>603</tmdbid>
</movie>`)
	got := Parse(data)
	if len(got.UniqueIDs) != 1 {
		t.Fatalf("expected 1 deduped id, got %#v", got.UniqueIDs)
	}
}

func TestParseEmptyValuesSkipped(t *testing.T) {
	data := []byte(`<movie>
  <uniqueid type="tmdb"></uniqueid>
  <uniqueid type="">123</uniqueid>
  <uniqueid type="imdb">tt1</uniqueid>
</movie>`)
	got := Parse(data)
	want := []UniqueID{{Provider: "imdb", ID: "tt1"}}
	if !reflect.DeepEqual(got.UniqueIDs, want) {
		t.Fatalf("got %#v, want %#v", got.UniqueIDs, want)
	}
}

func TestParseBareIDImdb(t *testing.T) {
	got := Parse([]byte(`<movie><id>tt0133093</id></movie>`))
	want := []UniqueID{{Provider: "imdb", ID: "tt0133093"}}
	if !reflect.DeepEqual(got.UniqueIDs, want) {
		t.Fatalf("got %#v, want %#v", got.UniqueIDs, want)
	}
}

func TestDiscoverStemMatch(t *testing.T) {
	dir := t.TempDir()
	media := filepath.Join(dir, "The Matrix (1999).mkv")
	nfoPath := filepath.Join(dir, "The Matrix (1999).nfo")
	mustWrite(t, media, "x")
	mustWrite(t, nfoPath, "<movie/>")

	got, ok := Discover(media)
	if !ok || got != nfoPath {
		t.Fatalf("Discover = %q,%v want %q,true", got, ok, nfoPath)
	}
}

func TestDiscoverMovieNfo(t *testing.T) {
	dir := t.TempDir()
	media := filepath.Join(dir, "video.mkv")
	nfoPath := filepath.Join(dir, "movie.nfo")
	mustWrite(t, media, "x")
	mustWrite(t, nfoPath, "<movie/>")

	got, ok := Discover(media)
	if !ok || got != nfoPath {
		t.Fatalf("Discover = %q,%v want %q,true", got, ok, nfoPath)
	}
}

func TestDiscoverSingleNfo(t *testing.T) {
	dir := t.TempDir()
	media := filepath.Join(dir, "video.mkv")
	nfoPath := filepath.Join(dir, "whatever.nfo")
	mustWrite(t, media, "x")
	mustWrite(t, nfoPath, "<movie/>")

	got, ok := Discover(media)
	if !ok || got != nfoPath {
		t.Fatalf("Discover = %q,%v want %q,true", got, ok, nfoPath)
	}
}

func TestDiscoverNoneWhenAmbiguous(t *testing.T) {
	dir := t.TempDir()
	media := filepath.Join(dir, "video.mkv")
	mustWrite(t, media, "x")
	mustWrite(t, filepath.Join(dir, "a.nfo"), "<movie/>")
	mustWrite(t, filepath.Join(dir, "b.nfo"), "<movie/>")

	if got, ok := Discover(media); ok {
		t.Fatalf("expected no sidecar for ambiguous dir, got %q", got)
	}
}

func TestDiscoverNone(t *testing.T) {
	dir := t.TempDir()
	media := filepath.Join(dir, "video.mkv")
	mustWrite(t, media, "x")
	if got, ok := Discover(media); ok {
		t.Fatalf("expected no sidecar, got %q", got)
	}
}

func TestParseFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "movie.nfo")
	mustWrite(t, p, `<movie><title>T</title><uniqueid type="tmdb">5</uniqueid></movie>`)
	got := ParseFile(p)
	if !got.OK || got.Path != p || got.Title != "T" {
		t.Fatalf("ParseFile = %#v", got)
	}
}

func TestParseFileMissing(t *testing.T) {
	got := ParseFile(filepath.Join(t.TempDir(), "nope.nfo"))
	if got.OK {
		t.Fatalf("expected OK=false for missing file")
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
