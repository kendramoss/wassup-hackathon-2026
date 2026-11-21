//go:build !unix && !windows

package budget

// Other targets have no portable directory fsync through os; the ledger is
// exercised on Linux and Windows.
func syncDir(string) error { return nil }
