package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"math"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/woozymasta/a2s/pkg/a2s"
	tele "gopkg.in/telebot.v4"

	"github.com/alexzulu/dayz-stats-tg-bot/internal/dayz_server"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/retry"
)

// App represents the application itself, encapsulating its configuration and state.
type App struct {
	flags struct {
		updInterval time.Duration
		srvAddr     string
		tgBotTkn    string
		tgChatID    int64
		tgThreadID  int
		tgMessageID int
	}
	flagSet *flag.FlagSet
}

// NewApp creates a new instance of the App.
func NewApp(name string) *App {
	app := App{
		flagSet: flag.NewFlagSet(name, flag.ContinueOnError),
	}

	app.flagSet.DurationVar(
		&app.flags.updInterval,
		"update-interval",
		10*time.Second, //nolint:mnd
		"Interval between updates",
	)

	app.flagSet.StringVar(
		&app.flags.srvAddr,
		"server-address",
		os.Getenv("SERVER_ADDRESS"),
		"<ip-or-domain>:<port> of the DayZ server to query [$SERVER_ADDRESS]",
	)

	app.flagSet.StringVar(
		&app.flags.tgBotTkn,
		"tg-bot-token",
		os.Getenv("TG_BOT_TOKEN"),
		"Telegram bot token [$TG_BOT_TOKEN]",
	)

	app.flagSet.Int64Var(
		&app.flags.tgChatID,
		"tg-chat-id",
		-1000000000000,
		"Telegram chat ID to post/edit the message in",
	)

	app.flagSet.IntVar(
		&app.flags.tgThreadID,
		"tg-thread-id",
		0,
		"Telegram thread (topic) ID to post/edit the message in",
	)

	app.flagSet.IntVar(
		&app.flags.tgMessageID,
		"tg-message-id",
		0,
		"Telegram message ID to edit (if not specified, the bot will post a new message every time)",
	)

	return &app
}

// Run parses the command-line arguments, validates them, and starts the main application logic.
func (a *App) Run(ctx context.Context, args []string) error {
	if err := a.flagSet.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}

		return err
	}

	if a.flags.updInterval <= time.Second {
		return fmt.Errorf("update interval must be greater than 1 second: %s", a.flags.updInterval)
	}

	host, port, srvAddrErr := net.SplitHostPort(a.flags.srvAddr)
	if srvAddrErr != nil {
		return fmt.Errorf("invalid server address: %w", srvAddrErr)
	}

	if host == "" {
		return fmt.Errorf("server host is empty: %s", a.flags.srvAddr)
	}

	portNum, portParseErr := strconv.ParseUint(port, 10, 16)
	if portParseErr != nil || portNum <= 0 || portNum > math.MaxUint16 {
		return fmt.Errorf("invalid server port: %s", port)
	}

	if len(a.flags.tgBotTkn) != 46 { //nolint:mnd
		return fmt.Errorf("invalid Telegram bot token length: %s", a.flags.tgBotTkn)
	}

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	log.Debug("Starting DayZ Telegram stats bot")

	return a.run(ctx, opts{
		ServerHost:     host,
		ServerPort:     uint16(portNum),
		UpdateInterval: a.flags.updInterval,
		TGBotToken:     a.flags.tgBotTkn,
		TGChatID:       a.flags.tgChatID,
		TGThreadID:     a.flags.tgThreadID,
		TGMessageID:    a.flags.tgMessageID,
	}, log)
}

type opts struct {
	ServerHost     string
	ServerPort     uint16
	UpdateInterval time.Duration
	TGBotToken     string
	TGChatID       int64
	TGThreadID     int
	TGMessageID    int
}

func (a *App) run(ctx context.Context, opt opts, log *slog.Logger) error { //nolint:funlen
	// create a new Telegram bot instance
	bot, botErr := retry.Do(
		ctx,
		func() (*tele.Bot, error) {
			return tele.NewBot(tele.Settings{
				Token:  opt.TGBotToken,
				Poller: &tele.LongPoller{Timeout: 10 * time.Second}, //nolint:mnd
			})
		},
		retry.Attempts(60), //nolint:mnd
		retry.Interval(time.Second),
		retry.OnError(func(err error, attempt int) {
			log.Warn("Failed to connect to Telegram, retrying",
				slog.Any("error", err),
				slog.Int("attempt", attempt),
			)
		}),
	)
	if botErr != nil {
		return botErr
	}
	defer func() { _, _ = bot.Close() }()

	// reply to any text message with a simple "hello" to confirm the bot is alive
	bot.Handle(tele.OnText, func(c tele.Context) error { return c.Reply("I'm alive!") })

	// start the bot in a separate goroutine, and stop it when the context is canceled
	go func() {
		go func() { <-ctx.Done(); bot.Stop() }()

		bot.Start()
	}()

	log.Debug("Connected to Telegram",
		slog.Int64("chat_id", opt.TGChatID),
		slog.Int("thread_id", opt.TGThreadID),
	)

	// if no message ID provided, send a new (initial) message to the chat/thread
	if opt.TGMessageID == 0 {
		msgID, initErr := a.sendInitMessage(bot, opt)
		if initErr != nil {
			return fmt.Errorf("sending initial message: %w", initErr)
		}

		log.Info("Sent initial message", slog.Int("message_id", msgID))

		opt.TGMessageID = msgID // update the message ID for future edits
	}

	defer func() {
		if _, err := bot.Edit(
			&tele.Message{ID: opt.TGMessageID, Chat: &tele.Chat{ID: opt.TGChatID}},
			"||🛑 Bot offline||",
			&tele.SendOptions{ThreadID: opt.TGThreadID, ParseMode: tele.ModeMarkdownV2},
		); err != nil {
			log.Error("Failed to update message to indicate bot offline", slog.Any("error", err))
		}
	}()

	var (
		startedAt        = time.Now()
		lastServerDownAt *time.Time
	)

	timer := time.NewTimer(100 * time.Millisecond) //nolint:mnd // use a short delay to trigger the first update
	defer timer.Stop()

	log.Debug("Starting the update loop", slog.Duration("interval", opt.UpdateInterval))

	// start the update loop, which will run until the context is canceled
	for {
		select {
		case <-ctx.Done():
			log.Info("Context canceled, exiting update loop",
				slog.Duration("uptime", time.Since(startedAt).Round(time.Millisecond)),
			)

			return nil // exit gracefully on context cancellation

		case <-timer.C:
			func() { // use IIFE to ensure timer reset happens even if an error occurs
				defer timer.Reset(opt.UpdateInterval) // reset the timer for the next update

				info, infoErr := retry.Do(
					ctx,
					func() (*dayz_server.ServerInfo, error) {
						c, cErr := a2s.New(opt.ServerHost, int(opt.ServerPort))
						if cErr != nil {
							return nil, fmt.Errorf("creating A2S client: %w", cErr)
						}
						defer func() { _ = c.Close() }()

						log.Debug("Connected to DayZ server, requesting server info",
							slog.String("host", opt.ServerHost),
							slog.Uint64("port", uint64(opt.ServerPort)),
						)

						return dayz_server.GetServerInfo(c)
					},
					retry.Attempts(5), //nolint:mnd
					retry.OnError(func(err error, attempt int) {
						log.Warn("Failed to get server info, retrying",
							slog.Any("error", err),
							slog.Int("attempt", attempt),
						)
					}),
				)
				if infoErr != nil {
					log.Error("Failed to get server info", slog.Any("error", infoErr))

					if lastServerDownAt == nil {
						lastServerDownAt = new(time.Now())
					}

					if err := a.updateMessageServerDown(bot, opt, time.Since(*lastServerDownAt)); err != nil {
						log.Error("Failed to update message to indicate server down", slog.Any("error", err))
					}

					return
				}

				lastServerDownAt = nil // reset the server down timestamp since we successfully got the info

				if err := a.updateMessageInfo(bot, opt, *info); err != nil {
					log.Error("Failed to update message", "error", err)

					return
				}

				log.Debug("Updated message with server info",
					slog.Int("players_online", int(info.PlayersCount)),
					slog.Duration("ping", info.Ping),
				)
			}()
		}
	}
}

// sendInitMessage sends an initial message to the specified Telegram chat and thread, indicating that the bot is
// initializing. It returns the message ID of the sent message or an error if the operation fails.
func (*App) sendInitMessage(bot *tele.Bot, opt opts) (int, error) {
	msg, msgErr := bot.Send(
		&tele.Chat{ID: opt.TGChatID},
		"🟢 Initializing...",
		&tele.SendOptions{
			ThreadID:            opt.TGThreadID,
			ParseMode:           tele.ModeMarkdown,
			DisableNotification: true,
			Protected:           true,
		},
	)
	if msgErr != nil {
		return 0, fmt.Errorf("sending initial message: %w", msgErr)
	}

	if _, err := bot.Edit(msg,
		fmt.Sprintf("✅ Initialization complete. Message ID: %d", msg.ID),
		&tele.SendOptions{ThreadID: opt.TGThreadID},
	); err != nil {
		return 0, fmt.Errorf("editing initial message: %w", err)
	}

	if err := bot.Pin(msg, tele.Silent); err != nil {
		return 0, fmt.Errorf("pinning initial message: %w", err)
	}

	return msg.ID, nil
}

// updateMessageInfo updates the specified Telegram message with the latest server information.
func (*App) updateMessageInfo(bot *tele.Bot, opt opts, info dayz_server.ServerInfo) error {
	var text strings.Builder
	text.Grow(128) //nolint:mnd // preallocate some space for the message

	text.WriteString("🎮 *Игроков* \\(online\\): ")
	text.WriteRune('*')
	text.WriteString(strconv.Itoa(int(info.PlayersCount)))
	text.WriteString("*/")
	text.WriteString(strconv.Itoa(int(info.MaxPlayersCount)))
	text.WriteRune('\n')

	if info.ServerTime[0] != 0 || info.ServerTime[1] != 0 {
		const sunRiseHour, sunSetHour = 5, 20

		if info.ServerTime[0] >= sunRiseHour && info.ServerTime[0] < sunSetHour {
			text.WriteString("☀️ ")
		} else {
			text.WriteString("🌙 ")
		}

		text.WriteString("*Время*: ")
		_, _ = fmt.Fprintf(&text, "*%02d:%02d*", info.ServerTime[0], info.ServerTime[1])
		text.WriteString(" \\(местное\\)\n")
	}

	pingMicro := info.Ping.Microseconds()

	text.WriteString("🏁 *Пинг*: ")                                        // e.g. `100.32 ms`
	_, _ = fmt.Fprintf(&text, "*%d*\\.%d", pingMicro/1000, pingMicro%100) //nolint:mnd
	text.WriteString(" мс")

	if _, err := bot.Edit(
		&tele.Message{ID: opt.TGMessageID, Chat: &tele.Chat{ID: opt.TGChatID}},
		text.String(),
		&tele.SendOptions{ThreadID: opt.TGThreadID, ParseMode: tele.ModeMarkdownV2},
	); err != nil {
		return fmt.Errorf("editing message with ID %d: %w", opt.TGMessageID, err)
	}

	return nil
}

// updateMessageServerDown updates the specified Telegram message to indicate that the server is down or unreachable.
func (*App) updateMessageServerDown(bot *tele.Bot, opt opts, downtime time.Duration) error {
	var text strings.Builder
	text.Grow(128) //nolint:mnd // preallocate some space for the message

	text.WriteString("❌ *Сервер недоступен*")

	if downtime.Seconds() > 1 {
		total := int(downtime.Round(time.Second).Seconds())

		text.WriteString(" _\\(лежит уже ")
		_, _ = fmt.Fprintf(&text, "%02d:%02d:%02d", total/3600, (total%3600)/60, total%60) //nolint:mnd
		text.WriteString("\\)_")
	}

	if _, err := bot.Edit(
		&tele.Message{ID: opt.TGMessageID, Chat: &tele.Chat{ID: opt.TGChatID}},
		text.String(),
		&tele.SendOptions{ThreadID: opt.TGThreadID, ParseMode: tele.ModeMarkdownV2},
	); err != nil {
		return fmt.Errorf("editing message with ID %d: %w", opt.TGMessageID, err)
	}

	return nil
}
