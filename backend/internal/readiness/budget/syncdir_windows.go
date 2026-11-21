//go:build windows

package budget

// syncDir is a no-op on Windows: os.File.Sync on a directory handle returns
// ERROR_ACCESS_DENIED there, and Windows does not expose a portable directory
// fsync through os. The ledger's own file is still synced by its writer.
func syncDir(string) error { return nil }
