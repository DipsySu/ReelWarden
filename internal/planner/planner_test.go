package planner

import (
	"github.com/reelwarden/reelwarden/internal/store"
	"testing"
)

func TestCreateDryRunRequiresConfirmedMatch(t *testing.T) {
	st := store.New()
	a := st.UpsertAsset(store.MediaAsset{LibraryRootID: "root", RelativePath: "Dune.2021.mkv", ParsedTitle: "Dune", ParsedYear: 2021, ScanState: "scanned", MatchState: "needs_review"})
	if _, err := CreateDryRun(st, a.ID); err == nil {
		t.Fatal("expected unconfirmed asset to be rejected")
	}
}
func TestCreateDryRun(t *testing.T) {
	st := store.New()
	a := st.UpsertAsset(store.MediaAsset{LibraryRootID: "root", RelativePath: "Dune.2021.mkv", ParsedTitle: "Dune", ParsedYear: 2021, ScanState: "scanned", MatchState: "needs_review"})
	c := store.Candidate{ID: store.NewID("cand"), AssetID: a.ID, Title: "Dune", Year: 2021}
	st.SaveCandidates(a.ID, []store.Candidate{c})
	if _, err := st.Confirm(a.ID, c.ID); err != nil {
		t.Fatal(err)
	}
	p, err := CreateDryRun(st, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !p.DryRun || p.State != "dry_run" {
		t.Fatalf("bad plan %#v", p)
	}
}
