# Dropbox Setup

## 1. Create a Dropbox App

1. Go to [Dropbox App Console](https://www.dropbox.com/developers/apps)
2. Click **Create app**
3. Choose **Scoped access**
4. Choose **Full Dropbox** access (or **App folder** if you prefer)
5. Name your app (e.g., `kpub`)
6. Note your **App key** and **App secret**

## 2. Set Permissions

In your app's settings, go to the **Permissions** tab and enable:

- `files.content.write` — required to upload files
- `files.content.read` — optional, for verification

Click **Submit** to save.

## 3. Run the Setup Wizard

The setup wizard handles OAuth authorization automatically:

```bash
kpub setup
```

It will:
1. Ask for your App Key and App Secret
2. Open your browser to authorize the app
3. Exchange the code for access and refresh tokens
4. Save `data/dropbox.json` with the tokens

## 4. Configure

The wizard writes the Dropbox config to `config.yaml` for you. The relevant section looks like:

```yaml
defaults:
  storage:
    type: dropbox
    dropbox:
      app_key: "your-app-key"
      app_secret: "your-app-secret"
      token_file: "/data/dropbox.json"
      upload_path: "/Apps/Rakuten Kobo/"
```

## Notes

- The `upload_path` should match your Kobo's sync folder in Dropbox
- Tokens are automatically refreshed when they expire
- The `dropbox.json` file is updated in-place when tokens are refreshed
