# Telegram Setup

## 1. Create a Telegram Application

1. Go to [my.telegram.org](https://my.telegram.org)
2. Log in with your phone number
3. Click **API development tools**
4. Fill in the form (app title and short name can be anything)
5. Note your **App api_id** and **App api_hash**

These go into your `config.yaml`:

```yaml
telegram:
  app_id: 12345678
  app_hash: "your-app-hash-here"
```

## 2. Add Chats to Monitor

Add the handles of the Telegram chats you want to monitor for ebook files. These can be bots, groups, or channels:

```yaml
chats:
  - handle: "@ebook-bot"
  - handle: "@another-bot"
    accepted_formats: [".epub"]
```

## 3. First Run Authentication

On first run, the server will prompt you to authenticate as your Telegram user account:

1. Enter your phone number (e.g. `+1234567890`)
2. Enter the verification code sent to your Telegram app
3. If you have 2FA enabled, enter your password

The session is saved to `/data/session.json`. Subsequent runs skip authentication.

## Notes

- The `app_id` and `app_hash` identify your application to Telegram
- No bot tokens are needed — the server authenticates as your user account
- The server only monitors incoming messages in the configured chats
- Messages you send to a monitored chat are also processed
- Status notifications go to your Saved Messages
