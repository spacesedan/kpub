package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/tg"
	"github.com/lmittmann/tint"
)

type Config struct {
	AppID         int
	AppHash       string
	BotToken      string
	DropboxKey    string
	DropBoxSecret string
}

var (
	currentTokens DropboxTokens
	tokenFilePath = "/data/dropbox.json"
)

func main() {
	logger := slog.New(tint.NewHandler(os.Stderr, &tint.Options{
		Level:     slog.LevelDebug,
		AddSource: true,
	}))

	slog.SetDefault(logger)

	appIDStr := os.Getenv("APP_ID")
	appHash := os.Getenv("APP_HASH")
	botToken := os.Getenv("BOT_TOKEN")
	dropboxKey := os.Getenv("DROPBOX_KEY")
	dropboxSecret := os.Getenv("DROPBOX_SECRET")

	if appIDStr == "" || appHash == "" || botToken == "" || dropboxKey == "" || dropboxSecret == "" {
		log.Fatal("FATAL: Env vars APP_ID, APP_HASH, BOT_TOKEN, DROPBOX_KEY, and DROPBOX_SECRET must be set")
	}

	appID, _ := strconv.Atoi(appIDStr)

	cfg := Config{
		AppID:         appID,
		AppHash:       appHash,
		BotToken:      botToken,
		DropboxKey:    dropboxKey,
		DropBoxSecret: dropboxSecret,
	}

	_, err := os.Stat(tokenFilePath)
	if os.IsNotExist(err) {
		log.Fatal("missing token file")
	}

	file, err := os.Open(tokenFilePath)
	if err != nil {
		log.Fatalf("FATAL: Could not open token file '%s'. Check permissions. Error: %v", tokenFilePath, err)
	}
	defer file.Close()

	if err := json.NewDecoder(file).Decode(&currentTokens); err != nil {
		log.Fatalf("FATAL: 'access_token' or 'refresh_token' is missing from the token file '%s'", tokenFilePath)
	}

	if currentTokens.AccessToken == "" || currentTokens.RefreshToken == "" {
		log.Fatalf("FATAL: 'access_token' or 'refresh_token' is missing form the token file '%s'.", tokenFilePath)
	}

	slog.Info("Successfully loaded Dropbox tokens.")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	client := telegram.NewClient(cfg.AppID, cfg.AppHash, telegram.Options{})

	slog.Info("Starting Telegram Client")
	if err := client.Run(ctx, func(ctx context.Context) error {
		dispatcher := tg.NewUpdateDispatcher()
		opts := telegram.Options{
			UpdateHandler: dispatcher,
		}

		slog.Info("Attempting to connect to Z-Lib Bot...")
		return telegram.BotFromEnvironment(ctx, opts, func(ctx context.Context, client *telegram.Client) error {
			slog.Info("Successfully connected to Z-Lib Bot, listening for outgoing messages")

			d := downloader.NewDownloader()
			api := tg.NewClient(client)
			dispatcher.OnNewMessage(func(ctx context.Context, e tg.Entities, update *tg.UpdateNewMessage) error {
				message, ok := update.Message.(*tg.Message)
				if !ok {
					log.Printf("Received a non-standard message type: %T. Ignoring.", update.Message)
					return nil
				}

				media, ok := message.Media.(*tg.MessageMediaDocument)
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
					slog.Warn("Received a document with no filename attribute")
					return nil
				}

				go processFile(api, d, doc, fileName, message.PeerID, e, cfg)

				return nil
			})
			return nil
		}, telegram.RunUntilCanceled)
	}); err != nil {
		log.Fatalf("Telegram client failed to run: %v", err)
	}
}

func resolvePeerManually(entities tg.Entities, peerID tg.PeerClass) (tg.InputPeerClass, error) {
	switch p := peerID.(type) {
	case *tg.PeerUser:
		user, ok := entities.Users[p.UserID]
		if !ok {
			return nil, fmt.Errorf("user with ID %d not found in entities", p.UserID)
		}
		return user.AsInputPeer(), nil
	case *tg.PeerChat:
		chat, ok := entities.Chats[p.ChatID]
		if !ok {
			return nil, fmt.Errorf("chat with ID %d not found in entities", p.ChatID)
		}
		return chat.AsInputPeer(), nil
	case *tg.PeerChannel:
		channel, ok := entities.Channels[p.ChannelID]
		if !ok {
			return nil, fmt.Errorf("channel with ID %d not found in entities", p.ChannelID)
		}
		return channel.AsInputPeer(), nil
	default:
		return nil, fmt.Errorf("unknown peer type: %T", p)
	}
}

func processFile(api *tg.Client, d *downloader.Downloader, doc *tg.Document, fileName string, peerID tg.PeerClass, entities tg.Entities, cfg Config) {
	slog.Info("File Received! Starting process", slog.String("fileName", fileName))

	inputPeer, err := resolvePeerManually(entities, peerID)
	if err != nil {
		slog.Error("Could not resolve peer", slog.String("error", err.Error()))
		return
	}

	const downloadDir = "/data/downloads"
	const convertedDir = "/data/converted"
	_ = os.MkdirAll(downloadDir, os.ModePerm)
	_ = os.MkdirAll(convertedDir, os.ModePerm)
	downloadPath := filepath.Join(downloadDir, fileName)
	defer os.Remove(downloadPath)

	slog.Info("Downloading", slog.String("fileName", fileName))
	sendMessage(api, inputPeer, fmt.Sprintf("Downloaded '%s'. Now converting...", fileName))
	location := doc.AsInputDocumentFileLocation()
	_, err = d.Download(api, location).ToPath(context.Background(), downloadPath)
	if err != nil {
		slog.Error("Failed to download file", slog.Any("reason", err))
		sendMessage(api, inputPeer, fmt.Sprintf("Sorry, I couldn't downlaod '%s'.", fileName))
		return
	}

	slog.Info("Download complete. Converting to KEPUB")
	kepubPath, err := convertToKepub(downloadPath)
	if err != nil {
		slog.Error("Falied to convert to KEPUB",
			slog.String("fileName", fileName),
			slog.String("reason", err.Error()))
		sendMessage(api, inputPeer, fmt.Sprintf("Sorry, I failed to convert '%s'.", fileName))
		return
	}
	defer os.Remove(kepubPath)

	slog.Info("Conversion complete. Uploading to Dropbox", slog.String("filename", filepath.Base(kepubPath)))
	sendMessage(api, inputPeer, fmt.Sprintf("Conversion completed! Uploading '%s' to Dropbox", filepath.Base(kepubPath)))
	err = uploadToDropbox(kepubPath, cfg)
	if err != nil {
		slog.Error("Failed to upload to Dropbox", slog.String("reason", err.Error()))
		sendMessage(api, inputPeer, fmt.Sprintf("Sorry, I failed to upload '%s' to Dropbox...", filepath.Base(kepubPath)))
		return
	}

	slog.Info("Success! Pipeline complete", slog.String("filename", filepath.Base(kepubPath)))
	sendMessage(api, inputPeer, fmt.Sprintf("Success! '%s' is now in your Dropbox", filepath.Base(kepubPath)))
}

func sendMessage(api *tg.Client, peer tg.InputPeerClass, message string) {
	_, _ = api.MessagesSendMedia(context.Background(), &tg.MessagesSendMediaRequest{
		Peer:     peer,
		Message:  message,
		RandomID: time.Now().UnixNano(),
	})
}

func convertToKepub(inputFile string) (string, error) {
	const convertedDir = "/data/converted"
	baseName := filepath.Base(inputFile)
	ext := filepath.Ext(baseName)
	newBaseName := strings.TrimSuffix(baseName, ext) + ".kepub.epub"
	outputFile := filepath.Join(convertedDir, newBaseName)

	slog.Info("Starting conversion with ebook-convert", "input", inputFile, "output", outputFile)

	cmd := exec.Command("ebook-convert", inputFile, outputFile)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ebook-convert failed: %v\nStderr: %s", err, stderr.String())
	}

	slog.Info("ebook-convert completed ssuccessfully")
	return outputFile, nil
}

type DropboxTokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

func refreshDropboxToken(cfg Config) error {
	slog.Info("Dropbox access token has expired. Attempting to refresh...")

	tokenURL := "https://api.dropboxapi.com/oauth2/token"

	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", currentTokens.RefreshToken)

	req, err := http.NewRequest(http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create refresh request: %v", err)
	}

	req.SetBasicAuth(cfg.DropboxKey, cfg.DropBoxSecret)
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute refresh token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("token refesh failed with status %s: %s", resp.Status, string(bodyBytes))
	}

	var newAccessTokenResponse struct {
		AccessToken string `json:"access_token"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&newAccessTokenResponse); err != nil {
		return fmt.Errorf("failed to decode refresh response: %v", err)
	}

	slog.Info("Successfully refreshed Dropbox access token")

	currentTokens.AccessToken = newAccessTokenResponse.AccessToken

	file, err := os.Create(tokenFilePath)
	if err != nil {
		slog.Error("CRITICAL: Failed to open token file for writing", "error", err)
		return nil
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(currentTokens); err != nil {
		slog.Error("CRITICAL: failed to write new token to file", "error", err)
	}

	return nil
}

type DropboxAPIArg struct {
	Path string `json:"path"`
	Mode string `json:"mode"`
}

func uploadToDropbox(localFilePath string, cfg Config) error {
	for i := 0; i < 2; i++ {
		url := "https://content.dropboxapi.com/2/files/upload"
		file, err := os.Open(localFilePath)
		if err != nil {
			return nil
		}
		defer file.Close()

		req, err := http.NewRequest(http.MethodPost, url, file)
		if err != nil {
			return nil
		}

		req.Header.Set("Authorization", "Bearer "+currentTokens.AccessToken)
		req.Header.Set("Content-Type", "application/octet-stream")
		apiArg := DropboxAPIArg{
			Path: `/Apps/Rakuten Kobo/` + filepath.Base(localFilePath),
			Mode: "add",
		}
		apiArgJSON, _ := json.Marshal(apiArg)
		req.Header.Set("Dropbox-API-Arg", string(apiArgJSON))

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to execute upload request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			slog.Info("Successfully uploaded file to Dropbox", "file", localFilePath)
			return nil
		}

		if resp.StatusCode == http.StatusUnauthorized {
			slog.Warn("Dropbox upload failed with 401 Unauthorized. This is likely an expirated token")

			if i == 0 {
				refreshErr := refreshDropboxToken(cfg)
				if refreshErr != nil {
					return fmt.Errorf("failed to refresh token, cannot retry upload: %w", refreshErr)
				}
				slog.Info("Retrying Dropbox upload with new token...")
				continue
			}
		}
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("dropbox API returned a non-OK status: %s - Body: %s", resp.Status, string(bodyBytes))
	}
	return fmt.Errorf("dropbox upload failed after multiple retries")
}
