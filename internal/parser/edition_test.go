package parser

import (
	"reflect"
	"testing"
)

func TestExtractTechnicalTags(t *testing.T) {
	tests := []struct {
		name     string
		in       string // separator-spaced working string
		wantTags []string
		wantRest string
	}{
		// Tag output order follows the canonical spec list, not input order, so the
		// result is deterministic regardless of how a release names its tags.
		{
			name:     "common bluray release; DTS-HD MA + channel layout fully consumed",
			in:       "Dune 1080p BluRay x264 DTS-HD MA 5 1",
			wantTags: []string{"1080p", "BluRay", "x264", "DTS"},
			wantRest: "Dune",
		},
		{
			name:     "uhd remux hdr dv atmos",
			in:       "Movie 2160p UHD BluRay REMUX HDR DV TrueHD Atmos 7 1",
			wantTags: []string{"2160p", "HDR", "DV", "REMUX", "BluRay", "TrueHD", "Atmos", "7.1"},
			wantRest: "Movie",
		},
		{
			name:     "web-dl hevc 10bit ddp glued to channel layout",
			in:       "Show WEB-DL HEVC 10bit DDP5 1",
			wantTags: []string{"WEB-DL", "HEVC", "DDP", "10bit"},
			wantRest: "Show",
		},
		{
			name:     "no tags",
			in:       "Just A Title",
			wantTags: nil,
			wantRest: "Just A Title",
		},
		{
			name:     "h265 dotted and dolby vision",
			in:       "Film H 265 Dolby Vision",
			wantTags: []string{"Dolby Vision", "H.265"},
			wantRest: "Film",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTags, gotRest := extractTechnicalTags(tt.in)
			gotRest = collapseSpaces(gotRest)
			if !equalTags(gotTags, tt.wantTags) {
				t.Errorf("tags = %v, want %v", gotTags, tt.wantTags)
			}
			if gotRest != tt.wantRest {
				t.Errorf("rest = %q, want %q", gotRest, tt.wantRest)
			}
		})
	}
}

func TestExtractEdition(t *testing.T) {
	tests := []struct {
		in       string
		wantEd   string
		wantRest string
	}{
		{"The Movie Directors Cut here", "Director's Cut", "The Movie here"},
		{"Film Extended Edition", "Extended Edition", "Film"},
		{"Title Theatrical Cut", "Theatrical Cut", "Title"},
		{"Plain Title", "", "Plain Title"},
		{"Movie Final Cut 2007", "Final Cut", "Movie 2007"},
		{"Show Unrated", "Unrated", "Show"},
	}
	for _, tt := range tests {
		ed, rest := extractEdition(tt.in)
		rest = collapseSpaces(rest)
		if ed != tt.wantEd {
			t.Errorf("extractEdition(%q) edition = %q, want %q", tt.in, ed, tt.wantEd)
		}
		if rest != tt.wantRest {
			t.Errorf("extractEdition(%q) rest = %q, want %q", tt.in, rest, tt.wantRest)
		}
	}
}

func TestExtractReleaseGroup(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		wantGroup string
		wantRest  string
	}{
		{"trailing dash group", "Dune.2021.1080p-RARBG", "RARBG", "Dune.2021.1080p"},
		{"leading bracket group", "[NekoSub] 沙丘 2021", "NekoSub", "沙丘 2021"},
		{"group with at sign", "Movie.2020-CMCT@HDSky", "CMCT@HDSky", "Movie.2020"},
		{"no group", "Plain Title 2019", "", "Plain Title 2019"},
		{"trailing year is not a group", "Movie-2021", "", "Movie-2021"},
		{"trailing tech tag is not a group", "Movie-x265", "", "Movie-x265"},
		{"single char not a group", "Movie-A", "", "Movie-A"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotGroup, gotRest := extractReleaseGroup(tt.in)
			if gotGroup != tt.wantGroup {
				t.Errorf("group = %q, want %q", gotGroup, tt.wantGroup)
			}
			if gotRest != tt.wantRest {
				t.Errorf("rest = %q, want %q", gotRest, tt.wantRest)
			}
		})
	}
}

// equalTags treats nil and empty as equal for convenience.
func equalTags(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return reflect.DeepEqual(a, b)
}
