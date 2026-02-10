package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the top-level configuration loaded from YAML.
type Config struct {
	Telegram TelegramConfig `yaml:"telegram"`
	Defaults DefaultsConfig `yaml:"defaults"`
	Paths    PathsConfig    `yaml:"paths"`
	Chats    []ChatConfig   `yaml:"chats"`
}

type TelegramConfig struct {
	AppID   int    `yaml:"app_id"`
	AppHash string `yaml:"app_hash"`
}

type DefaultsConfig struct {
	AcceptedFormats []string      `yaml:"accepted_formats"`
	Storage         StorageConfig `yaml:"storage"`
}

type StorageConfig struct {
	Type    string        `yaml:"type"`
	Dropbox DropboxConfig `yaml:"dropbox"`
}

type DropboxConfig struct {
	AppKey     string `yaml:"app_key"`
	AppSecret  string `yaml:"app_secret"`
	TokenFile  string `yaml:"token_file"`
	UploadPath string `yaml:"upload_path"`
}

type PathsConfig struct {
	DownloadDir  string `yaml:"download_dir"`
	ConvertedDir string `yaml:"converted_dir"`
}

type ChatConfig struct {
	Handle          string         `yaml:"handle"`
	AcceptedFormats []string       `yaml:"accepted_formats,omitempty"`
	Storage         *StorageConfig `yaml:"storage,omitempty"`
}

// ResolvedChat holds the fully-merged configuration for a single monitored chat.
type ResolvedChat struct {
	Handle          string
	AcceptedFormats map[string]bool
	Storage         StorageConfig
}

// Load reads the YAML config file, applies defaults, and validates.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	applyDefaults(&cfg)

	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}

	return &cfg, nil
}

func applyDefaults(cfg *Config) {
	if len(cfg.Defaults.AcceptedFormats) == 0 {
		cfg.Defaults.AcceptedFormats = []string{".epub", ".mobi", ".azw3"}
	}
	if cfg.Defaults.Storage.Type == "" {
		cfg.Defaults.Storage.Type = "dropbox"
	}
	if cfg.Defaults.Storage.Dropbox.TokenFile == "" {
		cfg.Defaults.Storage.Dropbox.TokenFile = "/data/dropbox.json"
	}
	if cfg.Defaults.Storage.Dropbox.UploadPath == "" {
		cfg.Defaults.Storage.Dropbox.UploadPath = "/Apps/Rakuten Kobo/"
	}
	if cfg.Paths.DownloadDir == "" {
		cfg.Paths.DownloadDir = "/data/downloads"
	}
	if cfg.Paths.ConvertedDir == "" {
		cfg.Paths.ConvertedDir = "/data/converted"
	}
}

func validate(cfg *Config) error {
	if cfg.Telegram.AppID == 0 {
		return fmt.Errorf("telegram.app_id is required")
	}
	if cfg.Telegram.AppHash == "" {
		return fmt.Errorf("telegram.app_hash is required")
	}
	if len(cfg.Chats) == 0 {
		return fmt.Errorf("at least one chat must be configured")
	}

	handles := make(map[string]bool)
	for i, chat := range cfg.Chats {
		if chat.Handle == "" {
			return fmt.Errorf("chats[%d].handle is required", i)
		}
		if !strings.HasPrefix(chat.Handle, "@") {
			return fmt.Errorf("chats[%d].handle must start with @", i)
		}
		if handles[chat.Handle] {
			return fmt.Errorf("duplicate chat handle: %q", chat.Handle)
		}
		handles[chat.Handle] = true
	}

	// Validate storage config for defaults (and any chat-level overrides)
	if cfg.Defaults.Storage.Type == "dropbox" {
		d := cfg.Defaults.Storage.Dropbox
		if d.AppKey == "" {
			return fmt.Errorf("defaults.storage.dropbox.app_key is required")
		}
		if d.AppSecret == "" {
			return fmt.Errorf("defaults.storage.dropbox.app_secret is required")
		}
	}

	return nil
}

// ResolvedChatConfig merges per-chat overrides onto global defaults.
func ResolvedChatConfig(defaults DefaultsConfig, chat ChatConfig) ResolvedChat {
	// Accepted formats: use chat-specific if provided, else global defaults
	formats := defaults.AcceptedFormats
	if len(chat.AcceptedFormats) > 0 {
		formats = chat.AcceptedFormats
	}

	fmtMap := make(map[string]bool, len(formats))
	for _, f := range formats {
		fmtMap[strings.ToLower(f)] = true
	}

	// Storage: start with global defaults, overlay chat-specific fields
	storage := defaults.Storage
	if chat.Storage != nil {
		if chat.Storage.Type != "" {
			storage.Type = chat.Storage.Type
		}
		// Merge dropbox sub-fields
		if chat.Storage.Dropbox.AppKey != "" {
			storage.Dropbox.AppKey = chat.Storage.Dropbox.AppKey
		}
		if chat.Storage.Dropbox.AppSecret != "" {
			storage.Dropbox.AppSecret = chat.Storage.Dropbox.AppSecret
		}
		if chat.Storage.Dropbox.TokenFile != "" {
			storage.Dropbox.TokenFile = chat.Storage.Dropbox.TokenFile
		}
		if chat.Storage.Dropbox.UploadPath != "" {
			storage.Dropbox.UploadPath = chat.Storage.Dropbox.UploadPath
		}
	}

	return ResolvedChat{
		Handle:          chat.Handle,
		AcceptedFormats: fmtMap,
		Storage:         storage,
	}
}
