package storage

import (
	"context"
	"fmt"

	"github.com/spacesedan/kpub/internal/config"
)

// Uploader uploads a local file to remote storage.
type Uploader interface {
	Upload(ctx context.Context, localPath string, remoteName string) error
}

// NewUploader creates an Uploader from the given storage config.
func NewUploader(cfg config.StorageConfig) (Uploader, error) {
	switch cfg.Type {
	case "dropbox":
		return NewDropboxUploader(cfg.Dropbox)
	default:
		return nil, fmt.Errorf("unsupported storage type: %q", cfg.Type)
	}
}
