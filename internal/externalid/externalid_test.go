package externalid

import (
	"reflect"
	"testing"
)

func TestParseFileNameForms(t *testing.T) {
	cases := []struct {
		name string
		rel  string
		want []Match
	}{
		{
			name: "tmdbid bracket",
			rel:  "Movies/The Matrix (1999) [tmdbid-603].mkv",
			want: []Match{{Provider: "tmdb", ID: "603", Source: SourceFileName}},
		},
		{
			name: "tmdb brace short key",
			rel:  "The Matrix {tmdb-603}.mkv",
			want: []Match{{Provider: "tmdb", ID: "603", Source: SourceFileName}},
		},
		{
			name: "imdbid bracket",
			rel:  "The Matrix [imdbid-tt0133093].mkv",
			want: []Match{{Provider: "imdb", ID: "tt0133093", Source: SourceFileName}},
		},
		{
			name: "imdb brace",
			rel:  "The Matrix {imdb-tt0133093}.mkv",
			want: []Match{{Provider: "imdb", ID: "tt0133093", Source: SourceFileName}},
		},
		{
			name: "tvdb brace",
			rel:  "Show {tvdb-12345}.mkv",
			want: []Match{{Provider: "tvdb", ID: "12345", Source: SourceFileName}},
		},
		{
			name: "tvdbid bracket",
			rel:  "Show [tvdbid-12345].mkv",
			want: []Match{{Provider: "tvdb", ID: "12345", Source: SourceFileName}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Parse(tc.rel)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Parse(%q) = %#v, want %#v", tc.rel, got, tc.want)
			}
		})
	}
}

func TestParseDirSource(t *testing.T) {
	got := Parse("The Matrix (1999) [tmdbid-603]/movie.mkv")
	want := []Match{{Provider: "tmdb", ID: "603", Source: SourceDirName}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestParseDedupePrefersFileName(t *testing.T) {
	got := Parse("Show [tmdbid-603]/Show [tmdbid-603].mkv")
	want := []Match{{Provider: "tmdb", ID: "603", Source: SourceFileName}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestParseMultipleDistinct(t *testing.T) {
	got := Parse("Movie [tmdbid-603] [imdbid-tt0133093].mkv")
	want := []Match{
		{Provider: "tmdb", ID: "603", Source: SourceFileName},
		{Provider: "imdb", ID: "tt0133093", Source: SourceFileName},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestParseInvalidIDsSkipped(t *testing.T) {
	cases := []string{
		"Movie [tmdbid-abc].mkv",   // tmdb must be numeric
		"Movie [imdbid-12345].mkv", // imdb must be tt+digits
		"Movie [tvdb-x12].mkv",     // tvdb must be numeric
		"Movie [foobar-1].mkv",     // unknown provider
		"Movie 1999.mkv",           // no embedded id
		"",                         // empty path
	}
	for _, rel := range cases {
		if got := Parse(rel); got != nil {
			t.Fatalf("Parse(%q) = %#v, want nil", rel, got)
		}
	}
}

func TestParseCaseInsensitive(t *testing.T) {
	got := Parse("Movie [TMDBID-603].mkv")
	want := []Match{{Provider: "tmdb", ID: "603", Source: SourceFileName}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	// IMDb id case-folds to lower.
	got = Parse("Movie [imdbid-TT0133093].mkv")
	want = []Match{{Provider: "imdb", ID: "tt0133093", Source: SourceFileName}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}
