//go:build !darwin

package budget

// checkFSType is not implemented off darwin; the path-based refusal still
// applies and the operator must keep the ledger on a local disk.
func checkFSType(string) error { return nil }
