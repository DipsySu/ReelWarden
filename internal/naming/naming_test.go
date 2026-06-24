package naming

import (
	"github.com/reelwarden/reelwarden/internal/store"
	"testing"
)

func TestJellyfinPath(t *testing.T) {
	got := JellyfinPath(store.MediaAsset{RelativePath: "raw/Dune.2021.mkv"}, store.Candidate{Title: "Dune", Year: 2021})
	want := "Dune (2021)/Dune (2021).mkv"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
