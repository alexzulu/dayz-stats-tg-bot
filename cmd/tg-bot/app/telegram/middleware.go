package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"slices"

	tele "gopkg.in/telebot.v4"
	mw "gopkg.in/telebot.v4/middleware"
)

// NewLoggerMiddleware returns a middleware that logs incoming messages with structured logging.
func NewLoggerMiddleware(ctx context.Context, log *slog.Logger) tele.MiddlewareFunc {
	return func(next tele.HandlerFunc) tele.HandlerFunc {
		return func(c tele.Context) error {
			act := func() string {
				switch {
				case c.Callback() != nil:
					return "callback"
				case c.Message() != nil:
					return "message"
				default:
					return "action"
				}
			}()

			log.LogAttrs(ctx, slog.LevelDebug, fmt.Sprintf("Incoming %s received", act), loggerFieldsFromTgContext(c)...)

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

// NewAutoCallbackRespondMiddleware returns a middleware that automatically responds to every callback.
func NewAutoCallbackRespondMiddleware() tele.MiddlewareFunc { return mw.AutoRespond() }

// loggerFieldsFromTgContext extracts structured logging fields from a [tele.Context].
func loggerFieldsFromTgContext(c tele.Context) []slog.Attr {
	var fields []slog.Attr

	if sender := c.Sender(); sender != nil {
		fields = append(fields,
			slog.String("from_user", fmt.Sprintf("%d:%s", sender.ID, sender.Username)),
		)
	}

	if msg := c.Message(); msg != nil {
		fields = append(fields,
			slog.Any("message_id", msg.ID),
			slog.Any("message_text", msg.Text),
		)
	}

	if cb := c.Callback(); cb != nil {
		fields = append(fields,
			slog.String("callback_unique", cb.Unique),
			slog.String("callback_data", cb.Data),
		)
	}

	return fields
}
