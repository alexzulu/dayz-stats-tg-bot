package vehicles

import (
	"context"
	"log/slog"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	tele "gopkg.in/telebot.v4"

	"github.com/alexzulu/dayz-stats-tg-bot/cmd/tg-bot/app/telegram/clients"
	tt "github.com/alexzulu/dayz-stats-tg-bot/cmd/tg-bot/app/telegram/tools"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/config"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/statez"
)

// NewSelectServerHandler returns the /vehicles handler - a per-server vehicle summary (total count with the
// free count in parentheses) above an inline picker where tapping a server fires ServerVehiclesListBtn for it.
func NewSelectServerHandler( //nolint:gocognit,funlen
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

		type vehInfo struct {
			total, free int
			err         error
		}

		var (
			wg      sync.WaitGroup
			mu      sync.Mutex
			dataMap = make(map[string]vehInfo, len(names))
		)

		reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second) //nolint:mnd
		defer cancel()

		for _, name := range names {
			wg.Go(func() {
				vehicles, err := cp.Get(cfg.Servers[name]).Vehicles(reqCtx)
				if err != nil {
					log.Error("Failed to fetch vehicles for server summary",
						slog.String("server", name),
						slog.Any("error", err),
					)
				}

				var total, free int

				if err == nil && vehicles != nil {
					for i := range *vehicles {
						v := (*vehicles)[i]

						classType := strings.ToLower(strings.TrimSpace(v.Class.Type))
						if strings.Contains(classType, "boat") || strings.Contains(classType, "motorbike") {
							continue // exclude boats/motorbikes, matching the vehicles list table
						}

						total++

						if !statez.IsVehicleOccupied(v) {
							free++
						}
					}
				}

				mu.Lock()
				defer mu.Unlock()

				if _, ok := dataMap[name]; !ok {
					dataMap[name] = vehInfo{total, free, err}
				}
			})
		}

		wg.Wait()

		var b strings.Builder

		b.Grow(128 + len(names)*48) //nolint:mnd // rough estimate

		bw(&b, "Select a server to list its vehicles:\n\n")
		bw(&b, "| Server | Vehicles |\n")
		bw(&b, "|:------:|:--------:|\n")

		for _, name := range names {
			info, ok := dataMap[name]
			switch {
			case !ok:
				continue // should never happen, but just in case
			case info.err != nil:
				bw(&b, "| `", name, "` ", warnIf(true), " | - |\n")

				continue
			}

			bw(&b, "| `", name, "` ")
			bw(&b,
				"| ", strconv.Itoa(info.total),
				" *(free: ", warnIf(info.free < 8), strconv.Itoa(info.free), ")*", //nolint:mnd
			)
			bw(&b, "|\n")
		}

		bw(&b, "\n\n")

		var (
			markup  tele.ReplyMarkup
			buttons = make([]tele.Btn, 0, len(names))
		)

		for _, name := range names {
			buttons = append(buttons, markup.Data("🗺️ "+name, ServerVehiclesListBtn.Unique, name))
		}

		markup.Inline(markup.Split(2, buttons)...) //nolint:mnd

		return c.Send(&tele.InputRichMessage{Markdown: b.String()}, &markup)
	}
}
