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
	"sync"
	"time"

	"github.com/woozymasta/a2s/pkg/a2s"
	"github.com/woozymasta/bercon-cli/pkg/bercon"

	"github.com/alexzulu/dayz-stats-tg-bot/internal/retry"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/server"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/tgbot"
)

// App represents the application itself, encapsulating its configuration and state.
type App struct {
	// struct for holding command-line flags and their values
	flags struct {
		updInterval time.Duration
		a2cAddr     string
		rconAddr    string
		rconPwd     string
		tgBotTkn    string
		tgChatID    int64
		tgThreadID  int
		tgMessageID int

		set *flag.FlagSet
	}

	// struct for holding validated options derived from flags, will be available only after Run() is called
	opts struct {
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
}

// NewApp creates a new instance of the App.
func NewApp(name string) *App {
	app := App{}
	app.flags.set = flag.NewFlagSet(name, flag.ContinueOnError)

	app.flags.set.DurationVar(
		&app.flags.updInterval,
		"update-interval",
		10*time.Second, //nolint:mnd
		"Interval between updates",
	)

	app.flags.set.StringVar(
		&app.flags.a2cAddr,
		"a2c-address",
		os.Getenv("A2C_ADDRESS"),
		"<ip-or-domain>:<port> of the DayZ server's A2C interface (required) [$A2C_ADDRESS]",
	)

	app.flags.set.StringVar(
		&app.flags.rconAddr,
		"rcon-address",
		os.Getenv("RCON_ADDRESS"),
		"<ip-or-domain>:<port> of the DayZ server's BattlEye RCon [$RCON_ADDRESS]",
	)

	app.flags.set.StringVar(
		&app.flags.rconPwd,
		"rcon-password",
		os.Getenv("RCON_PASSWORD"),
		"Password for the DayZ server's BattlEye RCon [$RCON_PASSWORD]",
	)

	app.flags.set.StringVar(
		&app.flags.tgBotTkn,
		"tg-bot-token",
		os.Getenv("TG_BOT_TOKEN"),
		"Telegram bot token [$TG_BOT_TOKEN]",
	)

	app.flags.set.Int64Var(
		&app.flags.tgChatID,
		"tg-chat-id",
		-1000000000000,
		"Telegram chat ID to post/edit the message in",
	)

	app.flags.set.IntVar(
		&app.flags.tgThreadID,
		"tg-thread-id",
		0,
		"Telegram thread (topic) ID to post/edit the message in",
	)

	app.flags.set.IntVar(
		&app.flags.tgMessageID,
		"tg-message-id",
		0,
		"Telegram message ID to edit (if not specified, the bot will post a new message every time)",
	)

	return &app
}

// Run parses the command-line arguments, validates them, and starts the main application logic.
func (a *App) Run(ctx context.Context, args []string) error {
	if err := a.flags.set.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}

		return err
	}

	a.opts.TGChatID = a.flags.tgChatID
	a.opts.TGThreadID = a.flags.tgThreadID
	a.opts.TGMessageID = a.flags.tgMessageID

	// validate the update interval
	if a.flags.updInterval <= time.Second {
		return fmt.Errorf("update interval must be greater than 1 second: %s", a.flags.updInterval)
	}

	a.opts.UpdateInterval = a.flags.updInterval

	{ // validate the A2C address (required)
		if a.flags.a2cAddr == "" {
			return errors.New("A2C server address is required")
		}

		host, port, sErr := net.SplitHostPort(a.flags.a2cAddr)
		if sErr != nil {
			return fmt.Errorf("invalid A2C server address: %w", sErr)
		}

		if host == "" {
			return fmt.Errorf("A2C server host is empty: %s", a.flags.a2cAddr)
		}

		portNum, pErr := strconv.ParseUint(port, 10, 16)
		if pErr != nil || portNum <= 0 || portNum > math.MaxUint16 {
			return fmt.Errorf("invalid A2C server port: %s", port)
		}

		a.opts.A2CHost, a.opts.A2CPort = host, uint16(portNum)
	}

	// validate the RCon address if provided
	if a.flags.rconAddr != "" {
		if _, _, err := net.SplitHostPort(a.flags.rconAddr); err != nil {
			return fmt.Errorf("invalid RCon server address: %w", err)
		}

		a.opts.RCONAddr, a.opts.RCONPassword = a.flags.rconAddr, a.flags.rconPwd
	}

	// validate the Telegram bot token
	if len(a.flags.tgBotTkn) != 46 { //nolint:mnd
		return fmt.Errorf("invalid Telegram bot token length: %s", a.flags.tgBotTkn)
	}

	a.opts.TGBotToken = a.flags.tgBotTkn

	// create a structured logger
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	return a.run(ctx, log)
}

func (a *App) run(ctx context.Context, log *slog.Logger) error { //nolint:funlen,gocognit
	log.Debug("Starting DayZ Telegram stats bot")

	bot := tgbot.New(a.opts.TGBotToken)

	var (
		connWg       sync.WaitGroup
		tgErr, rcErr error
	)

	const initialAttempts, initialInterval = 60, time.Second

	// try connecting to the Telegram API
	connWg.Add(1)
	connWg.Go(func() {
		defer connWg.Done()

		if _, botConnErr := retry.Do(ctx,
			func() (struct{}, error) { return struct{}{}, bot.Connect() },
			retry.Attempts(initialAttempts),
			retry.Interval(initialInterval),
			retry.OnError(func(err error, attempt int) {
				log.Warn("Failed to connect to Telegram, retrying", slog.Any("error", err), slog.Int("attempt", attempt))
			}),
		); botConnErr != nil {
			tgErr = fmt.Errorf("failed to connect to Telegram: %w", botConnErr)

			return
		}

		log.Debug("Connected to Telegram", slog.Int64("chat_id", a.opts.TGChatID), slog.Int("thread_id", a.opts.TGThreadID))

		// if no message ID provided, send a new (initial) message to the chat/thread
		if a.opts.TGMessageID == 0 {
			msgID, err := bot.SendInitialMessage(a.opts.TGChatID, a.opts.TGThreadID)
			if err != nil {
				tgErr = fmt.Errorf("failed to send initial message to Telegram chat: %w", err)

				return
			}

			log.Info("The initial message is sent to Telegram chat", slog.Int("message_id", msgID))

			a.opts.TGMessageID = msgID // update the message ID for future usage
		}
	})

	var rc *bercon.Connection

	// if RCon address is provided, try connecting to the RCon server
	if a.opts.RCONAddr != "" {
		connWg.Add(1)
		connWg.Go(func() {
			defer connWg.Done()

			rc, rcErr = retry.Do(ctx,
				func() (*bercon.Connection, error) { return bercon.Open(a.opts.RCONAddr, a.opts.RCONPassword) },
				retry.Attempts(initialAttempts),
				retry.Interval(initialInterval),
				retry.OnError(func(err error, attempt int) {
					log.Warn("Failed to connect to RCon server, retrying", slog.Any("error", err), slog.Int("attempt", attempt))
				}),
			)
			if rcErr != nil {
				log.Warn("Failed to connect to RCon server", slog.Any("error", rcErr)) // RCon connection is optional
			}
		})
	}

	connWg.Wait()

	// telegram connection is critical, so we return the error immediately
	if tgErr != nil {
		return tgErr
	}

	// start handling Telegram messages/updates in a separate goroutine
	if err := bot.Start(ctx); err != nil {
		return err
	}

	defer func() {
		if rc != nil {
			if err := rc.Close(); err != nil {
				log.Error("Failed to close RCon connection", slog.Any("error", err))
			}
		}

		// on exit, update the message to indicate that the bot is offline
		if err := bot.SetMessageOffline(a.opts.TGChatID, a.opts.TGThreadID, a.opts.TGMessageID); err != nil {
			log.Error("Failed to update message to indicate bot offline", slog.Any("error", err))
		}
	}()

	var (
		startedAt        = time.Now()
		lastServerDownAt *time.Time
	)

	timer := time.NewTimer(time.Duration(0)) // trigger the first update immediately
	defer timer.Stop()

	log.Debug("Starting the update loop", slog.Duration("interval", a.opts.UpdateInterval))

	// start the update loop, which will run until the context is canceled
	for {
		select {
		case <-timer.C:
			func() { // use IIFE to ensure timer reset happens even if an error occurs
				defer timer.Reset(a.opts.UpdateInterval) // reset the timer for the next update

				var (
					wg    sync.WaitGroup
					info  server.Info
					acErr error // used to store both error types for A2C - connection and info retrieval
				)

				const attempts, interval = 15, 50 * time.Millisecond

				// request server info via A2C
				wg.Add(1)
				wg.Go(func() {
					defer wg.Done()

					var ac *a2s.Client

					ac, acErr = retry.Do(ctx,
						func() (*a2s.Client, error) { return a2s.New(a.opts.A2CHost, int(a.opts.A2CPort)) },
						retry.Attempts(attempts),
						retry.Interval(interval),
						retry.OnError(func(err error, attempt int) {
							log.Warn("Failed to connect to A2S server, retrying",
								slog.Any("error", err),
								slog.Int("attempt", attempt),
							)
						}),
					)
					if acErr != nil {
						log.Error("Failed to connect to A2S server", slog.Any("error", acErr))

						return
					}

					if _, acErr = retry.Do(ctx,
						func() (struct{}, error) { return struct{}{}, info.GetFromA2S(ac) },
						retry.Attempts(attempts),
						retry.Interval(interval),
						retry.OnError(func(err error, attempt int) {
							log.Warn("Failed to get server info via A2C, retrying",
								slog.Any("error", err),
								slog.Int("attempt", attempt),
							)
						}),
					); acErr != nil {
						log.Error("Failed to get server info via A2C", slog.Any("error", acErr))
					}
				})

				// if RCon connection is available, request server info via RCon as well
				if rc != nil {
					wg.Add(1)
					wg.Go(func() {
						defer wg.Done()

						if _, err := retry.Do(ctx,
							func() (struct{}, error) { return struct{}{}, info.GetFromRCon(rc) },
							retry.Attempts(attempts),
							retry.Interval(interval),
							retry.OnError(func(err error, attempt int) {
								log.Warn("Failed to get server info via RCon, retrying",
									slog.Any("error", err),
									slog.Int("attempt", attempt),
								)
							}),
						); err != nil {
							log.Error("Failed to get server info via RCon", slog.Any("error", err))
						}
					})
				}

				wg.Wait() // block until all info retrieval attempts are complete

				// A2C error is critical, so we handle it by updating the message to indicate the server is down
				if acErr != nil {
					if lastServerDownAt == nil {
						lastServerDownAt = new(time.Now())
					}

					if err := bot.SetMessagesServerDown(
						a.opts.TGChatID,
						a.opts.TGThreadID,
						a.opts.TGMessageID,
						time.Since(*lastServerDownAt),
					); err != nil {
						log.Error("Failed to update message to indicate server down", slog.Any("error", err))
					}

					return
				}

				lastServerDownAt = nil // reset the server down timestamp since we successfully got the info

				if err := bot.SetMessagesServerInfo(a.opts.TGChatID, a.opts.TGThreadID, a.opts.TGMessageID, &info); err != nil {
					log.Error("Failed to update message", slog.Any("error", err))

					return
				}

				log.Debug("Updated message with server info",
					slog.Int("players_online", int(info.PlayersCount)),
					slog.Duration("ping", info.Ping),
				)
			}()

		case <-ctx.Done():
			log.Info("Context canceled, exiting update loop",
				slog.Duration("uptime", time.Since(startedAt).Round(time.Millisecond)),
			)

			return nil // exit gracefully on context cancellation
		}
	}
}
