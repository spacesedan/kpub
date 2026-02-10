package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/spacesedan/kpub/internal/config"
)

type dropboxTokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// DropboxUploader uploads files to Dropbox.
type DropboxUploader struct {
	mu         sync.Mutex
	tokens     dropboxTokens
	tokenFile  string
	appKey     string
	appSecret  string
	uploadPath string
}

// NewDropboxUploader loads tokens from disk and returns a ready uploader.
func NewDropboxUploader(cfg config.DropboxConfig) (*DropboxUploader, error) {
	data, err := os.ReadFile(cfg.TokenFile)
	if err != nil {
		return nil, fmt.Errorf("reading dropbox token file %q: %w", cfg.TokenFile, err)
	}

	var tokens dropboxTokens
	if err := json.Unmarshal(data, &tokens); err != nil {
		return nil, fmt.Errorf("parsing dropbox token file %q: %w", cfg.TokenFile, err)
	}

	if tokens.AccessToken == "" || tokens.RefreshToken == "" {
		return nil, fmt.Errorf("'access_token' or 'refresh_token' is missing from %q", cfg.TokenFile)
	}

	return &DropboxUploader{
		tokens:     tokens,
		tokenFile:  cfg.TokenFile,
		appKey:     cfg.AppKey,
		appSecret:  cfg.AppSecret,
		uploadPath: cfg.UploadPath,
	}, nil
}

// Upload uploads a local file to Dropbox, retrying once on 401 after refreshing the token.
func (d *DropboxUploader) Upload(ctx context.Context, localPath string, remoteName string) error {
	for attempt := 0; attempt < 2; attempt++ {
		err := d.doUpload(ctx, localPath, remoteName)
		if err == nil {
			return nil
		}

		if attempt == 0 && isUnauthorized(err) {
			slog.Warn("Dropbox upload failed with 401, refreshing token and retrying...")
			if refreshErr := d.refreshToken(); refreshErr != nil {
				return fmt.Errorf("failed to refresh token, cannot retry upload: %w", refreshErr)
			}
			slog.Info("Retrying Dropbox upload with new token...")
			continue
		}

		return err
	}
	return fmt.Errorf("dropbox upload failed after multiple retries")
}

type unauthorizedError struct {
	msg string
}

func (e *unauthorizedError) Error() string { return e.msg }

func isUnauthorized(err error) bool {
	_, ok := err.(*unauthorizedError)
	return ok
}

type dropboxAPIArg struct {
	Path string `json:"path"`
	Mode string `json:"mode"`
}

func (d *DropboxUploader) doUpload(ctx context.Context, localPath string, remoteName string) error {
	uploadURL := "https://content.dropboxapi.com/2/files/upload"

	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open file for upload: %w", err)
	}
	defer file.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, file)
	if err != nil {
		return fmt.Errorf("failed to create upload request: %w", err)
	}

	d.mu.Lock()
	accessToken := d.tokens.AccessToken
	d.mu.Unlock()

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/octet-stream")

	apiArg := dropboxAPIArg{
		Path: filepath.Join(d.uploadPath, remoteName),
		Mode: "add",
	}
	apiArgJSON, _ := json.Marshal(apiArg)
	req.Header.Set("Dropbox-API-Arg", string(apiArgJSON))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute upload request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		slog.Info("Successfully uploaded file to Dropbox", "file", remoteName)
		return nil
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusUnauthorized {
		return &unauthorizedError{
			msg: fmt.Sprintf("dropbox returned 401: %s", string(bodyBytes)),
		}
	}

	return fmt.Errorf("dropbox API returned non-OK status: %s - Body: %s", resp.Status, string(bodyBytes))
}

func (d *DropboxUploader) refreshToken() error {
	slog.Info("Dropbox access token has expired, attempting to refresh...")

	tokenURL := "https://api.dropboxapi.com/oauth2/token"

	data := url.Values{}
	data.Set("grant_type", "refresh_token")

	d.mu.Lock()
	data.Set("refresh_token", d.tokens.RefreshToken)
	d.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create refresh request: %w", err)
	}

	req.SetBasicAuth(d.appKey, d.appSecret)
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute refresh request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("token refresh failed with status %s: %s", resp.Status, string(bodyBytes))
	}

	var result struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode refresh response: %w", err)
	}

	slog.Info("Successfully refreshed Dropbox access token")

	// Write to a temp file first, then rename for atomicity.
	d.mu.Lock()
	d.tokens.AccessToken = result.AccessToken
	tokensToSave := d.tokens
	d.mu.Unlock()

	tmp := d.tokenFile + ".tmp"
	file, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("failed to save refreshed token: %w", err)
	}

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(tokensToSave); err != nil {
		file.Close()
		os.Remove(tmp)
		return fmt.Errorf("failed to write refreshed token: %w", err)
	}
	if err := file.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("failed to close token file: %w", err)
	}
	if err := os.Rename(tmp, d.tokenFile); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("failed to rename token file: %w", err)
	}

	return nil
}
