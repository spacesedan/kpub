package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lmittmann/tint"
	"github.com/spf13/cobra"
	"github.com/spacesedan/kpub/internal/cli"
	"github.com/spacesedan/kpub/internal/config"
	"github.com/spacesedan/kpub/internal/dockerutil"
	"github.com/spacesedan/kpub/internal/supervisor"
)

var version = "dev"

const imageName = "ghcr.io/spacesedan/kpub"

func main() {
	rootCmd := &cobra.Command{
		Use:     "kpub",
		Short:   "kpub — monitors Telegram chats for ebooks, converts and uploads them to your Kobo",
		Version: version,
		RunE:    runServer,
	}
	rootCmd.Flags().String("config", "/data/config.yaml", "path to config file")

	// --- setup ---
	setupCmd := &cobra.Command{
		Use:   "setup",
		Short: "Interactive setup wizard to generate config.yaml and dropbox.json",
		RunE:  runSetup,
	}
	setupCmd.Flags().String("data-dir", "data", "directory for config.yaml and dropbox.json")

	// --- run ---
	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Pull image and start the container",
		RunE:  runRun,
	}
	runCmd.Flags().String("data-dir", "data", "directory to bind-mount as /data")
	runCmd.Flags().BoolP("detach", "d", false, "run container in the background")

	// --- update ---
	updateCmd := &cobra.Command{
		Use:   "update",
		Short: "Pull the latest kpub image",
		RunE:  runUpdate,
	}
	updateCmd.Flags().Bool("restart", false, "restart container after pulling")
	updateCmd.Flags().String("data-dir", "data", "directory to bind-mount as /data (used with --restart)")

	// --- stop ---
	stopCmd := &cobra.Command{
		Use:   "stop",
		Short: "Gracefully stop the running container",
		RunE:  runStop,
	}

	// --- reload ---
	reloadCmd := &cobra.Command{
		Use:   "reload",
		Short: "Restart the container to pick up config changes",
		RunE:  runReload,
	}
	reloadCmd.Flags().String("data-dir", "data", "directory to bind-mount as /data")

	// --- chat ---
	chatCmd := &cobra.Command{
		Use:   "chat",
		Short: "Manage monitored chat configurations",
	}
	chatCmd.PersistentFlags().String("data-dir", "data", "directory containing config.yaml")

	chatAddCmd := &cobra.Command{
		Use:   "add",
		Short: "Add a new chat to monitor",
		RunE:  runChatAdd,
	}

	chatListCmd := &cobra.Command{
		Use:   "list",
		Short: "List monitored chats",
		RunE:  runChatList,
	}

	chatRemoveCmd := &cobra.Command{
		Use:   "remove [@handle]",
		Short: "Remove a monitored chat",
		Args:  cobra.ExactArgs(1),
		RunE:  runChatRemove,
	}

	chatCmd.AddCommand(chatAddCmd, chatListCmd, chatRemoveCmd)

	rootCmd.AddCommand(setupCmd, runCmd, stopCmd, reloadCmd, updateCmd, chatCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// runServer is the default command — starts the Telegram chat monitor server.
func runServer(cmd *cobra.Command, args []string) error {
	slog.SetDefault(slog.New(tint.NewHandler(os.Stderr, &tint.Options{
		Level:     slog.LevelDebug,
		AddSource: true,
	})))

	configPath, _ := cmd.Flags().GetString("config")

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	slog.Info("Configuration loaded", "chats", len(cfg.Chats))

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	sv := supervisor.New(configPath, cfg, ctx)
	return sv.Run()
}

// runSetup launches the interactive setup wizard TUI.
func runSetup(cmd *cobra.Command, args []string) error {
	dataDir, _ := cmd.Flags().GetString("data-dir")
	m := cli.NewSetupModel(dataDir)
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("setup wizard: %w", err)
	}
	return nil
}

// runRun pulls the image and starts the Docker container.
func runRun(cmd *cobra.Command, args []string) error {
	if err := dockerutil.CheckDocker(); err != nil {
		return err
	}

	dataDir, _ := cmd.Flags().GetString("data-dir")
	detach, _ := cmd.Flags().GetBool("detach")

	// Resolve to absolute path for the bind mount.
	absDataDir, err := filepath.Abs(dataDir)
	if err != nil {
		return fmt.Errorf("resolving data-dir: %w", err)
	}

	image := imageName + ":latest"
	m := cli.NewRunModel(absDataDir, detach, image)
	p := tea.NewProgram(m)
	result, err := p.Run()
	if err != nil {
		return fmt.Errorf("run: %w", err)
	}

	// For foreground mode: Bubbletea exits after pull, then we hand off to docker run.
	rm := result.(cli.RunModel)
	if rm.NeedsForegroundRun() {
		return cli.RunForeground(image, absDataDir)
	}
	if rm.Err() != nil {
		log.Fatal(rm.Err())
	}

	return nil
}

// runUpdate pulls the latest kpub image.
func runUpdate(cmd *cobra.Command, args []string) error {
	if err := dockerutil.CheckDocker(); err != nil {
		return err
	}

	dataDir, _ := cmd.Flags().GetString("data-dir")
	restart, _ := cmd.Flags().GetBool("restart")

	absDataDir, err := filepath.Abs(dataDir)
	if err != nil {
		return fmt.Errorf("resolving data-dir: %w", err)
	}

	image := imageName + ":latest"
	m := cli.NewUpdateModel(absDataDir, restart, image)
	p := tea.NewProgram(m)
	result, err := p.Run()
	if err != nil {
		return fmt.Errorf("update: %w", err)
	}

	um := result.(cli.UpdateModel)
	if um.Err() != nil {
		log.Fatal(um.Err())
	}

	return nil
}

// runChatAdd launches the interactive TUI to add a new chat.
func runChatAdd(cmd *cobra.Command, args []string) error {
	dataDir, _ := cmd.Flags().GetString("data-dir")
	m := cli.NewAddChatModel(dataDir)
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("add chat: %w", err)
	}
	return nil
}

// runChatList prints all configured chats.
func runChatList(cmd *cobra.Command, args []string) error {
	dataDir, _ := cmd.Flags().GetString("data-dir")
	return cli.ListChats(dataDir)
}

// runChatRemove removes a chat by handle.
func runChatRemove(cmd *cobra.Command, args []string) error {
	dataDir, _ := cmd.Flags().GetString("data-dir")
	return cli.RemoveChat(dataDir, args[0])
}

const containerName = "kpub"

// runStop gracefully stops the running container.
func runStop(cmd *cobra.Command, args []string) error {
	if err := dockerutil.CheckDocker(); err != nil {
		return err
	}

	if err := dockerutil.StopContainer(containerName); err != nil {
		return err
	}

	fmt.Println("\n  " + cli.Success.Render("Container stopped."))
	return nil
}

// runReload restarts the container to pick up config changes.
func runReload(cmd *cobra.Command, args []string) error {
	if err := dockerutil.CheckDocker(); err != nil {
		return err
	}

	dataDir, _ := cmd.Flags().GetString("data-dir")
	absDataDir, err := filepath.Abs(dataDir)
	if err != nil {
		return fmt.Errorf("resolving data-dir: %w", err)
	}

	if err := dockerutil.StopContainer(containerName); err != nil {
		return err
	}

	image := imageName + ":latest"
	if err := dockerutil.RunContainer(containerName, image, absDataDir, true); err != nil {
		return err
	}

	fmt.Println("\n  " + cli.Success.Render("Container restarted."))
	return nil
}
