package tgbot

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	tele "gopkg.in/telebot.v4"

	"github.com/alexzulu/dayz-stats-tg-bot/internal/server"
)

// TGBot is a wrapper around the telebot library that provides methods for interacting with the Telegram Bot API.
type TGBot struct {
	token   string
	client  *tele.Bot // initialized via Connect()
	started atomic.Bool
}

var (
	ErrNotInitialized = errors.New("client is not initialized")     //nolint:godoclint
	ErrInitialized    = errors.New("client is already initialized") //nolint:godoclint
	ErrAlreadyStarted = errors.New("bot is already started")        //nolint:godoclint
)

// New creates a new TGBot instance.
func New(token string) *TGBot {
	return &TGBot{token: token}
}

// Connect initializes the Telegram bot client. It must be called before any other methods.
// If the client is already initialized, it returns ErrInitialized.
func (t *TGBot) Connect() error {
	if t.client != nil {
		return ErrInitialized
	}

	c, err := tele.NewBot(tele.Settings{
		Token:  t.token,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second}, //nolint:mnd
	})
	if err != nil {
		return err
	}

	t.client = c

	return nil
}

// Start starts the bot and listens for incoming messages. The bot will run until the provided context is canceled.
func (t *TGBot) Start(ctx context.Context) error {
	if t.client == nil {
		return ErrNotInitialized
	}

	if !t.started.CompareAndSwap(false, true) {
		return ErrAlreadyStarted
	}

	t.client.Handle(tele.OnText, func(c tele.Context) error {
		if c.Chat().Type != tele.ChatPrivate {
			return nil
		}

		return c.Reply("I'm alive!")
	})

	// start the bot in a separate goroutine, and stop it when the context is canceled (this required for handling
	// incoming messages)
	go func() {
		// stop the bot when the context is canceled
		go func() { <-ctx.Done(); t.client.Stop(); t.started.Store(false) }()

		t.client.Start()
	}()

	return nil
}

// SendInitialMessage sends an initial message to a chat and returns the message ID.
func (t *TGBot) SendInitialMessage(chatID int64, threadID int) (int, error) {
	if t.client == nil {
		return 0, ErrNotInitialized
	}

	msg, msgErr := t.client.Send(
		&tele.Chat{ID: chatID},
		"🟢 Initializing...",
		&tele.SendOptions{
			ThreadID:            threadID,
			ParseMode:           tele.ModeMarkdown,
			DisableNotification: true,
		},
	)
	if msgErr != nil {
		return 0, msgErr
	}

	if _, err := t.client.Edit(msg,
		fmt.Sprintf("✅ Initialization complete. Message ID: %d", msg.ID),
		&tele.SendOptions{ThreadID: threadID},
	); err != nil {
		return 0, err
	}

	if err := t.client.Pin(msg, tele.Silent); err != nil {
		return 0, err
	}

	return msg.ID, nil
}

// SetMessagesServerInfo edits a message in a chat to display the current server information.
func (t *TGBot) SetMessagesServerInfo(chatID int64, threadID, messageID int, info *server.Info) error {
	switch {
	case t.client == nil:
		return ErrNotInitialized
	case info == nil:
		return errors.New("server info is nil")
	}

	var text strings.Builder
	text.Grow(128) //nolint:mnd // preallocate some space for the message

	// append player count information (if available)
	if info.MaxPlayersCount > 0 || info.PlayersCount > 0 {
		text.WriteString("🎮 *Игроков* \\(online\\): *")
		text.WriteString(strconv.Itoa(int(info.PlayersCount)))
		text.WriteString("*/")
		text.WriteString(strconv.Itoa(int(info.MaxPlayersCount)))
		text.WriteRune('\n')
	}

	// append server time information (if available)
	if info.ServerTime != nil {
		const sunRiseHour, sunSetHour = 5, 20

		if info.ServerTime[0] >= sunRiseHour && info.ServerTime[0] < sunSetHour {
			text.WriteString("☀️")
		} else {
			text.WriteString("🌙")
		}

		text.WriteString(" *Время*: *")
		text.WriteString(info.ServerTime.String())
		text.WriteString("* \\(местное\\)\n")
	}

	// append ping information (if available)
	if info.Ping != time.Duration(0) {
		pingMicro := info.Ping.Microseconds()

		text.WriteString("🏁 *Пинг*: ")
		_, _ = fmt.Fprintf(&text, "*%d*\\.%02d", pingMicro/1000, (pingMicro%1000)/10) //nolint:mnd
		text.WriteString(" мс\n")
	}

	// append player names (if available)
	if len(info.PlayerNames) > 0 {
		text.WriteString("\n> *Хомячат*:\n")

		names := make([]string, len(info.PlayerNames))
		copy(names, info.PlayerNames)
		sort.Strings(names)

		for _, name := range names {
			text.WriteString("> ‣ ")
			text.WriteString(emojiForUsername(name))
			text.WriteString(" __")
			text.WriteString(escapeForMarkdownV2(name))
			text.WriteString("__\n")
		}
	}

	if _, err := t.client.Edit(
		&tele.Message{ID: messageID, Chat: &tele.Chat{ID: chatID}},
		strings.TrimRight(text.String(), "\n"),
		&tele.SendOptions{ThreadID: threadID, ParseMode: tele.ModeMarkdownV2},
	); err != nil {
		if errors.Is(err, tele.ErrSameMessageContent) {
			return nil
		}

		return err
	}

	return nil
}

// SetMessageOffline edits a message in a chat to indicate that the bot is offline.
func (t *TGBot) SetMessageOffline(chatID int64, threadID, messageID int) error {
	if t.client == nil {
		return ErrNotInitialized
	}

	if _, err := t.client.Edit(
		&tele.Message{ID: messageID, Chat: &tele.Chat{ID: chatID}},
		"||🛑 Bot offline||",
		&tele.SendOptions{ThreadID: threadID, ParseMode: tele.ModeMarkdownV2},
	); err != nil {
		if errors.Is(err, tele.ErrSameMessageContent) {
			return nil
		}

		return err
	}

	return nil
}

// SetMessagesServerDown edits a message in a chat to indicate that the server is down and shows the downtime duration.
func (t *TGBot) SetMessagesServerDown(chatID int64, threadID, messageID int, downtime time.Duration) error {
	if t.client == nil {
		return ErrNotInitialized
	}

	var text strings.Builder
	text.Grow(64) //nolint:mnd // preallocate some space for the message

	text.WriteString("❌ *Сервер недоступен*")

	if downtime.Seconds() > 1 {
		total := int(downtime.Round(time.Second).Seconds())

		text.WriteString(" _\\(лежит уже ")
		_, _ = fmt.Fprintf(&text, "%02d:%02d:%02d", total/3600, (total%3600)/60, total%60) //nolint:mnd
		text.WriteString("\\)_")
	}

	if _, err := t.client.Edit(
		&tele.Message{ID: messageID, Chat: &tele.Chat{ID: chatID}},
		text.String(),
		&tele.SendOptions{ThreadID: threadID, ParseMode: tele.ModeMarkdownV2},
	); err != nil {
		if errors.Is(err, tele.ErrSameMessageContent) {
			return nil
		}

		return err
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

var userEmojis = [...]string{ //nolint:gochecknoglobals
	"🪖", "🎒", "🔪", "🗡️", "🪓", "🔫", "💣", "🛡️", "⚔️", "🏕️", "🥫", "🍖", "🥩", "🍄", "🩹",
	"💉", "💊", "🩸", "☢️", "☣️", "💀", "👀", "🐺", "🦌", "🐻", "🐗", "🦊", "🐟", "🔦", "📻",
	"🧭", "🔧", "📦", "🎯", "🍺", "🍻",
}

// emojiForUsername deterministically returns an emoji for a given username.
func emojiForUsername(username string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(username))

	idx := h.Sum32() % uint32(len(userEmojis))

	return userEmojis[idx]
}
