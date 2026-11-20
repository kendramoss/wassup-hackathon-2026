package budget

import (
	"fmt"
	"syscall"
)

// checkFSType accepts only local APFS/HFS volumes: advisory flock and fsync
// are not trustworthy on network or FUSE filesystems.
func checkFSType(dir string) error {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return err
	}
	name := make([]byte, 0, 16)
	for _, c := range st.Fstypename {
		if c == 0 {
			break
		}
		name = append(name, byte(c))
	}
	switch string(name) {
	case "apfs", "hfs":
		return nil
	}
	return fmt.Errorf("%w: filesystem %q", ErrNonLocalFS, string(name))
}
