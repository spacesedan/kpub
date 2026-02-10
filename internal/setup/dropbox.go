package setup

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
)

// DropboxTokens holds the OAuth tokens returned by Dropbox.
type DropboxTokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// DropboxAuthURL constructs the Dropbox OAuth2 authorization URL.
func DropboxAuthURL(appKey string) string {
	return fmt.Sprintf(
		"https://www.dropbox.com/oauth2/authorize?client_id=%s&response_type=code&token_access_type=offline",
		url.QueryEscape(appKey),
	)
}

// OpenBrowser tries to open the given URL in the user's default browser.
func OpenBrowser(u string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
		args = []string{u}
	case "linux":
		cmd = "xdg-open"
		args = []string{u}
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", u}
	default:
		return fmt.Errorf("unsupported platform %q", runtime.GOOS)
	}

	return exec.Command(cmd, args...).Start()
}

// ExchangeDropboxCode exchanges an authorization code for access and refresh tokens.
func ExchangeDropboxCode(appKey, appSecret, code string) (*DropboxTokens, error) {
	tokenURL := "https://api.dropboxapi.com/oauth2/token"

	data := url.Values{}
	data.Set("code", code)
	data.Set("grant_type", "authorization_code")

	req, err := http.NewRequest(http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("creating token request: %w", err)
	}

	req.SetBasicAuth(appKey, appSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing token request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Dropbox returned %s: %s", resp.Status, string(body))
	}

	var tokens DropboxTokens
	if err := json.Unmarshal(body, &tokens); err != nil {
		return nil, fmt.Errorf("parsing token response: %w", err)
	}

	if tokens.AccessToken == "" || tokens.RefreshToken == "" {
		return nil, fmt.Errorf("response missing access_token or refresh_token")
	}

	return &tokens, nil
}
