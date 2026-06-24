package compliance

import "testing"

func TestEvaluateTMDBAIBlocksActiveCombination(t *testing.T) {
	got := EvaluateTMDBAI(RuntimeInputs{TMDBEnabled: true, AIEnabled: true, TMDBAIStatus: StatusBlocked})
	if got.Allowed {
		t.Fatal("expected TMDB + AI to be blocked when gate is not accepted")
	}
	if got.ErrorCode != ErrProviderAICombinationBlocked {
		t.Fatalf("expected %s, got %s", ErrProviderAICombinationBlocked, got.ErrorCode)
	}
}

func TestEvaluateTMDBAIAllowsInactiveCombination(t *testing.T) {
	got := EvaluateTMDBAI(RuntimeInputs{TMDBEnabled: true, AIEnabled: false, TMDBAIStatus: StatusBlocked})
	if !got.Allowed {
		t.Fatalf("expected inactive combination to be allowed: %#v", got)
	}
}
