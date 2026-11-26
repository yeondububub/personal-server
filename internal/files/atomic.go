package files

import (
	"fmt"
	"os"
	"path/filepath"
)

// AtomicWriteFile writes data to a temporary file in the same directory, syncs it, and renames it over targetPath.
func AtomicWriteFile(targetPath string, data []byte, defaultPerm os.FileMode) error {
	perm := defaultPerm
	if info, err := os.Stat(targetPath); err == nil {
		perm = info.Mode().Perm()
	}

	dir := filepath.Dir(targetPath)
	tmp, err := os.CreateTemp(dir, ".ps-atomic-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpName, targetPath); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}
	cleanup = false
	return nil
}
