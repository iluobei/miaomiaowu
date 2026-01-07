package proxygroups

import (
	"embed"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const (
	defaultFileName  = "proxy-groups.json"
	embeddedFileName = "proxy-groups.default.json"
)

//go:embed proxy-groups.default.json
var embeddedFS embed.FS

// Ensure makes sure the proxy groups configuration file exists in targetDir.
// If it does not, it writes the embedded default configuration.
// Returns the full path to the configuration file.
func Ensure(targetDir string) (string, error) {
	if targetDir == "" {
		targetDir = "."
	}

	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return "", err
	}

	path := filepath.Join(targetDir, defaultFileName)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		data, readErr := embeddedFS.ReadFile(embeddedFileName)
		if readErr != nil {
			return "", readErr
		}
		if writeErr := os.WriteFile(path, data, 0o644); writeErr != nil {
			return "", writeErr
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	return path, nil
}

// Load reads the proxy groups configuration file into v if provided.
// v should be a pointer to a slice or struct that can hold the JSON data.
func Load(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	if v == nil {
		return nil
	}

	return json.Unmarshal(data, v)
}

// Save writes v into the provided path in pretty JSON form.
// Uses atomic write (write to temp file then rename) for safety.
func Save(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}

	return os.Rename(tmp, path)
}
