# DayZ Stats Telegram Bot

A Telegram bot that monitors a DayZ server and keeps a single pinned message updated with live stats: player count,
player names, in-game time (with day/night indicator), ping, etc. When the server goes down, the message switches to
a downtime counter; when the bot shuts down gracefully, it marks the message as offline.

Player names are fetched via BattlEye RCon and are optional - the bot works without RCon, just without the name list.

## Requirements

- A Telegram **group with Topics (forum mode) enabled**
- Go 1.26+ to build from source

## Setup

### Create a Telegram bot

1. Open [@BotFather](https://t.me/BotFather) and send `/newbot`
2. Give it a name (e.g. `DayZ Server Status`) and a username (e.g. `dayzserver_status_bot`)
3. BotFather will reply with the bot's **auth token** - save it

### Add the bot to your group

1. Add the bot as a member of your group
2. Promote it to **admin** with the following permissions:
   - `Manage topics`
   - `Pin messages`
   - `Remain anonymous`

### Get your chat ID and topic (thread) ID

The easiest way - forward any message from the target topic to [@idbot](https://t.me/idbot). Simply post a message with
the text like `test` in the topic, forward it to `@idbot`, and it will reply with the chat ID and thread ID:

```text
🔎 TELEGRAM GROUP CHECK

👥 Group: <your group name>
🆔 -1001234567890 | 13 digits
```

The `-1001234567890` is the **chat ID**.

> [!NOTE]
> Chat IDs for supergroups are negative and start with `-100`.

To get the **thread ID**, click on the topic title, and you will see the "Invite link" in the format
`https://t.me/<your_group_name>/<thread_id>`. The `<thread_id>` is the topic ID.

> Alternatively, you may post any message in the topic and use
> `curl https://api.telegram.org/bot<BOT_TOKEN>/getUpdates | jq` to find all the relevant IDs in the JSON output.

### First run - let the bot create its message

On the very first launch, **do not** pass `--tg-message-id`. The bot will:

1. Post a new message in the specified topic
2. Pin it
3. Print the message ID to the log

```
./bot \
  --a2c-address "your.dayz.server:2302" \
  --tg-bot-token "123456789:ABCdef..." \
  --tg-chat-id -1001234567890 \
  --tg-thread-id 42
```

Look for the message ID in the log output:

```
msg=Sent initial message message_id=999
```

Stop the bot (`Ctrl+C`) and note the `message_id`.

> [!IMPORTANT]
> The bot can only edit messages it has sent itself. If you pass someone else's message ID, edits will fail.

### All subsequent runs

Pass `--tg-message-id` with the value from the previous step. From this point on the bot is safe to restart as many
times as needed - it will always update the same message.

```
./bot \
  --a2c-address "your.dayz.server:2302" \
  --tg-bot-token "123456789:ABCdef..." \
  --tg-chat-id -1001234567890 \
  --tg-thread-id 42 \
  --tg-message-id 999
```

To also show player names, add `--rcon-address` and `--rcon-password`:

```
./bot \
  --a2c-address "your.dayz.server:2302" \
  --rcon-address "your.dayz.server:2306" \
  --rcon-password "your-rcon-password" \
  --tg-bot-token "123456789:ABCdef..." \
  --tg-chat-id -1001234567890 \
  --tg-thread-id 42 \
  --tg-message-id 999
```

### Recovery

If the bot's message gets deleted from the topic, repeat steps above - launch without `--tg-message-id`, grab the new
message ID from the log, then restart with it.

## Configuration

All flags can also be set via environment variables where noted.

At least one of `--a2c-address` or `--rcon-address` must be provided.

| Flag                | Env variable    | Default | Description                                                                         |
|---------------------|-----------------|---------|-------------------------------------------------------------------------------------|
| `--a2c-address`     | `A2C_ADDRESS`   | -       | `host:port` of the DayZ server's A2C query interface (usually game port or `27016`) |
| `--rcon-address`    | `RCON_ADDRESS`  | -       | `host:port` of the DayZ server's BattlEye RCon (optional, enables player names)     |
| `--rcon-password`   | `RCON_PASSWORD` | -       | Password for the BattlEye RCon (required when `--rcon-address` is set)              |
| `--tg-bot-token`    | `TG_BOT_TOKEN`  | -       | Telegram bot auth token from @BotFather                                             |
| `--tg-chat-id`      | -               | -       | Telegram chat (group) ID                                                            |
| `--tg-thread-id`    | -               | `0`     | Topic (thread) ID inside the group                                                  |
| `--tg-message-id`   | -               | `0`     | Message ID to edit (omit on first run to create a new one)                          |
| `--update-interval` | -               | `10s`   | How often to poll the server. Must be greater than `1s`                             |

## Building

```bash
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o ./bot ./cmd/bot/
```

## Installing

### Running on a VPS/VDS with systemd

This guide covers deploying the bot as a systemd service on a Linux VPS. All commands are run as root.

```bash
# create a dedicated system user
useradd --system --home-dir /home/telegram-bot --create-home --shell /usr/sbin/nologin telegram-bot

# download and extract the latest release binary
curl -fsSL "https://github.com/alexzulu/dayz-stats-tg-bot/releases/latest/download/bot-linux-amd64.tar.gz" \
  | tar -xzf - -C /home/telegram-bot/

# set the correct owner and make it executable
chown telegram-bot:telegram-bot /home/telegram-bot/bot
chmod 755 /home/telegram-bot/bot

# create the log file
touch /var/log/bot.log
chmod 644 /var/log/bot.log

# create the systemd unit file
nano /etc/systemd/system/bot.service
```

```ini
[Unit]
Description=Telegram Bot
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=telegram-bot
Group=telegram-bot
WorkingDirectory=/home/telegram-bot
Environment="TG_BOT_TOKEN=1234567890:XXXXXXXXXXXXXXXXX"
Environment="RCON_PASSWORD=your-rcon-password"
ExecStart=/home/telegram-bot/bot --a2c-address 123.123.123.123:27016 --rcon-address 123.123.123.123:2306 --tg-thread-id 11 --tg-chat-id -1002222222222 --tg-message-id 33

Restart=on-failure
RestartSec=5

StandardOutput=append:/var/log/bot.log
StandardError=append:/var/log/bot.log

NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=read-only
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

```bash
# reload systemd to pick up the new unit, then enable and start the service
systemctl daemon-reload
systemctl enable --now bot.service
systemctl status bot.service

# check the logs to confirm the bot is running
tail -f /var/log/bot.log

# create a logrotate config for the bot logs
nano /etc/logrotate.d/bot
```

```text
/var/log/bot.log {
    weekly
    rotate 8
    compress
    delaycompress
    missingok
    notifempty
    copytruncate
}
```

```bash
logrotate -d /etc/logrotate.d/bot # dry-run to check the config is valid
logrotate -f /etc/logrotate.d/bot # force an immediate rotation
```

> [!NOTE]
> On some Ubuntu installations `/var/log` is group-writable, which causes logrotate to fail with
> `error: skipping "/var/log/bot.log" because parent directory has insecure permissions`. Fix: `chmod 755 /var/log`

### Updating the bot

```bash
# all the commands below are run as root

# download the new binary into /tmp, then swap it in
curl -fsSL "https://github.com/alexzulu/dayz-stats-tg-bot/releases/latest/download/bot-linux-amd64.tar.gz" \
  | tar -xzf - -C /tmp/
mv /tmp/bot /home/telegram-bot/bot-new

# stop the running service, replace the binary, and start it back
systemctl stop bot.service
mv /home/telegram-bot/bot-new /home/telegram-bot/bot
chown telegram-bot:telegram-bot /home/telegram-bot/bot
chmod 755 /home/telegram-bot/bot
systemctl start bot.service
systemctl status bot.service

# check the logs to confirm it came back up cleanly
tail -f /var/log/bot.log
```
