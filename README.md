# DayZ Stats Telegram Bot

A Telegram bot that monitors a DayZ server and keeps a single pinned message updated with live stats: player count,
in-game time (with day/night indicator), ping, etc. When the server goes down, the message switches to a downtime
counter; when the bot shuts down gracefully, it marks the message as offline.

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

The easiest way - forward any message from the target topic to [@userinfobot](https://t.me/userinfobot) or use the
Telegram Web - the URL contains both IDs in the form `#-100XXXXXXXXXX/THREAD_ID`.

> [!NOTE]
> Chat IDs for supergroups are negative and start with `-100`.

Also, you may use `curl https://api.telegram.org/bot<BOT_TOKEN>/getUpdates | jq` to get the chat ID and thread ID from
the JSON output.

### First run - let the bot create its message

On the very first launch, **do not** pass `--tg-message-id`. The bot will:

1. Post a new message in the specified topic
2. Pin it
3. Print the message ID to the log

```
./bot \
  --server-address "your.dayz.server:2302" \
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
  --server-address "your.dayz.server:2302" \
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

| Flag                | Env variable     | Default | Description                                                                       |
|---------------------|------------------|---------|-----------------------------------------------------------------------------------|
| `--server-address`  | `SERVER_ADDRESS` | -       | `host:port` of the DayZ server (A2S query port, usually game port + 0 or `27016`) |
| `--tg-bot-token`    | `TG_BOT_TOKEN`   | -       | Telegram bot auth token from @BotFather                                           |
| `--tg-chat-id`      | -                | -       | Telegram chat (group) ID                                                          |
| `--tg-thread-id`    | -                | `0`     | Topic (thread) ID inside the group                                                |
| `--tg-message-id`   | -                | `0`     | Message ID to edit (omit on first run to create a new one)                        |
| `--update-interval` | -                | `10s`   | How often to poll the server. Must be greater than `1s`                           |

## Building

```bash
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o ./bot ./cmd/bot/
```
