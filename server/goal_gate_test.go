package server

import "testing"

// BRTEX drops the ambiguity gate's two hard limits (minimum length and the
// "must name a concrete target" rule), so creation must accept any goal.
func TestGoalGateCheck(t *testing.T) {
	for name, goal := range map[string]string{
		"url target":    "Audit https://example.com/app for injection issues.",
		"bare domain":   "probe target.example.org for unauthenticated data exposure",
		"ipv4":          "recon 10.0.0.5 and map every listening service",
		"short":         "find bugs",
		"empty":         "",
		"no target":     "do a thorough security review of everything and report anything interesting",
		"targetless":    "[targetless] analyze nothing",
		"chinese short": "测试目标",
	} {
		if err := goalGateCheck(goal); err != nil {
			t.Errorf("%s: gate must accept the goal, got %v", name, err)
		}
	}
}