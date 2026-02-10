package setup

import (
	"strings"

	"github.com/spacesedan/kpub/internal/config"
)

// ChatInput holds a chat handle passed from the TUI.
type ChatInput struct {
	Handle string
}

// BuildConfig creates a config.Config from the wizard inputs.
func BuildConfig(appID int, appHash, dropboxAppKey, dropboxAppSecret string, chats []ChatInput) *config.Config {
	cfgChats := make([]config.ChatConfig, len(chats))
	for i, c := range chats {
		cfgChats[i] = config.ChatConfig{
			Handle: c.Handle,
		}
	}

	return &config.Config{
		Telegram: config.TelegramConfig{
			AppID:   appID,
			AppHash: appHash,
		},
		Defaults: config.DefaultsConfig{
			AcceptedFormats: []string{".epub", ".mobi", ".azw3"},
			Storage: config.StorageConfig{
				Type: "dropbox",
				Dropbox: config.DropboxConfig{
					AppKey:     dropboxAppKey,
					AppSecret:  dropboxAppSecret,
					TokenFile:  "/data/dropbox.json",
					UploadPath: "/Apps/Rakuten Kobo/",
				},
			},
		},
		Paths: config.PathsConfig{
			DownloadDir:  "/data/downloads",
			ConvertedDir: "/data/converted",
		},
		Chats: cfgChats,
	}
}

// Mask returns a partially redacted version of a secret string.
func Mask(s string) string {
	if len(s) <= 6 {
		return strings.Repeat("*", len(s))
	}
	return s[:3] + strings.Repeat("*", len(s)-6) + s[len(s)-3:]
}
