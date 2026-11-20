package middleware_test

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	tele "gopkg.in/telebot.v4"

	"github.com/alexzulu/dayz-stats-tg-bot/cmd/tg-bot/app/telegram/middleware"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/testutil/assert"
)

// roundTripFunc adapts a function to http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// newTestContext builds a tele.Context backed by a stub HTTP transport that answers every API call with a
// minimal ok response, so middleware branches that reply (c.Send) never touch the network - no server needed.
func newTestContext(t *testing.T, sender *tele.User) tele.Context {
	t.Helper()

	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		const body = `{"ok":true,"result":{"message_id":1,"chat":{"id":1,"type":"private"},"date":0}}`

		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})}

	bot, err := tele.NewBot(tele.Settings{Offline: true, Client: client, Token: "test"})
	assert.NoError(t, err)

	return bot.NewContext(tele.Update{Message: &tele.Message{
		Sender: sender,
		Chat:   &tele.Chat{ID: 1, Type: tele.ChatPrivate},
		Text:   "hi",
	}})
}

func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

func TestNewLoggerMiddleware(t *testing.T) {
	t.Parallel()

	ctx := newTestContext(t, &tele.User{ID: 1})
	wantErr := errors.New("from handler")

	var called bool

	handler := middleware.NewLoggerMiddleware(t.Context(), discardLogger())(func(tele.Context) error {
		called = true

		return wantErr
	})

	err := handler(ctx)

	assert.True(t, called)          // next is always invoked
	assert.ErrorIs(t, err, wantErr) // and its result is returned unchanged
}

func TestNewAuthMiddleware(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		sender   *tele.User
		adminIDs []int64
		wantNext bool
	}{
		"admin passes through":      {sender: &tele.User{ID: 42}, adminIDs: []int64{42}, wantNext: true},
		"non-admin is blocked":      {sender: &tele.User{ID: 7}, adminIDs: []int64{42}, wantNext: false},
		"missing sender is blocked": {sender: nil, adminIDs: []int64{42}, wantNext: false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ctx := newTestContext(t, tt.sender)

			var called bool

			handler := middleware.NewAuthMiddleware(t.Context(), discardLogger(), tt.adminIDs)(func(tele.Context) error {
				called = true

				return nil
			})

			_ = handler(ctx)

			assert.Equal(t, tt.wantNext, called)
		})
	}
}

func TestNewRecoverMiddleware(t *testing.T) {
	t.Parallel()

	t.Run("recovers from a panic and swallows it", func(t *testing.T) {
		t.Parallel()

		ctx := newTestContext(t, &tele.User{ID: 1})

		handler := middleware.NewRecoverMiddleware(t.Context(), discardLogger())(func(tele.Context) error {
			panic(errors.New("boom"))
		})

		var err error

		assert.NotPanics(t, func() { err = handler(ctx) })
		assert.NoError(t, err)
	})

	t.Run("passes a normal error through unchanged", func(t *testing.T) {
		t.Parallel()

		ctx := newTestContext(t, &tele.User{ID: 1})
		wantErr := errors.New("handler failed")

		handler := middleware.NewRecoverMiddleware(t.Context(), discardLogger())(func(tele.Context) error {
			return wantErr
		})

		assert.ErrorIs(t, handler(ctx), wantErr)
	})
}
