package server

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
