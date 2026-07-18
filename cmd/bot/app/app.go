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
	"sync"
	"time"

	"github.com/woozymasta/a2s/pkg/a2s"
	"github.com/woozymasta/bercon-cli/pkg/bercon"
	tele "gopkg.in/telebot.v4"

	"github.com/alexzulu/dayz-stats-tg-bot/internal/dayz_server"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/retry"
)

// App represents the application itself, encapsulating its configuration and state.
type App struct {
	flags struct {
		updInterval time.Duration
		a2cAddr     string
		rconAddr    string
		rconPwd     string
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
		&app.flags.a2cAddr,
		"a2c-address",
		os.Getenv("A2C_ADDRESS"),
		"<ip-or-domain>:<port> of the DayZ server's A2C interface [$A2C_ADDRESS]",
	)

	app.flagSet.StringVar(
		&app.flags.rconAddr,
		"rcon-address",
		os.Getenv("RCON_ADDRESS"),
		"<ip-or-domain>:<port> of the DayZ server's BattlEye RCon [$RCON_ADDRESS]",
	)

	app.flagSet.StringVar(
		&app.flags.rconPwd,
		"rcon-password",
		os.Getenv("RCON_PASSWORD"),
		"Password for the DayZ server's BattlEye RCon [$RCON_PASSWORD]",
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

	opt := opts{
		TGChatID:    a.flags.tgChatID,
		TGThreadID:  a.flags.tgThreadID,
		TGMessageID: a.flags.tgMessageID,
	}

	// validate the update interval
	if a.flags.updInterval <= time.Second {
		return fmt.Errorf("update interval must be greater than 1 second: %s", a.flags.updInterval)
	}

	opt.UpdateInterval = a.flags.updInterval

	// ensure that at least one of the A2C or RCon addresses is provided
	if a.flags.a2cAddr == "" && a.flags.rconAddr == "" {
		return errors.New("at least one of A2C or RCon address must be provided")
	}

	// validate the A2C address if provided
	if a.flags.a2cAddr != "" {
		host, port, sErr := net.SplitHostPort(a.flags.a2cAddr)
		if sErr != nil {
			return fmt.Errorf("invalid A2C server address: %w", sErr)
		}

		if host == "" {
			return fmt.Errorf("server host is empty: %s", a.flags.a2cAddr)
		}

		portNum, pErr := strconv.ParseUint(port, 10, 16)
		if pErr != nil || portNum <= 0 || portNum > math.MaxUint16 {
			return fmt.Errorf("invalid server port: %s", port)
		}

		opt.A2CHost, opt.A2CPort = host, uint16(portNum)
	}

	// validate the RCon address if provided
	if a.flags.rconAddr != "" {
		if _, _, err := net.SplitHostPort(a.flags.rconAddr); err != nil {
			return fmt.Errorf("invalid RCon server address: %w", err)
		}

		opt.RCONAddr, opt.RCONPassword = a.flags.rconAddr, a.flags.rconPwd
	}

	// validate the Telegram bot token
	if len(a.flags.tgBotTkn) != 46 { //nolint:mnd
		return fmt.Errorf("invalid Telegram bot token length: %s", a.flags.tgBotTkn)
	}

	opt.TGBotToken = a.flags.tgBotTkn

	// create a structured logger
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	return a.run(ctx, opt, log)
}

type opts struct {
	A2CHost        string
	A2CPort        uint16
	UpdateInterval time.Duration
	RCONAddr       string
	RCONPassword   string
	TGBotToken     string
	TGChatID       int64
	TGThreadID     int
	TGMessageID    int
}

func (a *App) run(ctx context.Context, opt opts, log *slog.Logger) error { //nolint:funlen
	log.Debug("Starting DayZ Telegram stats bot")

	// create a new Telegram bot instance (retry is required because sometimes Telegram API is unavailable)
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
	bot.Handle(tele.OnText, func(c tele.Context) error {
		if c.Chat().Type != tele.ChatPrivate {
			return nil
		}

		return c.Reply("I'm alive!")
	})

	// start the bot in a separate goroutine, and stop it when the context is canceled (this required for handling
	// incoming messages)
	go func() {
		go func() { <-ctx.Done(); bot.Stop() }() // stop the bot when the context is canceled

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

	// on exit, update the message to indicate that the bot is offline
	defer func() {
		if _, err := bot.Edit(
			&tele.Message{ID: opt.TGMessageID, Chat: &tele.Chat{ID: opt.TGChatID}},
			"||🛑 Bot offline||",
			&tele.SendOptions{ThreadID: opt.TGThreadID, ParseMode: tele.ModeMarkdownV2},
		); err != nil {
			log.Error("Failed to update message to indicate bot offline", slog.Any("error", err))
		}
	}()

	// create both the A2S and RCon clients concurrently
	ac, rc, cErr := a.createClients(ctx, opt, log)
	defer func() { // ensure clients are closed after use
		if ac != nil {
			_ = ac.Close()
		}

		if rc != nil {
			_ = rc.Close()
		}
	}()
	if ac == nil && rc == nil { //nolint:wsl_v5
		return errors.New("failed to create both A2S and RCon clients")
	}
	if cErr != nil { //nolint:wsl_v5
		log.Warn("Failed to create some clients", slog.Any("error", cErr))
	}

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

				// request the server info
				info, infoErr := retry.Do(
					ctx,
					func() (*dayz_server.ServerInfo, error) { return dayz_server.GetServerInfo(ac, rc) },
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

// createClients concurrently creates and connects both the A2S and RCon clients based on the provided options.
// It MAY return non-nil error together with one of the clients if the other client failed to connect.
//
// The caller is responsible for closing the returned clients.
func (a *App) createClients(ctx context.Context, opt opts, log *slog.Logger) (*a2s.Client, *bercon.Connection, error) {
	var (
		wg sync.WaitGroup

		ac    *a2s.Client
		acErr error

		rc    *bercon.Connection
		rcErr error
	)

	if opt.A2CHost != "" && opt.A2CPort != 0 {
		wg.Go(func() {
			ac, acErr = retry.Do(
				ctx,
				func() (*a2s.Client, error) { return a2s.New(opt.A2CHost, int(opt.A2CPort)) },
				retry.Attempts(120),                  //nolint:mnd
				retry.Interval(500*time.Millisecond), //nolint:mnd
				retry.OnError(func(err error, attempt int) {
					log.Warn("Failed to connect to A2S server, retrying",
						slog.Any("error", err),
						slog.Int("attempt", attempt),
					)
				}),
			)
		})
	}

	if opt.RCONAddr != "" {
		wg.Go(func() {
			rc, rcErr = retry.Do(
				ctx,
				func() (*bercon.Connection, error) { return bercon.Open(opt.RCONAddr, opt.RCONPassword) },
				retry.Attempts(120),                  //nolint:mnd
				retry.Interval(500*time.Millisecond), //nolint:mnd
				retry.OnError(func(err error, attempt int) {
					log.Warn("Failed to connect to RCon server, retrying",
						slog.Any("error", err),
						slog.Int("attempt", attempt),
					)
				}),
			)
		})
	}

	wg.Wait()

	return ac, rc, errors.Join(acErr, rcErr)
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

	if len(info.PlayerNames) > 0 {
		text.WriteRune('\n')
		text.WriteString("👤 *Хомячат*: ")

		for i, name := range info.PlayerNames {
			if i > 0 {
				text.WriteString(", ")
			}

			text.WriteString(escapeForMarkdownV2(name))
		}
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

// escapeForMarkdownV2 escapes all MarkdownV2 special characters in s so the resulting string is safe to send as
// plain text with telebot.ModeMarkdownV2.
// It does NOT preserve any Markdown formatting that might already be in s - use this only for raw/untrusted text,
// not for strings you've hand-crafted with *bold*, _italic_, etc.
func escapeForMarkdownV2(s string) string {
	// order doesn't matter here since we do a single pass, but backslash must be in the set (it's the escape char itself)
	const specialChars = "_*[]()~`>#+-=|{}.!\\"

	var b strings.Builder
	b.Grow(len(s) + len(s)/4) // small headroom for escapes

	for _, r := range s {
		if strings.ContainsRune(specialChars, r) {
			b.WriteByte('\\')
		}

		b.WriteRune(r)
	}

	return b.String()
}
