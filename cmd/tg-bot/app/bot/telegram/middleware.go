package telegram

import (
	"context"
	"log/slog"
	"slices"

	tele "gopkg.in/telebot.v4"
	mw "gopkg.in/telebot.v4/middleware"
)

// NewLoggerMiddleware returns a middleware that logs incoming messages with structured logging.
func NewLoggerMiddleware(ctx context.Context, log *slog.Logger) tele.MiddlewareFunc {
	return func(next tele.HandlerFunc) tele.HandlerFunc {
		return func(c tele.Context) error {
			log.LogAttrs(ctx, slog.LevelDebug, "Incoming message received", loggerFieldsFromTgContext(c)...)

			return next(c)
		}
	}
}

// NewAuthMiddleware returns a middleware that checks if the sender is in the list of admin IDs.
func NewAuthMiddleware(ctx context.Context, log *slog.Logger, adminIDs []int64) tele.MiddlewareFunc {
	return func(next tele.HandlerFunc) tele.HandlerFunc {
		return func(c tele.Context) error {
			sender := c.Sender()
			if sender == nil {
				return c.Send("Unauthorized: no sender information")
			}

			if slices.Contains(adminIDs, sender.ID) {
				return next(c)
			}

			log.LogAttrs(ctx, slog.LevelWarn, "Unauthorized access attempt", loggerFieldsFromTgContext(c)...)

			return c.Send("Unauthorized: you are not an admin")
		}
	}
}

// NewRecoverMiddleware returns a middleware that recovers from panics in handlers and logs the error with
// structured logging.
func NewRecoverMiddleware(ctx context.Context, log *slog.Logger) tele.MiddlewareFunc {
	return mw.Recover(func(err error, c tele.Context) {
		log.LogAttrs(ctx, slog.LevelError, "Telegram handler panic recovered",
			append(loggerFieldsFromTgContext(c), slog.String("error", err.Error()))...,
		)
	})
}

// loggerFieldsFromTgContext extracts structured logging fields from a [tele.Context].
func loggerFieldsFromTgContext(c tele.Context) []slog.Attr {
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

	return fields
}
