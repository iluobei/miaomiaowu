package ruletemplates

import (
	"embed"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed *.yaml
var files embed.FS

// Ensure writes the embedded rule template files into the provided directory
// if they do not already exist. Existing files are left untouched so that user
// modifications persist across restarts.
func Ensure(targetDir string) error {
	if targetDir == "" {
		targetDir = "."
	}

	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return err
	}

	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		destination := filepath.Join(targetDir, name)

		// Skip if file already exists
		if _, err := os.Stat(destination); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}

		data, err := fs.ReadFile(files, name)
		if err != nil {
			return err
		}

		if err := os.WriteFile(destination, data, 0o644); err != nil {
			return err
		}
	}

	return nil
}
