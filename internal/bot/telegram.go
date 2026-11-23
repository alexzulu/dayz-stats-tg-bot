package bot

import (
	"errors"
	"log/slog"
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
	log      *slog.Logger

	client  *telebot.Bot
	started bool
	closed  bool

	mu sync.Mutex
}

// NewTelegram returns a Telegram for the given bot token. The bot is not connected until the first method that
// needs it.
func NewTelegram(botToken string, log *slog.Logger) *Telegram {
	return &Telegram{
		botToken: botToken,
		log:      log,
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

	onError := func() func(err error, _ telebot.Context) {
		if t.log != nil {
			return func(err error, _ telebot.Context) {
				if err == nil {
					return
				}

				t.log.Error(err.Error())
			}
		}

		return nil
	}()

	bot, err := telebot.NewBot(telebot.Settings{
		Token:   t.botToken,
		Poller:  &telebot.LongPoller{Timeout: 10 * time.Second}, //nolint:mnd
		OnError: onError,
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

// Command is a Telegram command with its handler, name, description, and optional middleware.
type Command struct {
	Text        string
	Description string
	Handler     telebot.HandlerFunc
	Middleware  []telebot.MiddlewareFunc
	AllChats    bool // when true the handler fires in every chat type, not just private chats
}

// RegisterPrivateCommands wires each command's handler and advertises the set, so clients show them under the
// "Menu" button.
//
// Commands with a nil handler are skipped, handlers fire only in private chats.
//
// The variadic m is applied to every command as outermost middleware (before per-command Middleware).
//
// NOTE: You need to call Start to start the polling loop after registering commands.
func (t *Telegram) RegisterPrivateCommands(commands []Command, globMw ...telebot.MiddlewareFunc) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	bot, bErr := t.bot()
	if bErr != nil {
		return bErr
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

		handler := cmd.Handler
		if !cmd.AllChats {
			handler = func(c telebot.Context) error {
				if chat := c.Chat(); chat != nil && chat.Type == telebot.ChatPrivate {
					return cmd.Handler(c)
				}

				return nil
			}
		}

		bot.Handle(
			"/"+trigger, // telebot routes on "/cmd"; text is bare, so prepend slash
			handler,
			append(slices.Clone(globMw), cmd.Middleware...)...,
		)
	}

	// clear any existing commands in default scope
	if err := bot.DeleteCommands(); err != nil {
		return err
	}

	if err := bot.SetCommands(cmds, telebot.CommandScope{Type: telebot.CommandScopeAllPrivateChats}); err != nil {
		return err
	}

	return nil
}

// Button is a Telegram callback button with its handler and optional middleware.
type Button struct {
	Button     telebot.CallbackEndpoint
	Handler    telebot.HandlerFunc
	Middleware []telebot.MiddlewareFunc
	AllChats   bool // when true the handler fires in every chat type, not just private chats
}

// RegisterPrivateButtonHandlers wires each button's handler. Handlers fire only in private chats.
//
// The variadic globMw is applied to every button as outermost middleware (before per-button Middleware).
//
// NOTE: You need to call Start to start the polling loop after registering handlers.
func (t *Telegram) RegisterPrivateButtonHandlers(btns []Button, globMw ...telebot.MiddlewareFunc) error {
	if len(btns) == 0 {
		return nil // no-op
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	bot, bErr := t.bot()
	if bErr != nil {
		return bErr
	}

	for _, btn := range btns {
		if btn.Handler == nil {
			continue
		}

		handler := btn.Handler
		if !btn.AllChats {
			handler = func(c telebot.Context) error {
				if chat := c.Chat(); chat != nil && chat.Type == telebot.ChatPrivate {
					return btn.Handler(c)
				}

				return nil
			}
		}

		bot.Handle(btn.Button, handler, append(slices.Clone(globMw), btn.Middleware...)...)
	}

	return nil
}

// Start starts the polling loop if it is not already running, or restarts otherwise. Safe to call more than once.
func (t *Telegram) Start() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	bot, err := t.bot()
	if err != nil {
		return err
	}

	if t.started {
		bot.Stop()
	}

	go bot.Start()

	t.started = true

	return nil
}

// Me returns the bot's own user (its ID and username), initializing the bot if necessary.
func (t *Telegram) Me() (*telebot.User, error) {
	bot, err := t.botWithLock()
	if err != nil {
		return nil, err
	}

	return bot.Me, nil
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
