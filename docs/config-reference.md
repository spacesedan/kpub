# Configuration Reference

The server is configured via a single YAML file (default: `/data/config.yaml`).

## Full Example

```yaml
telegram:
  app_id: 12345678
  app_hash: "abcdef1234567890"

defaults:
  accepted_formats: [".epub", ".mobi", ".azw3"]
  storage:
    type: dropbox
    dropbox:
      app_key: "your-key"
      app_secret: "your-secret"
      token_file: "/data/dropbox.json"
      upload_path: "/Apps/Rakuten Kobo/"

paths:
  download_dir: "/data/downloads"
  converted_dir: "/data/converted"

chats:
  - handle: "@ebook-bot"

  - handle: "@another-bot"
    accepted_formats: [".epub"]
    storage:
      dropbox:
        upload_path: "/Apps/Rakuten Kobo/Fiction/"
```

## Sections

### `telegram` (required)

| Field      | Type   | Required | Description                        |
|------------|--------|----------|------------------------------------|
| `app_id`   | int    | yes      | Telegram API application ID        |
| `app_hash` | string | yes      | Telegram API application hash      |

### `defaults` (optional)

Global defaults applied to all chats unless overridden.

| Field              | Type     | Default                          | Description                     |
|--------------------|----------|----------------------------------|---------------------------------|
| `accepted_formats` | []string | `[".epub", ".mobi", ".azw3"]`    | File extensions to accept       |
| `storage.type`     | string   | `"dropbox"`                      | Storage backend type            |

### `defaults.storage.dropbox`

| Field         | Type   | Default                  | Description                      |
|---------------|--------|--------------------------|----------------------------------|
| `app_key`     | string | —                        | Dropbox app key (required)       |
| `app_secret`  | string | —                        | Dropbox app secret (required)    |
| `token_file`  | string | `"/data/dropbox.json"`   | Path to OAuth token JSON file    |
| `upload_path` | string | `"/Apps/Rakuten Kobo/"`  | Dropbox folder for uploads       |

### `paths` (optional)

| Field           | Type   | Default              | Description                    |
|-----------------|--------|----------------------|--------------------------------|
| `download_dir`  | string | `"/data/downloads"`  | Temporary download directory   |
| `converted_dir` | string | `"/data/converted"`  | Temporary conversion directory |

### `chats` (required, at least one)

Each chat entry supports:

| Field              | Type          | Required | Description                              |
|--------------------|---------------|----------|------------------------------------------|
| `handle`           | string        | yes      | Telegram handle to monitor (must start with @) |
| `accepted_formats` | []string      | no       | Override global accepted formats         |
| `storage`          | StorageConfig | no       | Override global storage settings         |

### Per-chat Storage Overrides

Chat-level storage config is merged on top of the global defaults. You only need to specify the fields you want to override:

```yaml
chats:
  - handle: "@another-bot"
    storage:
      dropbox:
        upload_path: "/Apps/Rakuten Kobo/Fiction/"
```

This inherits `app_key`, `app_secret`, and `token_file` from defaults, but uses a custom `upload_path`.

## CLI Flags

| Flag       | Default              | Description          |
|------------|----------------------|----------------------|
| `--config` | `/data/config.yaml`  | Path to config file  |

## Managing Chats

You can manage monitored chats without editing `config.yaml` by hand:

```bash
kpub chat list                  # show all monitored chats
kpub chat add                   # interactive prompt for a chat handle
kpub chat remove @ebook-bot     # remove a chat by handle
```

All `chat` subcommands accept `--data-dir` (default `data`) to locate `config.yaml`. Adding or removing a chat preserves all other config sections.

The server watches the config file for changes and automatically picks up new or removed chats without restarting.
