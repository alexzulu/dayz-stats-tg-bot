package handlers

import (
	"context"
	"fmt"
	"log/slog"

	tele "gopkg.in/telebot.v4"
	mw "gopkg.in/telebot.v4/middleware"

	"github.com/alexzulu/dayz-stats-tg-bot/internal/bot"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/config"
)

// TelegramHandlers holds the bot's command handlers and the config/admin list they operate on.
type TelegramHandlers struct {
	tgAdminIDs []int64
	cfg        *config.File
}

// NewTelegramHandlers returns handlers whose admin-only commands are restricted to the given Telegram user IDs.
func NewTelegramHandlers(tgAdminIDs []int64, cfg *config.File) *TelegramHandlers {
	return &TelegramHandlers{
		tgAdminIDs: tgAdminIDs,
		cfg:        cfg,
	}
}

// Register wires the private commands into b, guarded by panic recovery and request logging applied to every
// command. Admin-only commands additionally carry a whitelist middleware restricting them to h.tgAdminIDs.
func (h *TelegramHandlers) Register(ctx context.Context, log *slog.Logger, b *bot.Telegram) error {
	// restricts a command to the configured admin IDs; attached per-command via Command.Middleware
	auth := mw.Whitelist(h.tgAdminIDs...)

	if err := b.RegisterPrivateCommands(
		[]bot.Command{
			{
				Text:        "ping",
				Description: "Check if the bot is alive",
				Handler:     h.handlePing(),
			},
			{
				Text:        "add_server",
				Description: "Add a new server DayZ server with StateZ daemon installed",
				Handler:     h.handleAddServer(),
				Middleware:  []tele.MiddlewareFunc{auth},
			},
		},
		mw.Recover(func(err error, _ tele.Context) {
			log.Error("telegram handler panic recovered", slog.String("error", err.Error()))
		}),
		tgLoggerMiddleware(ctx, log),
	); err != nil {
		return err
	}

	return nil
}

func (h *TelegramHandlers) handlePing() tele.HandlerFunc {
	return func(c tele.Context) error {
		if user := c.Sender(); user != nil {
			return c.Send(
				fmt.Sprintf("pong, mr\\. with id `%d`, I'm alive", c.Sender().ID),
				&tele.SendOptions{ParseMode: tele.ModeMarkdownV2},
			)
		}

		return c.Send("pong")
	}
}

func (h *TelegramHandlers) handleAddServer() tele.HandlerFunc {
	return func(c tele.Context) error {
		return c.Send("Add server command received")
	}
}

// tgLoggerMiddleware returns a middleware that logs incoming messages with structured logging.
func tgLoggerMiddleware(ctx context.Context, log *slog.Logger) tele.MiddlewareFunc {
	return func(next tele.HandlerFunc) tele.HandlerFunc {
		return func(c tele.Context) error {
			var fields []slog.Attr

			if sender := c.Sender(); sender != nil {
				fields = append(fields,
					slog.Any("from_user_id", sender.ID),
					slog.Any("from_user_username", sender.Username),
				)
			}

			if msg := c.Message(); msg != nil {
				fields = append(fields,
					slog.Any("message_id", msg.ID),
					slog.Any("message_text", msg.Text),
				)
			}

			log.LogAttrs(ctx, slog.LevelDebug, "Incoming message received", fields...)

			return next(c)
		}
	}
}
