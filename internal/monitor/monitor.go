package monitor

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/tg"

	"github.com/spacesedan/kpub/internal/converter"
	"github.com/spacesedan/kpub/internal/storage"
)

// monitoredChat holds config for a single monitored chat.
type monitoredChat struct {
	handle   string
	formats  map[string]bool
	uploader storage.Uploader
}

// Monitor manages a single Telegram user client that monitors multiple chats
// for ebook files.
type Monitor struct {
	appID        int
	appHash      string
	sessionPath  string
	downloadDir  string
	convertedDir string

	mu    sync.RWMutex
	peers map[string]*monitoredChat // "u123" or "c456" → chat config

	api        *tg.Client
	downloader *downloader.Downloader
	ready      chan struct{}
	wg         sync.WaitGroup
	logger     *slog.Logger
}

// New creates a Monitor from Telegram config and paths.
func New(appID int, appHash, sessionPath, downloadDir, convertedDir string) *Monitor {
	return &Monitor{
		appID:        appID,
		appHash:      appHash,
		sessionPath:  sessionPath,
		downloadDir:  downloadDir,
		convertedDir: convertedDir,
		peers:        make(map[string]*monitoredChat),
		ready:        make(chan struct{}),
		logger:       slog.Default().With("component", "monitor"),
	}
}

// Ready returns a channel that is closed when the monitor is connected and
// authenticated. Callers should wait on this before calling AddChat.
func (m *Monitor) Ready() <-chan struct{} {
	return m.ready
}

// Run connects to Telegram as a user, authenticates if needed, and listens
// for messages until ctx is cancelled.
func (m *Monitor) Run(ctx context.Context) error {
	dispatcher := tg.NewUpdateDispatcher()

	client := telegram.NewClient(m.appID, m.appHash, telegram.Options{
		UpdateHandler:  dispatcher,
		SessionStorage: &session.FileStorage{Path: m.sessionPath},
	})

	return client.Run(ctx, func(ctx context.Context) error {
		status, err := client.Auth().Status(ctx)
		if err != nil {
			return fmt.Errorf("getting auth status: %w", err)
		}

		if !status.Authorized {
			m.logger.Info("Not authorized, starting user authentication...")
			flow := auth.NewFlow(terminalAuth{}, auth.SendCodeOptions{})
			if err := flow.Run(ctx, client.Auth()); err != nil {
				return fmt.Errorf("user auth failed: %w", err)
			}
			m.logger.Info("Authentication successful")
		}

		m.api = tg.NewClient(client)
		m.downloader = downloader.NewDownloader()

		m.logger.Info("Connected and ready to monitor chats")
		close(m.ready)

		dispatcher.OnNewMessage(m.handleMessage)
		dispatcher.OnNewChannelMessage(m.handleChannelMessage)

		<-ctx.Done()
		m.logger.Info("Shutting down, waiting for in-flight files to complete...")
		m.wg.Wait()
		m.logger.Info("All in-flight files completed, monitor stopped")
		return nil
	})
}

// AddChat resolves a handle and adds it to the monitored set.
func (m *Monitor) AddChat(ctx context.Context, handle string, formats map[string]bool, uploader storage.Uploader) error {
	username := strings.TrimPrefix(handle, "@")

	resolved, err := m.api.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{
		Username: username,
	})
	if err != nil {
		return fmt.Errorf("resolving handle %q: %w", handle, err)
	}

	key := peerKey(resolved.Peer)
	if key == "" {
		return fmt.Errorf("unexpected peer type for %q: %T", handle, resolved.Peer)
	}

	m.mu.Lock()
	m.peers[key] = &monitoredChat{
		handle:   handle,
		formats:  formats,
		uploader: uploader,
	}
	m.mu.Unlock()

	m.logger.Info("Now monitoring chat", "handle", handle, "key", key)
	return nil
}

// RemoveChat removes a handle from the monitored set.
func (m *Monitor) RemoveChat(handle string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for key, chat := range m.peers {
		if chat.handle == handle {
			delete(m.peers, key)
			m.logger.Info("Stopped monitoring chat", "handle", handle, "key", key)
			return
		}
	}
}

// handleMessage handles messages from DMs and basic groups.
func (m *Monitor) handleMessage(ctx context.Context, e tg.Entities, update *tg.UpdateNewMessage) error {
	msg, ok := update.Message.(*tg.Message)
	if !ok {
		return nil
	}
	if msg.Out {
		return nil
	}

	var key string
	switch p := msg.PeerID.(type) {
	case *tg.PeerUser:
		key = fmt.Sprintf("u%d", p.UserID)
	case *tg.PeerChat:
		key = fmt.Sprintf("c%d", p.ChatID)
	default:
		return nil
	}

	m.mu.RLock()
	chat, monitored := m.peers[key]
	m.mu.RUnlock()

	if !monitored {
		return nil
	}

	return m.processDocument(ctx, msg, chat)
}

// handleChannelMessage handles messages from channels and supergroups.
func (m *Monitor) handleChannelMessage(ctx context.Context, e tg.Entities, update *tg.UpdateNewChannelMessage) error {
	msg, ok := update.Message.(*tg.Message)
	if !ok {
		return nil
	}
	if msg.Out {
		return nil
	}

	p, ok := msg.PeerID.(*tg.PeerChannel)
	if !ok {
		return nil
	}

	key := fmt.Sprintf("c%d", p.ChannelID)

	m.mu.RLock()
	chat, monitored := m.peers[key]
	m.mu.RUnlock()

	if !monitored {
		return nil
	}

	return m.processDocument(ctx, msg, chat)
}

// processDocument extracts a document from a message and kicks off processing.
func (m *Monitor) processDocument(ctx context.Context, msg *tg.Message, chat *monitoredChat) error {
	media, ok := msg.Media.(*tg.MessageMediaDocument)
	if !ok {
		return nil
	}

	doc, ok := media.Document.AsNotEmpty()
	if !ok {
		return nil
	}

	var fileName string
	for _, attr := range doc.Attributes {
		if f, ok := attr.(*tg.DocumentAttributeFilename); ok {
			fileName = f.FileName
			break
		}
	}

	if fileName == "" {
		m.logger.Warn("Received a document with no filename attribute", "chat", chat.handle)
		return nil
	}

	ext := strings.ToLower(filepath.Ext(fileName))
	if !chat.formats[ext] {
		m.logger.Info("Rejected file with unsupported format",
			slog.String("chat", chat.handle),
			slog.String("fileName", fileName),
			slog.String("extension", ext))
		return nil
	}

	// Use a context that won't be cancelled on shutdown so in-flight
	// file processing can complete while wg.Wait() blocks.
	fileCtx := context.WithoutCancel(ctx)
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.processFile(fileCtx, doc, fileName, chat)
	}()

	return nil
}

// processFile downloads, converts, and uploads an ebook file.
func (m *Monitor) processFile(ctx context.Context, doc *tg.Document, fileName string, chat *monitoredChat) {
	m.logger.Info("File received, starting process",
		slog.String("chat", chat.handle),
		slog.String("fileName", fileName))

	if err := os.MkdirAll(m.downloadDir, 0o750); err != nil {
		m.logger.Error("Failed to create download directory", slog.Any("reason", err))
		return
	}
	if err := os.MkdirAll(m.convertedDir, 0o750); err != nil {
		m.logger.Error("Failed to create converted directory", slog.Any("reason", err))
		return
	}
	downloadPath := filepath.Join(m.downloadDir, fileName)
	defer os.Remove(downloadPath)

	m.notify(ctx, fmt.Sprintf("[kpub] Processing '%s' from %s...", fileName, chat.handle))

	// Download
	m.logger.Info("Downloading", slog.String("fileName", fileName))
	location := doc.AsInputDocumentFileLocation()
	_, err := m.downloader.Download(m.api, location).ToPath(ctx, downloadPath)
	if err != nil {
		m.logger.Error("Failed to download file", slog.Any("reason", err))
		m.notify(ctx, fmt.Sprintf("[kpub] Failed to process '%s'.", fileName))
		return
	}

	// Convert
	m.logger.Info("Download complete, converting to KEPUB")
	kepubPath, err := converter.Convert(ctx, downloadPath, m.convertedDir)
	if err != nil {
		m.logger.Error("Failed to convert to KEPUB",
			slog.String("fileName", fileName),
			slog.String("reason", err.Error()))
		m.notify(ctx, fmt.Sprintf("[kpub] Failed to process '%s'.", fileName))
		return
	}
	defer os.Remove(kepubPath)

	// Upload
	remoteName := filepath.Base(kepubPath)
	m.logger.Info("Conversion complete, uploading to storage", slog.String("fileName", remoteName))
	err = chat.uploader.Upload(ctx, kepubPath, remoteName)
	if err != nil {
		m.logger.Error("Failed to upload", slog.String("reason", err.Error()))
		m.notify(ctx, fmt.Sprintf("[kpub] Failed to process '%s'.", fileName))
		return
	}

	m.logger.Info("Success! Pipeline complete", slog.String("fileName", remoteName))
	m.notify(ctx, fmt.Sprintf("[kpub] Done! '%s' is ready on your Kobo.", remoteName))
}

// notify sends a status message to the user's Saved Messages.
func (m *Monitor) notify(ctx context.Context, text string) {
	_, _ = m.api.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
		Peer:     &tg.InputPeerSelf{},
		Message:  text,
		RandomID: time.Now().UnixNano(),
	})
}

// peerKey returns a string key for a PeerClass ("u123" for users, "c456" for
// chats/channels).
func peerKey(peer tg.PeerClass) string {
	switch p := peer.(type) {
	case *tg.PeerUser:
		return fmt.Sprintf("u%d", p.UserID)
	case *tg.PeerChat:
		return fmt.Sprintf("c%d", p.ChatID)
	case *tg.PeerChannel:
		return fmt.Sprintf("c%d", p.ChannelID)
	default:
		return ""
	}
}
