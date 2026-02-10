# kpub

<!--toc:start-->
- [kpub](#kpub)
  - [Features](#features)
  - [Install](#install)
  - [Quick Start](#quick-start)
    - [1. Setup](#1-setup)
    - [2. Run](#2-run)
    - [3. Update](#3-update)
    - [4. Manage Chats](#4-manage-chats)
    - [5. Stop and Reload](#5-stop-and-reload)
  - [CLI Reference](#cli-reference)
    - [Flags](#flags)
  - [How It Works](#how-it-works)
  - [Requirements](#requirements)
  - [License](#license)
<!--toc:end-->

A Telegram chat monitor and ebook pipeline — watches your Telegram chats for ebook files, converts them to Kobo-compatible KEPUB format, and uploads them to Dropbox.

> **Disclaimer:** This tool is intended for use with **legally obtained ebooks only**. Please respect copyright laws and the rights of authors and publishers. The maintainers of this project are not responsible for how it is used.

> **Contributing:** This repository is not open for contributions. You are welcome to fork it and do whatever you want with it under the terms of the [MIT License](LICENSE).

If you find this useful, consider [buying me a coffee](https://ko-fi.com/spacesedan).

## Install

### Homebrew (macOS / Linux)

```bash
brew install spacesedan/tap/kpub
```

### Debian / Ubuntu

Download the `.deb` from the [releases page](https://github.com/spacesedan/kpub/releases), then:

```bash
sudo dpkg -i kpub_*.deb
```

### Fedora / RHEL

Download the `.rpm` from the [releases page](https://github.com/spacesedan/kpub/releases), then:

```bash
sudo rpm -i kpub_*.rpm
```

### Windows

Download the `.zip` from the [releases page](https://github.com/spacesedan/kpub/releases), extract it, and add `kpub.exe` to your PATH. Requires [Docker Desktop](https://www.docker.com/products/docker-desktop/).

### Download binary

Download the latest archive for your platform from the
[releases page](https://github.com/spacesedan/kpub/releases).

### Build from source

```bash
go install github.com/spacesedan/kpub/cmd/kpub@latest
```

## Features

- **Chat monitoring** — monitor any Telegram bot, group, or channel by handle (e.g. `@ebook-bot`)
- **Single user session** — authenticates once as your Telegram account, no bot tokens needed
- **Automatic conversion** — converts `.epub`, `.mobi`, `.azw3` to `.kepub.epub` using Calibre
- **Dropbox integration** — uploads converted files directly to your Kobo's sync folder
- **Hot reload** — add or remove monitored chats by editing the config file, no restart needed
- **Per-chat format filtering** — restrict accepted file types per chat
- **Per-chat upload paths** — route files from different chats to different folders
- **Graceful shutdown** — in-flight file processing completes before exit
- **Interactive TUI** — setup wizard, run, and update commands with Bubbletea-powered terminal UI

## Quick Start

### 1. Setup

The interactive setup wizard handles everything — Telegram credentials, Dropbox OAuth, and chat configuration — in a single command:

```bash
kpub setup
```

This walks you through 5 steps:

1. **Telegram credentials** — enter your `app_id` and `app_hash` from [my.telegram.org/apps](https://my.telegram.org/apps)
2. **Dropbox app credentials** — enter your `app_key` and `app_secret` from the [Dropbox App Console](https://www.dropbox.com/developers/apps)
3. **Dropbox authorization** — the wizard opens your browser to authorize the app, then you paste the code back
4. **Chat configuration** — add one or more chat handles to monitor (e.g. `@ebook-bot`)
5. **Review and save** — confirm and write `data/config.yaml` + `data/dropbox.json`

You can type `back` or press `Esc` at any step to return to the previous step. Press `Ctrl+C` to cancel.

To write files to a custom directory:

```bash
kpub setup --data-dir /path/to/dir
```

See [docs/telegram-setup.md](docs/telegram-setup.md) and [docs/dropbox-setup.md](docs/dropbox-setup.md) for more details on obtaining credentials. See [docs/config-reference.md](docs/config-reference.md) for all config options.

### 2. Run

Pull the image and start the container in one step:

```bash
kpub run
```

This will:

1. Remove any existing `kpub` container
2. Pull the latest `kpub` image from GHCR
3. Start the container with logs streaming to your terminal

On first run, you'll be prompted for your Telegram phone number and a verification code. After that, the session is saved and subsequent runs skip authentication.

To run in the background:

```bash
kpub run --detach
```

To use a custom data directory:

```bash
kpub run --data-dir /path/to/dir
```

### 3. Update

Pull the latest kpub image:

```bash
kpub update
```

To also restart the container after pulling:

```bash
kpub update --restart
```

### 4. Manage Chats

Add, list, or remove monitored chats without re-running the full setup wizard:

```bash
kpub chat list                  # show all monitored chats
kpub chat add                   # interactive prompt for a chat handle
kpub chat remove @ebook-bot     # remove a chat by handle (with confirmation)
```

These commands read and update the existing `config.yaml` — Telegram credentials, Dropbox settings, and other chats are left untouched.

The server automatically picks up config changes, so there's no need to restart after adding or removing chats.

### 5. Stop and Reload

Stop the running container gracefully:

```bash
kpub stop
```

Restart the container to pick up other config changes:

```bash
kpub reload
kpub reload --data-dir /path/to/dir
```

## CLI Reference

```
kpub                # Start the server (default behavior)
kpub setup          # Interactive setup wizard
kpub run            # Pull image + start container
kpub stop           # Gracefully stop the running container
kpub reload         # Restart container to pick up config changes
kpub update         # Pull latest kpub image
kpub chat list      # List monitored chats
kpub chat add       # Add a new chat (interactive)
kpub chat remove    # Remove a chat by handle
```

### Flags

| Command      | Flag         | Default            | Description                              |
|--------------|--------------|--------------------|------------------------------------------|
| (root)       | `--config`   | `/data/config.yaml`| Path to config file                      |
| setup        | `--data-dir` | `data`             | Directory for config.yaml and dropbox.json |
| run          | `--data-dir` | `data`             | Directory to bind-mount as /data         |
| run          | `--detach`   | `false`            | Run container in the background          |
| stop         | —            | —                  | No flags                                 |
| reload       | `--data-dir` | `data`             | Directory to bind-mount as /data         |
| update       | `--data-dir` | `data`             | Directory to bind-mount (used with --restart) |
| update       | `--restart`  | `false`            | Restart container after pulling          |
| chat (all)   | `--data-dir` | `data`             | Directory containing config.yaml         |

## How It Works

kpub is a single Go binary that runs inside a Docker image based on [linuxserver/calibre](https://github.com/linuxserver/docker-calibre). The pre-built image is published to GHCR (`ghcr.io/spacesedan/kpub`) — `kpub run` pulls it and starts the container.

1. The server connects to Telegram as your user account (single MTProto session)
2. It resolves each configured chat handle and monitors incoming messages
3. When a document appears in a monitored chat, the server downloads it via MTProto
4. The file is converted to `.kepub.epub` using Calibre's `ebook-convert`
5. The converted file is uploaded to Dropbox (syncs to your Kobo via the Dropbox app)
6. Status notifications are sent to your Saved Messages

## Requirements

- Docker
- Telegram API credentials (app_id + app_hash) from [my.telegram.org/apps](https://my.telegram.org/apps)
- Dropbox app credentials + OAuth tokens

## License

This project is licensed under the [MIT License](LICENSE).
