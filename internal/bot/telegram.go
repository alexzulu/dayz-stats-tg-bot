package bot

import (
	"errors"
	"slices"
	"strings"
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

// Command represents a Telegram bot command with its text, description, handler, and optional middleware.
type Command struct {
	Text        string
	Description string
	Handler     telebot.HandlerFunc
	Middleware  []telebot.MiddlewareFunc
}

// RegisterPrivateCommands wires each command's handler and advertises the set, so clients show them under the
// "Menu" button.
//
// Commands with a nil handler are skipped, handlers fire only in private chats.
//
// The variadic m is applied to every command as outermost middleware (before per-command Middleware).
func (t *Telegram) RegisterPrivateCommands(commands []Command, globMw ...telebot.MiddlewareFunc) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	bot, bErr := t.bot()
	if bErr != nil {
		return bErr
	}

	if t.started {
		bot.Stop()

		t.started = false
	}

	cmds := make([]telebot.Command, 0, len(commands))
	for _, cmd := range commands {
		if cmd.Handler == nil {
			continue
		}

		trigger := strings.TrimSpace(strings.TrimLeft(cmd.Text, "/ ")) // bare text, no leading slash
		if trigger == "" {
			continue
		}

		cmds = append(cmds, telebot.Command{
			Text:        trigger,
			Description: cmd.Description,
		})

		bot.Handle(
			"/"+trigger, // telebot routes on "/cmd"; text is bare, so prepend slash
			func(c telebot.Context) error {
				if chat := c.Chat(); chat != nil && chat.Type != telebot.ChatPrivate {
					return nil
				}

				return cmd.Handler(c)
			},
			append(slices.Clone(globMw), cmd.Middleware...)...,
		)
	}

	if err := bot.DeleteCommands(); err != nil {
		return err
	}

	if err := bot.SetCommands(cmds, telebot.CommandScope{Type: telebot.CommandScopeAllPrivateChats}); err != nil {
		return err
	}

	if !t.started {
		go bot.Start()

		t.started = true
	}

	return nil
}

// Ping verifies Telegram API reachability and bot token.
func (t *Telegram) Ping() error {
	bot, err := t.botWithLock()
	if err != nil {
		return err
	}

	// Raw surfaces network errors (wrapError) and API errors like 401 on a bad token (extractOk)
	_, err = bot.Raw("getMe", nil)

	return err
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
