package telegram

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"gh.tarampamp.am/dayz-statez/server/pkg/api"
	tele "gopkg.in/telebot.v4"

	tt "github.com/alexzulu/dayz-stats-tg-bot/cmd/tg-bot/app/telegram/tools"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/bot"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/config"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/gamemap/maps/chernarusplus"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/statez"
)

// StatusUpdater keeps every bound live-status message in sync with its server's StateZ data.
//
// The config is the single source of truth for what to update.
type StatusUpdater struct {
	log     *slog.Logger
	tb      *bot.Telegram
	cf      *config.File
	cp      *statez.CachedClientPool
	updFreq time.Duration
}

// NewStatusUpdater returns a StatusUpdater that refreshes bound messages every updFreq.
func NewStatusUpdater(
	log *slog.Logger,
	tb *bot.Telegram,
	cf *config.File,
	cp *statez.CachedClientPool,
	updFreq time.Duration,
) *StatusUpdater {
	return &StatusUpdater{
		log:     log,
		tb:      tb,
		cf:      cf,
		cp:      cp,
		updFreq: updFreq,
	}
}

// Run refreshes all bound messages immediately, then every updFreq. It blocks until ctx is canceled, so callers
// should run it in its own goroutine.
func (u *StatusUpdater) Run(ctx context.Context) {
	t := time.NewTimer(0) // fire immediately, then every updFreq
	defer t.Stop()

	defer func() {
		var wg sync.WaitGroup

		for _, liveMsg := range u.cf.Config().LiveStatusMessages {
			wg.Go(func() {
				tbot, err := u.tb.Bot() // fetched per tick
				if err != nil {
					return
				}

				if editErr := u.updateStatusMessageOffline(tbot, liveMsg); editErr != nil {
					u.log.Error("server status updater: failed to mark message as offline")
				}
			})
		}

		wg.Wait()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			u.refresh(ctx)

			t.Reset(u.updFreq)
		}
	}
}

// refresh updates all bound messages concurrently, then drops in one pass any binding whose message turned out to
// be gone.
func (u *StatusUpdater) refresh(ctx context.Context) {
	tbot, err := u.tb.Bot() // fetched per tick, not stored - the wrapper may replace the client on a restart
	if err != nil {
		u.log.Error("server status updater: bot unavailable", slog.String("error", err.Error()))

		return
	}

	cfg := u.cf.Config()
	if len(cfg.LiveStatusMessages) == 0 {
		return
	}

	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		stale []config.ConfigLiveStatusMessage // bindings whose message is gone, dropped after the fan-out
	)

	for _, liveMsg := range cfg.LiveStatusMessages {
		serverCfg, ok := cfg.Servers[liveMsg.ServerName]
		if !ok {
			continue // server was removed; /remove already drops its bindings, so just skip
		}

		client := u.cp.Get(serverCfg)

		wg.Go(func() {
			players, world, apiErr := u.fetchServerData(ctx, client)
			if apiErr != nil {
				u.log.Warn("server status updater: fetch failed",
					slog.String("server", liveMsg.ServerName),
					slog.String("error", apiErr.Error()),
				)
			}

			switch editErr := u.updateStatusMessage(tbot, liveMsg, players, world, apiErr); {
			case editErr == nil, u.isNotModified(editErr):
				// already up to date, nothing to do
			case u.isMessageGone(editErr):
				mu.Lock()
				stale = append(stale, liveMsg) //nolint:wsl_v5 // keep the tiny critical section cuddled
				mu.Unlock()
			default:
				u.log.Error("server status updater: edit failed",
					slog.String("server", liveMsg.ServerName),
					slog.Int64("chat_id", liveMsg.ChatID),
					slog.Int("message_id", liveMsg.MessageID),
					slog.String("error", editErr.Error()),
				)
			}
		})
	}

	wg.Wait()

	if len(stale) > 0 {
		u.cf.Update(func(cfg *config.Config) {
			cfg.LiveStatusMessages = slices.DeleteFunc(cfg.LiveStatusMessages, func(m config.ConfigLiveStatusMessage) bool {
				return slices.Contains(stale, m)
			})
		})

		if saveErr := u.cf.Save(); saveErr != nil {
			u.log.Error("server status updater: failed to persist config after dropping stale bindings",
				slog.String("error", saveErr.Error()),
			)
		}
	}
}

// fetchServerData pulls players and world for one server concurrently, returning the first error if either call
// fails.
func (*StatusUpdater) fetchServerData(
	ctx context.Context,
	client *statez.CachedClient,
) (*api.Players, *api.World, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second) //nolint:mnd
	defer cancel()

	var (
		wg sync.WaitGroup

		players    *api.Players
		playersErr error
		world      *api.World
		worldErr   error
	)

	wg.Go(func() { players, playersErr = client.Players(ctx) })
	wg.Go(func() { world, worldErr = client.World(ctx) })

	wg.Wait()

	return players, world, cmp.Or(playersErr, worldErr)
}

// updateStatusMessage edits the bound message in place - the server state on success, an error notice when
// the StateZ daemon could not be reached.
//
// It returns the raw telebot edit error so the caller can tell a gone message from a transient failure.
func (*StatusUpdater) updateStatusMessage(
	tb *tele.Bot,
	live config.ConfigLiveStatusMessage,
	players *api.Players,
	world *api.World,
	apiErr error,
) error {
	// StoredMessage lets us edit by chat+message ID without a tele.Context (we are off the update loop)
	stored := tele.StoredMessage{MessageID: strconv.Itoa(live.MessageID), ChatID: live.ChatID}

	var markdown string

	if apiErr != nil || players == nil || world == nil { //nolint:nestif
		markdown = "❌ **Сервер недоступен**"
	} else {
		var b strings.Builder

		b.Grow(256) //nolint:mnd // rough estimate

		{ // players
			b.WriteString("🎮 **Игроков** (online): **")
			b.WriteString(strconv.Itoa(world.Players.Online))
			b.WriteString("**/")
			b.WriteString(strconv.Itoa(world.Players.Max))
			b.WriteString("\\\n")
		}

		{ // world time
			const sunRiseHour, sunSetHour = 5, 20

			if world.Time.Hour >= sunRiseHour && world.Time.Hour < sunSetHour {
				b.WriteString("☀️")
			} else {
				b.WriteString("🌙")
			}

			b.WriteString(" **Время**: **")
			_, _ = fmt.Fprintf(&b, "%02d:%02d", world.Time.Hour, world.Time.Minute)
			b.WriteString("** ")
			b.WriteString("(местное)\\\n")
		}

		{ // weather
			const weatherStep = 0.3

			b.WriteString("🌦️ **Погода**: ")

			switch {
			case world.Weather.Rain >= weatherStep:
				b.WriteString("🌧️ дождь")
			case world.Weather.Snow >= weatherStep:
				b.WriteString("❄️ снег")
			case world.Weather.Fog >= weatherStep:
				b.WriteString("туман")
			default:
				b.WriteString("☀️ ясно")
			}

			b.WriteString(", ")
			_, _ = fmt.Fprintf(&b, "%.1f °C", world.Weather.Temperature)
			b.WriteString("\n")
		}

		b.WriteString("\n")

		if len(*players) > 0 { // list players
			b.WriteString("\n> **Хомячат**:\\\n")

			for i, p := range *players {
				b.WriteString("> ‣ ")
				b.WriteString(emojiForUsername(p.ID.Name))
				b.WriteString(" ")
				b.WriteString(tt.EscapeMarkdown(p.ID.Name))

				if m := chernarusplus.Map; world.MapName == m.Name() {
					if areas := m.InAreas(p.Pos.X, p.Pos.Y); slices.Contains(areas, chernarusplus.NWAFAreaName) {
						b.WriteString(" *(в PvP зоне)*")
					}
				}

				if i < len(*players)-1 {
					b.WriteString("\\\n")
				} else {
					b.WriteString("\n")
				}
			}
		}

		b.WriteString("\n")

		markdown = b.String()
	}

	_, err := tb.Edit(stored, &tele.InputRichMessage{Markdown: markdown, SkipEntityDetection: true})

	return err
}

// updateStatusMessageOffline edits the bound message in place to indicate the server is offline.
func (*StatusUpdater) updateStatusMessageOffline(
	tb *tele.Bot,
	live config.ConfigLiveStatusMessage,
) error {
	stored := tele.StoredMessage{MessageID: strconv.Itoa(live.MessageID), ChatID: live.ChatID}

	_, err := tb.Edit(stored, &tele.InputRichMessage{Markdown: "🛑 **Бот отключен**", SkipEntityDetection: true})

	return err
}

// isNotModified reports whether the edit was a no-op because the content did not change (an expected, benign
// outcome when the data is already current).
func (*StatusUpdater) isNotModified(err error) bool {
	return errors.Is(err, tele.ErrMessageNotModified) || errors.Is(err, tele.ErrSameMessageContent)
}

// isMessageGone reports whether the edit target no longer exists - the message was deleted, the chat is gone, or
// the bot was kicked - so the binding should be dropped.
func (*StatusUpdater) isMessageGone(err error) bool {
	if errors.Is(err, tele.ErrChatNotFound) ||
		errors.Is(err, tele.ErrNotFound) ||
		errors.Is(err, tele.ErrKickedFromGroup) ||
		errors.Is(err, tele.ErrKickedFromSuperGroup) ||
		errors.Is(err, tele.ErrKickedFromChannel) {
		return true
	}

	return strings.Contains(strings.ToLower(err.Error()), "not found")
}

var userEmojis = [...]rune{ //nolint:gochecknoglobals
	'🪖', '🎒', '🔪', '🪓', '🔫', '💣', '🥫', '🍖', '🥩', '🍄', '🩹', '💉', '💊', '🩸', '💀', '👀', '🐺',
	'🦌', '🐻', '🐗', '🦊', '🐟', '🔦', '📻', '🧭', '🔧', '📦', '🎯', '🍺', '🍻',
}

// emojiForUsername deterministically returns an emoji for a given username.
func emojiForUsername(username string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(username))

	idx := h.Sum32() % uint32(len(userEmojis))

	return string(userEmojis[idx])
}
