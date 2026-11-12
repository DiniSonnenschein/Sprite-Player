//go:build !windows

package main

import (
	"os"
	"path/filepath"
)

// moveToTrash moves the file into a hidden ".geloescht" folder next to it.
func moveToTrash(path string) error {
	dir := filepath.Join(filepath.Dir(path), ".geloescht")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.Rename(path, filepath.Join(dir, filepath.Base(path)))
}
