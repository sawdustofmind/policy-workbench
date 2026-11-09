package domain

import "slices"

type DecisionOutcome string

const (
	OutcomeAllow   DecisionOutcome = "ALLOW"
	OutcomeReject  DecisionOutcome = "REJECT"
	OutcomeNoMatch DecisionOutcome = "NO_MATCH"
)

type ConditionResult struct {
	Name    string `json:"name"`
	Matched bool   `json:"matched"`
	Detail  string `json:"detail"`
}

type RuleEvaluation struct {
	PolicyID   string            `json:"policy_id"`
	PolicyName string            `json:"policy_name"`
	Matched    bool              `json:"matched"`
	Checks     []ConditionResult `json:"checks"`
}

type PolicyDecision struct {
	PolicyID      string           `json:"policy_id"`
	PolicyName    string           `json:"policy_name"`
	Outcome       DecisionOutcome  `json:"outcome"`
	MatchedPolicy *Policy          `json:"matched_policy,omitempty"`
	Trace         []RuleEvaluation `json:"trace"`
}

func (decision PolicyDecision) Clone() PolicyDecision {
	if decision.MatchedPolicy != nil {
		policy := decision.MatchedPolicy.Clone()
		decision.MatchedPolicy = &policy
	}
	decision.Trace = slices.Clone(decision.Trace)
	for index := range decision.Trace {
		decision.Trace[index].Checks = slices.Clone(decision.Trace[index].Checks)
	}
	return decision
}
