package supervisor

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/spacesedan/kpub/internal/config"
	"github.com/spacesedan/kpub/internal/monitor"
	"github.com/spacesedan/kpub/internal/storage"
)

// Supervisor manages the lifecycle of a single Monitor, watching the config
// file for changes and adding/removing chats as needed.
type Supervisor struct {
	configPath string
	cfg        *config.Config
	ctx        context.Context
	monitor    *monitor.Monitor
	uploaders  map[string]storage.Uploader
	mu         sync.Mutex
}

// New creates a Supervisor.
func New(configPath string, cfg *config.Config, ctx context.Context) *Supervisor {
	return &Supervisor{
		configPath: configPath,
		cfg:        cfg,
		ctx:        ctx,
		uploaders:  make(map[string]storage.Uploader),
	}
}

// Run creates and starts the monitor, adds initial chats, then watches the
// config file for changes. Blocks until the parent context is cancelled.
func (s *Supervisor) Run() error {
	// Create the monitor.
	m := monitor.New(
		s.cfg.Telegram.AppID,
		s.cfg.Telegram.AppHash,
		"/data/session.json",
		s.cfg.Paths.DownloadDir,
		s.cfg.Paths.ConvertedDir,
	)
	s.monitor = m

	// Start monitor in background.
	monitorCtx, monitorCancel := context.WithCancel(s.ctx)
	defer monitorCancel()

	monitorDone := make(chan error, 1)
	go func() {
		monitorDone <- m.Run(monitorCtx)
	}()

	// Wait for the monitor to be ready (authenticated + connected).
	select {
	case <-m.Ready():
		slog.Info("Monitor is ready")
	case err := <-monitorDone:
		return fmt.Errorf("monitor exited during startup: %w", err)
	case <-s.ctx.Done():
		monitorCancel()
		<-monitorDone
		return nil
	}

	// Add all initial chats.
	for _, chatCfg := range s.cfg.Chats {
		resolved := config.ResolvedChatConfig(s.cfg.Defaults, chatCfg)
		if err := s.addChat(resolved); err != nil {
			slog.Error("Failed to add initial chat", "handle", resolved.Handle, "error", err)
		}
	}

	// Set up file watcher.
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("creating file watcher: %w", err)
	}
	defer watcher.Close()

	if err := watcher.Add(s.configPath); err != nil {
		return fmt.Errorf("watching config file: %w", err)
	}

	slog.Info("Watching config file for changes", "path", s.configPath)

	var debounce *time.Timer

	for {
		select {
		case <-s.ctx.Done():
			if debounce != nil {
				debounce.Stop()
			}
			slog.Info("Shutting down supervisor")
			monitorCancel()
			<-monitorDone
			return nil

		case err := <-monitorDone:
			if s.ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("monitor exited unexpectedly: %w", err)

		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}

			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Rename) {
				if debounce != nil {
					debounce.Stop()
				}
				debounce = time.AfterFunc(500*time.Millisecond, func() {
					s.reload()
					// Re-add the watch in case the file was replaced (atomic rename).
					_ = watcher.Add(s.configPath)
				})
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			slog.Error("File watcher error", "error", err)
		}
	}
}

// addChat creates an uploader and registers a chat with the monitor.
func (s *Supervisor) addChat(resolved config.ResolvedChat) error {
	tokenFile := resolved.Storage.Dropbox.TokenFile
	uploader, ok := s.uploaders[tokenFile]
	if !ok {
		var err error
		uploader, err = storage.NewUploader(resolved.Storage)
		if err != nil {
			return fmt.Errorf("creating uploader: %w", err)
		}
		s.uploaders[tokenFile] = uploader
	}

	if err := s.monitor.AddChat(s.ctx, resolved.Handle, resolved.AcceptedFormats, uploader); err != nil {
		return err
	}

	return nil
}

// reload reads the config file and reconciles the monitored chats.
func (s *Supervisor) reload() {
	slog.Info("Config file changed, reloading...")

	newCfg, err := config.Load(s.configPath)
	if err != nil {
		slog.Error("Failed to reload config, keeping existing chats", "error", err)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Build map of new chat configs by handle.
	newChats := make(map[string]config.ResolvedChat, len(newCfg.Chats))
	for _, chatCfg := range newCfg.Chats {
		resolved := config.ResolvedChatConfig(newCfg.Defaults, chatCfg)
		newChats[resolved.Handle] = resolved
	}

	// Build map of old chat configs by handle.
	oldChats := make(map[string]config.ResolvedChat, len(s.cfg.Chats))
	for _, chatCfg := range s.cfg.Chats {
		resolved := config.ResolvedChatConfig(s.cfg.Defaults, chatCfg)
		oldChats[resolved.Handle] = resolved
	}

	// Removed chats: in old, not in new.
	for handle := range oldChats {
		if _, exists := newChats[handle]; !exists {
			slog.Info("Removing chat", "handle", handle)
			s.monitor.RemoveChat(handle)
		}
	}

	// Update shared config.
	s.cfg = newCfg

	// Added or changed chats.
	for handle, newResolved := range newChats {
		if oldResolved, exists := oldChats[handle]; exists {
			if !chatConfigEqual(oldResolved, newResolved) {
				slog.Info("Chat config changed, re-adding", "handle", handle)
				s.monitor.RemoveChat(handle)
				if err := s.addChat(newResolved); err != nil {
					slog.Error("Failed to re-add chat after config change", "handle", handle, "error", err)
				}
			}
		} else {
			slog.Info("Adding new chat", "handle", handle)
			if err := s.addChat(newResolved); err != nil {
				slog.Error("Failed to add new chat", "handle", handle, "error", err)
			}
		}
	}
}

// chatConfigEqual compares two resolved chat configs to detect changes.
func chatConfigEqual(a, b config.ResolvedChat) bool {
	if a.Storage != b.Storage {
		return false
	}
	if !reflect.DeepEqual(a.AcceptedFormats, b.AcceptedFormats) {
		return false
	}
	return true
}
