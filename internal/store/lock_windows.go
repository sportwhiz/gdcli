//go:build windows

package store

import "os"

// Windows builds use process-local serialization for token writes.
// Cross-process locking can be added with LockFileEx if needed.
func lockFile(_ *os.File) error {
	return nil
}

func unlockFile(_ *os.File) error {
	return nil
}
