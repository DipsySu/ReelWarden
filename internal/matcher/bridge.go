package matcher

import (
	"github.com/reelwarden/reelwarden/internal/store"
)

// ToStoreCandidate maps a deterministic ScoreResult onto the store.Candidate shape
// the rest of the app (resolver State, UI, persistence) already consumes. The
// store types use integer Score / Evidence.Points while the scorer works in float
// rank_score / contribution space (§14.7/§14.8); this bridge scales 0..1 -> 0..100
// so the existing fields stay meaningful without changing the shared store types.
//
// ScoreBand is taken from the float rank_score (the source of truth), NOT recomputed
// from the scaled integer, so band routing matches §14.7 exactly.
//
// TODO(integrate): the resolver's Integrate stage calls this to populate
// store.Candidate.{Score,ScoreBand,Evidence} from R3 output. If a richer float
// rank_score field is later added to store.Candidate, prefer carrying the raw
// ScoreResult.RankScore instead of scaling.
func ToStoreCandidate(assetID string, r ScoreResult) store.Candidate {
	ev := make([]store.Evidence, 0, len(r.Evidence.Groups)+len(r.Evidence.Penalties))
	for _, g := range r.Evidence.Groups {
		ev = append(ev, store.Evidence{
			Group:   g.Group,
			Code:    g.SelectedEvidence.Type,
			Message: g.SelectedEvidence.Detail,
			Points:  scaleContribution(g.SelectedEvidence.Contribution),
		})
	}
	for _, p := range r.Evidence.Penalties {
		ev = append(ev, store.Evidence{
			Group:   "penalty",
			Code:    p.Type,
			Message: p.Detail,
			Points:  scaleContribution(p.Contribution), // negative
		})
	}
	return store.Candidate{
		ID:         store.NewID("cand"),
		AssetID:    assetID,
		Provider:   r.Provider,
		ProviderID: r.ProviderItemID,
		Title:      r.Title, // carried from ProviderCandidate.Title via ScoreResult; was dropped -> "Unknown Movie" on rename
		Year:       r.Year,  // carried from ProviderCandidate.Year via ScoreResult
		Score:      scaleScore(r.RankScore),
		ScoreBand:  r.ScoreBand, // float-derived band, not recomputed from the int
		Evidence:   ev,
	}
}

// ToStoreCandidates maps a ranked ScoreResult slice to store.Candidate values,
// preserving the scorer's order.
func ToStoreCandidates(assetID string, results []ScoreResult) []store.Candidate {
	out := make([]store.Candidate, 0, len(results))
	for _, r := range results {
		out = append(out, ToStoreCandidate(assetID, r))
	}
	return out
}

// scaleScore maps rank_score in [0,1] to the integer 0..100 store field.
func scaleScore(f float64) int { return int(round4(f)*100 + 0.5) }

// scaleContribution maps a signed contribution in [-1,1] to integer points.
func scaleContribution(f float64) int {
	if f < 0 {
		return -int(-f*100 + 0.5)
	}
	return int(f*100 + 0.5)
}
