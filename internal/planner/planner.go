package planner

import (
	"errors"

	"github.com/reelwarden/reelwarden/internal/naming"
	"github.com/reelwarden/reelwarden/internal/store"
)

func CreateDryRun(st *store.Store, assetID string) (store.ActionPlan, error) {
	asset, ok := st.Asset(assetID)
	if !ok {
		return store.ActionPlan{}, errors.New("PLAN_ASSET_NOT_FOUND")
	}
	if asset.MatchState != "confirmed" {
		return store.ActionPlan{}, errors.New("PLAN_MATCH_NOT_CONFIRMED")
	}
	var chosen store.Candidate
	for _, c := range st.Candidates(assetID) {
		if c.ID == asset.ConfirmedCandidateID {
			chosen = c
			break
		}
	}
	if chosen.ID == "" {
		return store.ActionPlan{}, errors.New("PLAN_CANDIDATE_NOT_FOUND")
	}
	p := store.ActionPlan{AssetID: asset.ID, SourceRelativePath: asset.RelativePath, TargetRelativePath: naming.JellyfinPath(asset, chosen), DryRun: true, State: "dry_run", Warnings: []string{"v0.1.1 preview only; no file operation will be executed"}}
	return st.SavePlan(p), nil
}
