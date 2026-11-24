# DayZ Stats Telegram Bot

A Telegram bot for DayZ that does two things - it keeps an auto-updating message in sync with the live state of the
game world (so a chat or topic always shows what's happening right now), and it gives admins a set of commands to
pull the full picture on demand (players, vehicles and more, right down to a visual rendering on the in-game map).

All the data comes from a [**StateZ** HTTP API server](https://github.com/tarampampam/dayz-statez) running alongside
the game server.

## Features

- **On-demand stats** - players, vehicles, etc.
- **Map rendering** - a player/players positions on the in-game map (adding a new map is a drop-in).
- **Live status message** - a self-updating message pinned in a group/forum topic, kept in sync with the game world.
- **Multiserver** - manage several StateZ servers at runtime from the bot.
- **In-memory caching** - StateZ API responses are cached with a short TTL, so bursts of commands don't hammer the daemon.
- **Admin-only** - every command is gated by a Telegram user-ID allowlist.

## Getting started

Open [@BotFather](https://t.me/BotFather), send `/newbot`, pick a name and username. BotFather replies with the bot's
**auth token** - save it.

Start the bot with only the token (no admin IDs yet):

```bash
./bot --tg-bot-token "123456789:ABCdef..."
```

Open a private chat with the bot and send it `/ping` - it replies with your numeric Telegram  user ID (alternatively,
forward any message to [@idbot](https://t.me/idbot)).

Stop the bot and start it again, passing your ID (several IDs are comma-separated):

```bash
./bot \
  --tg-bot-token "123456789:ABCdef..." \
  --tg-admin-ids "5551231234,5553213213"
```

From now on the bot accepts commands only from the listed users, in private chats.

Everything else is done from inside the bot - add your StateZ servers (name + URL + token), list online players,
inspect a player, view vehicles, and so on. Added servers are saved to the config file and survive restarts.

To keep a self-updating status message in a group (or forum topic):

1. Create a Telegram **group** (optionally enable **Topics** if you want the message in a specific topic).
2. Add the bot to the group and promote it to **admin** with the **Pin Messages** permission.
3. In the target chat/topic, trigger the bot's via `/bind@<your_bot_name>` command and pick a server.

## Configuration

Every flag has an environment-variable fallback.

| Flag             | Env variable   | Default                              | Description                                                      |
|------------------|----------------|--------------------------------------|------------------------------------------------------------------|
| `--tg-bot-token` | `TG_BOT_TOKEN` | -                                    | Telegram bot auth token from `@BotFather` (**required**)         |
| `--tg-admin-ids` | `TG_ADMIN_IDS` | -                                    | Comma-separated Telegram user IDs allowed to use the bot         |
| `--cache-ttl`    | `CACHE_TTL`    | `5s`                                 | TTL for cached StateZ data (clamped to `100ms`..`1h`)            |
| `--config-file`  | `CONFIG_FILE`  | `<os-config-dir>/dayz-stats-tg-bot.json` | Path to the JSON config (created automatically, no need to edit) |

> [!NOTE]
> An empty admin list locks everyone out - the bot starts but ignores all commands.

## Installing

### Running on a VPS/VDS with systemd

Deploy the bot as a systemd service on a Linux server (make sure it has access to the Telegram API first!).
All commands are run as root.

```bash
# create a dedicated system user
useradd --system --home-dir /home/dayz-stats-tg-bot --create-home --shell /usr/sbin/nologin dayz-stats-tg-bot

# download and extract the latest release binary
curl -fsSL "https://github.com/alexzulu/dayz-stats-tg-bot/releases/latest/download/tg-bot-linux-amd64.tar.gz" \
  | tar -xzf - -C /home/dayz-stats-tg-bot/

# set the correct owner and make it executable
chown dayz-stats-tg-bot:dayz-stats-tg-bot /home/dayz-stats-tg-bot/tg-bot
chmod 755 /home/dayz-stats-tg-bot/tg-bot

# create the systemd unit file
nano /etc/systemd/system/tg-bot.service
```

```ini
[Unit]
Description=DayZ Stats Telegram Bot
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=dayz-stats-tg-bot
Group=dayz-stats-tg-bot
WorkingDirectory=/home/dayz-stats-tg-bot
Environment="TG_BOT_TOKEN=1234567890:XXXXXXXXXXXXXXXXX"
Environment="TG_ADMIN_IDS=5551231234"
ExecStart=/home/dayz-stats-tg-bot/tg-bot

Restart=on-failure
RestartSec=5

NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=read-only
ReadWritePaths=/home/dayz-stats-tg-bot
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

```bash
# reload systemd to pick up the new unit, then enable and start the service
systemctl daemon-reload
systemctl enable --now tg-bot.service
systemctl status tg-bot.service
```

### Updating the bot

```bash
# all the commands below are run as root

# download the new binary into /tmp, then swap it in
curl -fsSL "https://github.com/alexzulu/dayz-stats-tg-bot/releases/latest/download/tg-bot-linux-amd64.tar.gz" \
  | tar -xzf - -C /tmp/
mv /tmp/tg-bot /home/dayz-stats-tg-bot/tg-bot-new

# stop the running service, replace the binary, and start it back
systemctl stop tg-bot.service
mv /home/dayz-stats-tg-bot/tg-bot-new /home/dayz-stats-tg-bot/tg-bot
chown dayz-stats-tg-bot:dayz-stats-tg-bot /home/dayz-stats-tg-bot/tg-bot
chmod 755 /home/dayz-stats-tg-bot/bot
systemctl start tg-bot.service
systemctl status tg-bot.service
```

## Maps

Game maps are embedded into the binary. Each map lives under `internal/gamemap/maps/<name>/` and is resolved by the
StateZ world name (`chernarusplus`, `nasdara`). Tiles and the generated `info.go` are produced by:

```bash
go generate ./...
```

Generation is sticky - existing tiles are never re-downloaded and an existing `info.go` is never regenerated. To add a
new map, create its package and run the generator; to refresh an existing one, delete its `tiles/` dir or `info.go` first.
