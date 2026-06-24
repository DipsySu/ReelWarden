package compliance

const (
	GateTMDBAI                      = "COMPLIANCE-TMDB-AI"
	StatusBlocked                   = "blocked"
	StatusAccepted                  = "accepted"
	ErrProviderAICombinationBlocked = "COMPLIANCE_PROVIDER_AI_COMBINATION_BLOCKED"
)

type RuntimeInputs struct {
	TMDBEnabled  bool
	AIEnabled    bool
	TMDBAIStatus string
}

type GateResult struct {
	GateID    string `json:"gate_id"`
	Status    string `json:"status"`
	Allowed   bool   `json:"allowed"`
	Reason    string `json:"reason"`
	ErrorCode string `json:"error_code,omitempty"`
}

func EvaluateTMDBAI(in RuntimeInputs) GateResult {
	result := GateResult{GateID: GateTMDBAI, Status: in.TMDBAIStatus, Allowed: true, Reason: "combination is not active"}
	if in.TMDBAIStatus == "" {
		result.Status = StatusBlocked
	}
	if in.TMDBEnabled && in.AIEnabled && result.Status != StatusAccepted {
		result.Allowed = false
		result.Reason = "TMDB and AI cannot be enabled together until the compliance gate is accepted by the project authority process"
		result.ErrorCode = ErrProviderAICombinationBlocked
	}
	return result
}
