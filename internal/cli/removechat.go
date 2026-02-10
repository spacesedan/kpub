package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spacesedan/kpub/internal/config"
	"github.com/spacesedan/kpub/internal/setup"
)

// RemoveChat removes a chat by handle from the config, with confirmation.
func RemoveChat(dataDir, handle string) error {
	configPath := filepath.Join(dataDir, "config.yaml")
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	idx := -1
	for i, chat := range cfg.Chats {
		if chat.Handle == handle {
			idx = i
			break
		}
	}

	if idx == -1 {
		return fmt.Errorf("chat %q not found", handle)
	}

	if len(cfg.Chats) == 1 {
		return fmt.Errorf("cannot remove the only chat — at least one chat must be configured")
	}

	fmt.Printf("\n  Remove chat %s? [y/N] ", Highlight.Render(handle))
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	answer := strings.TrimSpace(strings.ToLower(scanner.Text()))

	if answer != "y" && answer != "yes" {
		fmt.Println("\n" + Warning.Render("  Aborted."))
		return nil
	}

	cfg.Chats = append(cfg.Chats[:idx], cfg.Chats[idx+1:]...)

	if err := setup.WriteConfig(dataDir, cfg); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	fmt.Println("\n  " + Success.Render(fmt.Sprintf("Chat %q removed.", handle)))
	return nil
}
