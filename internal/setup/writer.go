package setup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spacesedan/kpub/internal/config"
	"gopkg.in/yaml.v3"
)

// WriteConfig serializes cfg to config.yaml in the given directory.
// It uses an atomic write (temp file + rename) so that file watchers
// never see a half-written config.
func WriteConfig(dir string, cfg *config.Config) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating directory %q: %w", dir, err)
	}

	path := filepath.Join(dir, "config.yaml")
	tmp := path + ".tmp"

	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("creating temp file %q: %w", tmp, err)
	}

	enc := yaml.NewEncoder(f)
	enc.SetIndent(2)
	if err := enc.Encode(cfg); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("writing config: %w", err)
	}
	if err := enc.Close(); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("closing yaml encoder: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("closing temp file: %w", err)
	}

	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("renaming temp file to %q: %w", path, err)
	}
	return nil
}

// WriteDropboxTokens serializes tokens to dropbox.json in the given directory.
func WriteDropboxTokens(dir string, tokens *DropboxTokens) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating directory %q: %w", dir, err)
	}

	path := filepath.Join(dir, "dropbox.json")
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating %q: %w", path, err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(tokens); err != nil {
		return fmt.Errorf("writing dropbox tokens: %w", err)
	}
	return nil
}
