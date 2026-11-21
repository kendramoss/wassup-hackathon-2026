//go:build windows

package budget

import (
	"os"

	"golang.org/x/sys/windows"
)

// lockExclusive takes the Windows equivalent of an exclusive, non-blocking
// flock on the first byte of the lock file. Every contender locks that same
// byte, so a second Open fails immediately; closing the handle releases it.
func lockExclusive(f *os.File) error {
	var ol windows.Overlapped
	return windows.LockFileEx(
		windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0, 1, 0, &ol,
	)
}
