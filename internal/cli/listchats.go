package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spacesedan/kpub/internal/config"
)

// ListChats loads the config and prints all configured chats.
func ListChats(dataDir string) error {
	configPath := filepath.Join(dataDir, "config.yaml")
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if len(cfg.Chats) == 0 {
		fmt.Println(Warning.Render("No chats configured."))
		return nil
	}

	fmt.Println()
	fmt.Println("  " + Title.Render("Monitored chats:"))
	fmt.Println()
	for i, chat := range cfg.Chats {
		fmt.Printf("  %s\n", Highlight.Render(fmt.Sprintf("%d. %s", i+1, chat.Handle)))
	}
	fmt.Println()
	return nil
}
