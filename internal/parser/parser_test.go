package parser

import "testing"

func TestParsePathExtractsCJKTitleYearAndTags(t *testing.T) {
	got := ParsePath("沙丘.2021.2160p.HDR.mkv")
	if got.Title != "沙丘" {
		t.Fatalf("title = %q", got.Title)
	}
	if got.Year != 2021 {
		t.Fatalf("year = %d", got.Year)
	}
	if len(got.Tags) < 2 {
		t.Fatalf("expected technical tags, got %#v", got.Tags)
	}
}
