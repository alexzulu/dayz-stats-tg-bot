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
	log *slog.Logger
	tb  *bot.Telegram
	cf  *config.File
	cp  *statez.CachedClientPool
}

// NewStatusUpdater returns a StatusUpdater for the bound live-status messages.
func NewStatusUpdater(log *slog.Logger, tb *bot.Telegram, cf *config.File, cp *statez.CachedClientPool) *StatusUpdater {
	return &StatusUpdater{log: log, tb: tb, cf: cf, cp: cp}
}

// Run drives the whole update cycle: it owns the timer, derives a safe refresh rate from the config on every
// tick, edits each bound message, drops the ones whose message is gone, and backs off when Telegram rate-limits
// us. It blocks until ctx is canceled, so callers should run it in its own goroutine. On exit it marks every
// bound message as offline.
func (u *StatusUpdater) Run(ctx context.Context) { //nolint:funlen
	const (
		idleInterval = 5 * time.Second // re-check the config this often while nothing is bound
		softGroupCap = 18              // edits/min we allow per group - Telegram's hard cap is 20
		editReserve  = 3               // edits/min kept free per group for /bind (send+edit+pin) and the offline sweep
	)

	// on shutdown, mark every bound message as offline (best-effort, in parallel so one slow edit can't stall the
	// whole shutdown)
	defer func() {
		tbot, err := u.tb.Bot()
		if err != nil {
			return
		}

		var wg sync.WaitGroup

		for _, liveMsg := range u.cf.Config().LiveStatusMessages {
			wg.Go(func() {
				if editErr := u.updateStatusMessageOffline(tbot, liveMsg); editErr != nil {
					u.log.Error("server status updater: failed to mark message as offline",
						slog.Int64("chat_id", liveMsg.ChatID),
						slog.Int("message_id", liveMsg.MessageID),
						slog.String("error", editErr.Error()),
					)
				}
			})
		}

		wg.Wait()
	}()

	timer := time.NewTimer(0) // fire immediately
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}

		tbot, err := u.tb.Bot() // fetched per tick, not stored - the wrapper may replace the client on a restart
		if err != nil {
			timer.Reset(idleInterval)

			continue
		}

		cfg := u.cf.Config()
		if len(cfg.LiveStatusMessages) == 0 {
			timer.Reset(idleInterval)

			continue
		}

		// the chat with the most bound messages sets the pace - it must stay under the cap, and we apply the same
		// rate to everyone for simplicity (chats with fewer messages just update a bit slower than they could)
		perChat := make(map[int64]int)
		maxPerChat := 1

		for _, m := range cfg.LiveStatusMessages {
			perChat[m.ChatID]++
			maxPerChat = max(maxPerChat, perChat[m.ChatID])
		}

		interval := time.Minute / time.Duration(max(1, (softGroupCap-editReserve)/maxPerChat))

		var (
			stale       []config.ConfigLiveStatusMessage // messages that no longer exist, dropped below
			flood       tele.FloodError                  // set when Telegram rate-limits us
			rateLimited bool
		)

		for _, liveMsg := range cfg.LiveStatusMessages {
			serverCfg, ok := cfg.Servers[liveMsg.ServerName]
			if !ok {
				continue // server was removed
			}

			players, world, vehicles, apiErr := u.fetchServerData(ctx, u.cp.Get(serverCfg))
			if apiErr != nil {
				u.log.Warn("server status updater: fetch failed",
					slog.String("server", liveMsg.ServerName),
					slog.String("error", apiErr.Error()),
				)
			}

			switch editErr := u.updateStatusMessage(tbot, liveMsg, players, world, vehicles, apiErr); {
			case editErr == nil, u.isNotModified(editErr):
				// already up to date, nothing to do
			case u.isMessageGone(editErr):
				stale = append(stale, liveMsg)
			case errors.As(editErr, &flood):
				rateLimited = true
			default:
				u.log.Error("server status updater: edit failed",
					slog.String("server", liveMsg.ServerName),
					slog.Int64("chat_id", liveMsg.ChatID),
					slog.Int("message_id", liveMsg.MessageID),
					slog.String("error", editErr.Error()),
				)
			}

			if rateLimited {
				break // stop editing; wait out Telegram's cooldown before the next cycle
			}
		}

		// drop bindings whose message is gone - the config is the source of truth
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

		// if Telegram rate-limited us, wait at least its cooldown - but never faster than our normal cadence
		if rateLimited {
			interval = max(interval, time.Duration(flood.RetryAfter)*time.Second)
			u.log.Warn("server status updater: rate limited, backing off",
				slog.String("retry_after", interval.String()),
			)
		}

		timer.Reset(interval)
	}
}

// fetchServerData pulls players and world for one server concurrently, returning the first error if either call
// fails.
func (*StatusUpdater) fetchServerData(
	ctx context.Context,
	client *statez.CachedClient,
) (*api.Players, *api.World, *api.Vehicles, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second) //nolint:mnd
	defer cancel()

	var (
		wg sync.WaitGroup

		players     *api.Players
		playersErr  error
		world       *api.World
		worldErr    error
		vehicles    *api.Vehicles
		vehiclesErr error
	)

	wg.Go(func() { players, playersErr = client.Players(ctx) })
	wg.Go(func() { world, worldErr = client.World(ctx) })
	wg.Go(func() { vehicles, vehiclesErr = client.Vehicles(ctx) })

	wg.Wait()

	return players, world, vehicles, cmp.Or(playersErr, worldErr, vehiclesErr)
}

// updateStatusMessage edits the bound message in place - the server state on success, an error notice when
// the StateZ daemon could not be reached.
//
// It returns the raw telebot edit error so the caller can tell a gone message from a transient failure.
func (*StatusUpdater) updateStatusMessage( //nolint:funlen
	tb *tele.Bot,
	live config.ConfigLiveStatusMessage,
	players *api.Players,
	world *api.World,
	vehicles *api.Vehicles,
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
			b.WriteString("\\\n")
		}

		{ // vehicles
			b.WriteString("🚗 **Машин на спавне**: ")

			var freeCars, cars int

			for _, v := range *vehicles {
				var (
					classType = strings.ToLower(strings.TrimSpace(v.Class.Type))
					isBoat    = strings.Contains(classType, "boat")
					isMoto    = strings.Contains(classType, "motorbike")
				)

				if !isBoat && !isMoto {
					cars++

					if !statez.IsVehicleOccupied(v) {
						freeCars++
					}
				}
			}

			b.WriteString(strconv.Itoa(freeCars))
			b.WriteString(" (из ")
			b.WriteString(strconv.Itoa(cars))
			b.WriteString(")\n")
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
