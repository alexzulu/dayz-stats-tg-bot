package app

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/alexzulu/dayz-stats-tg-bot/cmd/tg-bot/app/telegram"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/bot"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/config"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/statez"
)

// App is the main CLI application struct.
type App struct{ name string }

// NewApp creates a new instance of the App with the given name.
func NewApp(name string) *App { return &App{name: name} }

const (
	envCacheTTL       evnVar = "CACHE_TTL"
	envTgBotToken     evnVar = "TG_BOT_TOKEN" //nolint:gosec // it's just a name of an env :D
	envTgAdminIDs     evnVar = "TG_ADMIN_IDS"
	envConfigFilePath evnVar = "CONFIG_FILE"
)

// Run executes the application with the provided context and command-line arguments.
func (a *App) Run(ctx context.Context, args []string) error { //nolint:funlen // linear startup wiring
	var (
		flags = flag.NewFlagSet(a.name, flag.ExitOnError)
		log   = slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

		cacheTTL      time.Duration
		tgBotTkn      string
		tgAdminIDsStr string
		cfgPath       string
	)

	// attach flags to the flag set
	flags.DurationVar(
		&cacheTTL,
		"cache-ttl",
		envOrDur(envCacheTTL, 5*time.Second), //nolint:mnd
		"Time-to-live for cached server data [$"+envCacheTTL.String()+"]",
	)
	flags.StringVar(
		&tgBotTkn,
		"tg-bot-token",
		envOrStr(envTgBotToken, ""),
		"Telegram bot token [$"+envTgBotToken.String()+"]",
	)
	flags.StringVar(
		&tgAdminIDsStr,
		"tg-admin-ids",
		envOrStr(envTgAdminIDs, ""),
		"Comma-separated list of Telegram admin account IDs (e.g. '5551231234,5553213210') [$"+envTgAdminIDs.String()+"]",
	)

	flags.StringVar(
		&cfgPath,
		"config-file",
		envOrStr(envConfigFilePath, filepath.Join(config.DefaultDirPath(), "dayz-stats-tg-bot.json")),
		"Path to the configuration file (you DON'T need to create it manually, the app will create it and store "+
			"its settings there) [$"+envConfigFilePath.String()+"]",
	)

	// parse the command-line arguments
	if err := flags.Parse(args); err != nil {
		return err
	}

	// normalize cache-ttl (100ms <= cacheTTL <= 1h)
	cacheTTL = max(100*time.Millisecond, min(cacheTTL, time.Hour)) //nolint:mnd

	// parse the Telegram admin IDs
	tgAdminIDs := make([]int64, 0)

	for id := range strings.SplitSeq(tgAdminIDsStr, ",") {
		if s := strings.TrimSpace(id); s != "" {
			v, err := strconv.ParseInt(s, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid Telegram admin ID: %s", s)
			}

			tgAdminIDs = append(tgAdminIDs, v)
		}
	}

	if len(tgAdminIDs) == 0 {
		log.Warn("No Telegram admin IDs provided, make sure this isn't a mistake", slog.String("tg_admin_ids", tgAdminIDsStr))
	}

	// validate the provided Telegram bot token
	if len(tgBotTkn) != 46 { //nolint:mnd
		return fmt.Errorf("invalid Telegram bot token (length): %s", tgBotTkn)
	}

	log.Info("Loading configuration", slog.String("path", cfgPath))

	cfg := config.NewFile(cfgPath)
	if err := cfg.Load(); err != nil {
		return fmt.Errorf("loading config file: %w", err)
	}

	tgLog := log.With(slog.String("scope", "telegram-bot"))

	tgBot := bot.NewTelegram(tgBotTkn, tgLog)
	defer func() { _ = tgBot.Close() }()

	log.Info("Pinging Telegram API")

	if err := tgBot.Ping(); err != nil {
		return fmt.Errorf("pinging Telegram API: %w", err)
	}

	log.Info("Initialization complete",
		slog.String("cache_ttl", cacheTTL.String()),
		slog.Any("tg_admin_ids", tgAdminIDs),
		slog.String("config_file", cfgPath),
	)

	cp := statez.NewCachedClientPool(cacheTTL) // create clients pool to share StateZ clients

	if err := telegram.RegisterHandlers(ctx, tgLog, cfg, tgBot, cp, tgAdminIDs); err != nil {
		return fmt.Errorf("registering Telegram handlers: %w", err)
	}

	if err := tgBot.Start(); err != nil {
		return fmt.Errorf("starting Telegram bot: %w", err)
	}

	updaterClosed := make(chan struct{})

	// refresh bound live-status messages in the background until the context is canceled
	go func() {
		defer close(updaterClosed)

		telegram.NewStatusUpdater(log.With(slog.String("scope", "telegram-status")), tgBot, cfg, cp).Run(ctx)
	}()

	defer func(t time.Time) { log.Info("Exiting", slog.String("uptime", time.Since(t).String())) }(time.Now())

	<-ctx.Done()
	<-updaterClosed

	return nil
}
