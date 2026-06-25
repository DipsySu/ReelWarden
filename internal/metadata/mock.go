package metadata

import (
	"fmt"
	"strings"

	"github.com/reelwarden/reelwarden/internal/store"
)

func MockCandidates(asset store.MediaAsset) []store.Candidate {
	title := asset.ParsedTitle
	if title == "" {
		title = "Unknown Movie"
	}
	exact := store.Candidate{ID: store.NewID("cand"), AssetID: asset.ID, Provider: "mock", ProviderID: "mock:" + strings.ToLower(strings.ReplaceAll(title, " ", "-")), Title: title, Year: asset.ParsedYear, Score: 95, ScoreBand: "high", Evidence: []store.Evidence{{Group: "title", Code: "TITLE_EXACT", Message: "parsed title matches mock candidate", Points: 70}, {Group: "year", Code: "YEAR_MATCH", Message: fmt.Sprintf("parsed year %d matches", asset.ParsedYear), Points: 25}}}
	alt := exact
	alt.ID = store.NewID("cand")
	alt.ProviderID += "-alt"
	alt.Title = title + " Alternative"
	alt.Score = 62
	alt.ScoreBand = "medium"
	alt.Evidence = []store.Evidence{{Group: "title", Code: "TITLE_PARTIAL", Message: "candidate shares title tokens", Points: 45}, {Group: "auxiliary", Code: "MOCK_FALLBACK", Message: "mock provider fallback candidate", Points: 17}}
	return []store.Candidate{exact, alt}
}
