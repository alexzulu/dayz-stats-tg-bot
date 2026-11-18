package bot

import (
	"errors"
	"sync"
	"time"

	"gopkg.in/telebot.v4"
)

// ErrBotClosed is returned by Telegram methods after it has been closed.
var ErrBotClosed = errors.New("bot is closed")

// Telegram wraps a single [telebot.Bot] with a concurrency-safe lifecycle.
type Telegram struct {
	botToken string

	client  *telebot.Bot
	started bool
	closed  bool

	mu sync.Mutex
}

// NewTelegram returns a Telegram for the given bot token. The bot is not connected until the first method that
// needs it.
func NewTelegram(botToken string) *Telegram {
	return &Telegram{
		botToken: botToken,
	}
}

// bot returns the [telebot.Bot] instance, initializing it if necessary.
// The caller is responsible for locking the mutex before calling this function.
func (t *Telegram) bot() (*telebot.Bot, error) {
	if t.closed {
		return nil, ErrBotClosed
	}

	if t.client != nil {
		return t.client, nil
	}

	bot, err := telebot.NewBot(telebot.Settings{
		Token:  t.botToken,
		Poller: &telebot.LongPoller{Timeout: 10 * time.Second}, //nolint:mnd
	})
	if err != nil {
		return nil, err
	}

	t.client = bot

	return t.client, nil
}

// botWithLock returns the [telebot.Bot] instance, initializing it if necessary.
// It locks the mutex for the duration of the call.
func (t *Telegram) botWithLock() (*telebot.Bot, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	return t.bot()
}

// OnPrivateMessage registers the handler for text messages from private chats, ignoring group and channel messages;
// It connects the bot on first call and restarts the polling loop so the handler takes effect.
func (t *Telegram) OnPrivateMessage(h telebot.HandlerFunc, m ...telebot.MiddlewareFunc) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	bot, err := t.bot()
	if err != nil {
		return err
	}

	if t.started {
		bot.Stop()

		t.started = false
	}

	bot.Handle(telebot.OnText, func(c telebot.Context) error {
		if chat := c.Chat(); chat != nil && chat.Type != telebot.ChatPrivate {
			return nil
		}

		return h(c)
	}, m...)

	if !t.started {
		go bot.Start()

		t.started = true
	}

	return nil
}

// Close stops the polling loop and releases the bot. Safe to call more than once.
func (t *Telegram) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.client != nil {
		if t.started {
			t.client.Stop()

			t.started = false
		}

		t.client = nil
	}

	t.closed = true

	return nil
}
