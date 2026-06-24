package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/reelwarden/reelwarden/internal/store"
)

// TestScan_PopulatesIdentityAndCandidates walks a temp library, then checks the
// scanner wired the resolver: each asset gets a ParsedIdentity and candidates,
// and the scan succeeds even with no ffprobe / no network (offline mock door).
func TestScan_PopulatesIdentityAndCandidates(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "Dune (2021)", "Dune.2021.2160p.BluRay.x265.mkv"))
	mustWrite(t, filepath.Join(dir, "ignore.txt")) // non-video: skipped

	st := store.New()
	root, err := st.AddRoot(dir)
	if err != nil {
		t.Fatalf("AddRoot: %v", err)
	}

	res, err := Scan(context.Background(), st, root)
	if err != nil {
		t.Fatalf("Scan returned error (must be resilient): %v", err)
	}
	if res.Scanned != 1 {
		t.Fatalf("expected 1 scanned video, got %d", res.Scanned)
	}

	a := res.Assets[0]
	if a.ParsedTitle == "" {
		t.Fatal("expected a parsed title on the asset")
	}
	if a.ParsedIdentityID == "" {
		t.Fatal("expected the asset linked to a stored ParsedIdentity")
	}
	if _, ok := st.ParsedIdentityForAsset(a.ID); !ok {
		t.Fatal("expected a ParsedIdentity persisted for the asset")
	}
	if len(st.Candidates(a.ID)) == 0 {
		t.Fatal("expected candidate matches from the offline mock provider door")
	}
	// §14.7: scanner never auto-confirms.
	if a.MatchState == "confirmed" {
		t.Fatal("scanner must never set match_state=confirmed (§14.7)")
	}
}

func mustWrite(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("not a real video"), 0o644); err != nil {
		t.Fatal(err)
	}
}
