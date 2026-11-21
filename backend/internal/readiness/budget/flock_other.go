//go:build !unix && !windows

package budget

import "os"

// Other targets have no portable byte-range lock primitive here. The ledger
// is exercised on Linux and Windows; on anything else the in-process mutex
// still serialises access, but a second process is not fenced out.
func lockExclusive(*os.File) error { return nil }
