package parser

import (
	"testing"
)

// TestParse exercises the full §12 pipeline end-to-end. It asserts the load-bearing
// fields (RawTitle, NormalizedTitle, Year, Edition, ReleaseGroup) and spot-checks the
// presence of expected comparison keys / technical tags.
func TestParse(t *testing.T) {
	tests := []struct {
		name        string
		rel         string
		parent      string
		wantRaw     string
		wantNorm    string
		wantYear    int
		wantEd      string
		wantGroup   string
		wantTagsSub []string // tags that must all be present (order-insensitive)
		wantKeysSub []string // comparison keys that must all be present
	}{
		{
			name:     "cjk movie with year and group",
			rel:      "沙丘.2021.2160p.WEB-DL.x265-NoGroup.mkv",
			wantRaw:  "沙丘",
			wantNorm: "沙丘",
			wantYear: 2021, wantGroup: "NoGroup",
			wantTagsSub: []string{"2160p", "WEB-DL", "x265"},
		},
		{
			name:     "ascii movie bluray",
			rel:      "Dune.2021.1080p.BluRay.x264-RARBG.mkv",
			wantRaw:  "Dune",
			wantNorm: "dune",
			wantYear: 2021, wantGroup: "RARBG",
			wantTagsSub: []string{"1080p", "BluRay", "x264"},
		},
		{
			name:     "title-with-year-number keeps title digits, picks real year",
			rel:      "Blade.Runner.2049.2017.2160p.UHD.BluRay.x265-TT.mkv",
			wantRaw:  "Blade Runner 2049",
			wantNorm: "blade runner 2049",
			wantYear: 2017, wantGroup: "TT",
			wantTagsSub: []string{"2160p", "BluRay", "x265"},
			// ASCII title: word spacing is meaningful, so no spaceless/fold keys.
		},
		{
			name:        "numeric title is protected (2046)",
			rel:         "2046.2004.1080p.BluRay.mkv",
			wantRaw:     "2046",
			wantNorm:    "2046",
			wantYear:    2004,
			wantTagsSub: []string{"1080p", "BluRay"},
		},
		{
			name:     "numeric-only title with single year keeps title, no year",
			rel:      "1917.mkv",
			wantRaw:  "1917",
			wantNorm: "1917",
			wantYear: 0,
		},
		{
			name:     "fullwidth title and fullwidth year",
			rel:      "Ｄｕｎｅ　２０２１.mkv",
			wantRaw:  "Ｄｕｎｅ",
			wantNorm: "dune",
			wantYear: 2021,
		},
		{
			name:        "traditional chinese gets simplified fold key",
			rel:         "無間道.2002.BluRay.mkv",
			wantRaw:     "無間道",
			wantNorm:    "無間道",
			wantYear:    2002,
			wantTagsSub: []string{"BluRay"},
			wantKeysSub: []string{"无间道"},
		},
		{
			name:     "edition extracted, kept out of tags",
			rel:      "The.Lord.of.the.Rings.Extended.Edition.2001.1080p.mkv",
			wantRaw:  "The Lord of the Rings",
			wantNorm: "the lord of the rings",
			wantYear: 2001, wantEd: "Extended Edition",
			wantTagsSub: []string{"1080p"},
		},
		{
			name:     "directors cut edition with hdr/dv/atmos",
			rel:      "Movie.Title.2023.Directors.Cut.2160p.HDR.DV.Atmos-XYZ.mkv",
			wantRaw:  "Movie Title",
			wantNorm: "movie title",
			wantYear: 2023, wantEd: "Director's Cut", wantGroup: "XYZ",
			wantTagsSub: []string{"2160p", "HDR", "DV", "Atmos"},
		},
		{
			name:     "roman numeral sequel folds in normalized form",
			rel:      "Rocky.II.1979.mkv",
			wantRaw:  "Rocky II",
			wantNorm: "rocky 2",
			wantYear: 1979,
		},
		{
			name:     "bracketed fansub group",
			rel:      "[NekoSub] 沙丘 (2021).mkv",
			wantRaw:  "沙丘",
			wantNorm: "沙丘",
			wantYear: 2021, wantGroup: "NekoSub",
		},
		{
			name:     "garbled cjk filename preserved verbatim",
			rel:      "低zhi商犯罪.2023.mkv",
			wantRaw:  "低zhi商犯罪",
			wantNorm: "低zhi商犯罪",
			wantYear: 2023,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := Parse(tt.rel, tt.parent)
			if id.RawTitle != tt.wantRaw {
				t.Errorf("RawTitle = %q, want %q", id.RawTitle, tt.wantRaw)
			}
			if id.NormalizedTitle != tt.wantNorm {
				t.Errorf("NormalizedTitle = %q, want %q", id.NormalizedTitle, tt.wantNorm)
			}
			if id.Year != tt.wantYear {
				t.Errorf("Year = %d, want %d", id.Year, tt.wantYear)
			}
			if id.Edition != tt.wantEd {
				t.Errorf("Edition = %q, want %q", id.Edition, tt.wantEd)
			}
			if id.ReleaseGroup != tt.wantGroup {
				t.Errorf("ReleaseGroup = %q, want %q", id.ReleaseGroup, tt.wantGroup)
			}
			for _, want := range tt.wantTagsSub {
				if !contains(id.TechnicalTags, want) {
					t.Errorf("TechnicalTags %v missing %q", id.TechnicalTags, want)
				}
			}
			for _, want := range tt.wantKeysSub {
				if !contains(id.ComparisonKeys, want) {
					t.Errorf("ComparisonKeys %v missing %q", id.ComparisonKeys, want)
				}
			}
		})
	}
}

// TestParseInvariants checks structural guarantees independent of any single case.
func TestParseInvariants(t *testing.T) {
	id := Parse("沙丘.2021.1080p-NG.mkv", "")
	if id.ParserVersion != ParserVersion {
		t.Errorf("ParserVersion = %q, want %q", id.ParserVersion, ParserVersion)
	}
	if id.State != "parsed" {
		t.Errorf("State = %q, want parsed", id.State)
	}
	if len(id.Hypotheses) == 0 {
		t.Fatal("expected at least one query hypothesis")
	}
	first := id.Hypotheses[0]
	if first.Title != "沙丘" || first.Year != 2021 || first.Source != "rule" {
		t.Errorf("first hypothesis = %+v, want title=沙丘 year=2021 source=rule", first)
	}
	if id.Confidence <= 0 || id.Confidence > 0.95 {
		t.Errorf("confidence %v outside (0, 0.95]; must be a bounded heuristic", id.Confidence)
	}
}

// TestParentDirHypothesis verifies the local parent-dir signal becomes a hypothesis.
func TestParentDirHypothesis(t *testing.T) {
	id := Parse("Some Movie (2010)/movie.file.2010.mkv", "Some Movie (2010)")
	if id.ParentDirName != "Some Movie (2010)" {
		t.Errorf("ParentDirName = %q, want %q", id.ParentDirName, "Some Movie (2010)")
	}
	found := false
	for _, h := range id.Hypotheses {
		if h.Source == "parent_dir" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a parent_dir hypothesis, got %+v", id.Hypotheses)
	}
}

// TestParsePathBackCompat guards the legacy scanner contract: ParsePath still
// returns Title/Year/Tags and shares the new pipeline.
func TestParsePathBackCompat(t *testing.T) {
	r := ParsePath("Dune.2021.1080p.BluRay-RARBG.mkv")
	if r.Title != "Dune" {
		t.Errorf("Title = %q, want Dune", r.Title)
	}
	if r.Year != 2021 {
		t.Errorf("Year = %d, want 2021", r.Year)
	}
	if !contains(r.Tags, "1080p") || !contains(r.Tags, "BluRay") {
		t.Errorf("Tags = %v, want to include 1080p and BluRay", r.Tags)
	}
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
