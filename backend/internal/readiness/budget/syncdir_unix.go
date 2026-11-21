//go:build unix

package budget

import "os"

// syncDir flushes a rename to disk on Unix, where an fsync on the containing
// directory is the supported way to make the directory entry durable.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
