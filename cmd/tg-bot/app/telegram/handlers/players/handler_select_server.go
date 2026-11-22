package players

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"gh.tarampamp.am/dayz-statez/server/pkg/api"
	tele "gopkg.in/telebot.v4"

	"github.com/alexzulu/dayz-stats-tg-bot/cmd/tg-bot/app/telegram/clients"
	tt "github.com/alexzulu/dayz-stats-tg-bot/cmd/tg-bot/app/telegram/tools"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/config"
)

// NewSelectServerHandler returns the /players handler - a per-server summary table (map, players, in-game time,
// uptime, FPS) above an inline picker where tapping a server fires ServerPlayersListBtn for it. Each server's
// world snapshot is fetched concurrently; an unreachable server still gets a row (marked) and a button.
func NewSelectServerHandler(
	ctx context.Context,
	log *slog.Logger,
	cf *config.File,
	cp *clients.Pool,
) tele.HandlerFunc {
	return func(c tele.Context) error {
		cfg := cf.Config()
		if len(cfg.Servers) == 0 {
			return tt.SendRichMarkdownError(c, "No servers configured yet", nil)
		}

		names := slices.Sorted(maps.Keys(cfg.Servers)) // stable order

		type mapInfo struct {
			data *api.World
			err  error
		}

		var (
			wg      sync.WaitGroup
			mu      sync.Mutex
			dataMap = make(map[string]mapInfo, len(names))
		)

		reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second) //nolint:mnd
		defer cancel()

		for _, name := range names {
			wg.Go(func() {
				data, err := cp.Get(cfg.Servers[name]).World(reqCtx)
				if err != nil {
					log.Error("Failed to fetch world for server summary",
						slog.String("server", name),
						slog.Any("error", err),
					)
				}

				mu.Lock()
				defer mu.Unlock()

				if _, ok := dataMap[name]; !ok {
					dataMap[name] = mapInfo{data, err}
				}
			})
		}

		wg.Wait()

		var b strings.Builder

		b.Grow(128 + len(names)*64) //nolint:mnd // rough estimate

		bw(&b, "Select a server to list its players online:\n\n")
		bw(&b, "| Server | Map | Players | In-game time | Uptime |\n")
		bw(&b, "|:------:|:---:|:-------:|:------------:|:------:|\n")

		for _, name := range names {
			info, ok := dataMap[name]
			switch {
			case !ok:
				continue // should never happen, but just in case
			case info.err != nil || info.data == nil:
				bw(&b, "| `", name, "` ", warnIf(true), " | - | - | - | - |\n")

				continue
			}

			bw(&b, "| `", name, "` ")
			bw(&b, "| ", info.data.MapName, " ")
			bw(&b, "| ", strconv.Itoa(info.data.Players.Online), "/", strconv.Itoa(info.data.Players.Max), " ")
			bw(&b, "| ", fmt.Sprintf("%02d:%02d", info.data.Time.Hour, info.data.Time.Minute), " ")
			uptime := secondsToHours(info.data.UptimeSeconds)
			bw(&b, "| ", strconv.Itoa(uptime), " ", plural(uptime, "hour", "hours"), " ")
			bw(&b, "|\n")
		}

		bw(&b, "\n\n")

		var (
			markup  tele.ReplyMarkup
			buttons = make([]tele.Btn, 0, len(names))
		)

		for _, name := range names {
			buttons = append(buttons, markup.Data("🗺️ "+name, ServerPlayersListBtn.Unique, name))
		}

		markup.Inline(markup.Split(2, buttons)...) //nolint:mnd

		return c.Send(&tele.InputRichMessage{Markdown: b.String()}, &markup)
	}
}
