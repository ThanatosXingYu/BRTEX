package server

import "regexp"

// goalDomainRE extracts the bare host names a task goal names. It is used by
// the ask_operator target check (goalTargets), not by goal creation: BRTEX
// dropped the gate's minimum-length and "must name a target" limits, but the
// operator-warning path still needs to know which hosts a goal mentions.
var goalDomainRE = regexp.MustCompile(`(?i)\b[a-z0-9][a-z0-9-]{1,61}\.[a-z]{2,}(:[0-9]+)?(/\S*)?\b`)

// Ambiguity gate (ouroboros take, #99) historically refused a task goal that
// was shorter than 80 bytes or that named no concrete target (URL, domain, IP,
// localhost:port or absolute path), with a literal [targetless] opt-out.
//
// BRTEX deliberately drops both hard limits: short goals and target-less goals
// are legitimate in this deployment, so creation no longer inspects the goal.
// The hook is kept so callers and the gate's tests stay in place if the policy
// is ever tightened again.
func goalGateCheck(string) error {
	return nil
}
