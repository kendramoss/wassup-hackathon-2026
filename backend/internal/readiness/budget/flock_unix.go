//go:build unix

package budget

import (
	"os"
	"syscall"
)

// lockExclusive takes a non-blocking exclusive advisory lock on the lock file.
// A second process gets ErrLocked immediately rather than waiting.
func lockExclusive(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}
