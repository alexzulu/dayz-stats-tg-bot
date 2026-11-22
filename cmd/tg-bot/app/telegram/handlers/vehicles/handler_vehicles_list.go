package vehicles

import (
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"io"
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
	"github.com/alexzulu/dayz-stats-tg-bot/cmd/tg-bot/app/telegram/validate"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/config"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/gamemap"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/statez"
)

// ServerVehiclesListBtn is the inline button that lists a server's vehicles; its callback payload is the
// server name.
var ServerVehiclesListBtn = tele.Btn{Text: "Vehicles", Unique: "vehicles"} //nolint:gochecknoglobals

// NewServerVehiclesListBtnHandler returns the callback handler for ServerVehiclesListBtn. It fetches the
// server's vehicles, renders a per-model availability table, and (best-effort) an overview map with each
// vehicle plotted and labeled by model.
func NewServerVehiclesListBtnHandler( //nolint:funlen
	ctx context.Context,
	log *slog.Logger,
	cf *config.File,
	cp *clients.Pool,
) tele.HandlerFunc {
	return func(c tele.Context) error {
		serverName := c.Data() // received from [ServerVehiclesListBtn] button via `callback_data`

		if err := validate.ServerName(serverName); err != nil {
			return tt.SendRichMarkdownError(c, "Invalid server name", err)
		}

		cfg := cf.Config()

		serverConfig, exists := cfg.Servers[serverName]
		if !exists {
			return tt.SendRichMarkdownError(c, fmt.Sprintf("Server `%s` does not exist", serverName), nil)
		}

		// vehicles are required; world is only needed for the map, so its failure is non-fatal
		var (
			client = cp.Get(serverConfig)
			wg     sync.WaitGroup

			vehicles *api.Vehicles
			vErr     error
			world    *api.World
			wErr     error
		)

		reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second) //nolint:mnd
		defer cancel()

		wg.Go(func() { vehicles, vErr = client.Vehicles(reqCtx) })
		wg.Go(func() { world, wErr = client.World(reqCtx) })

		wg.Wait()

		switch {
		case vErr != nil:
			return tt.SendRichMarkdownError(c, "Failed to fetch vehicles", vErr)
		case len(*vehicles) == 0:
			return tt.SendRichMarkdownError(c, fmt.Sprintf("No vehicles tracked on server `%s`", serverName), nil)
		}

		type tableRow struct{ total, free int }

		var (
			mapPoints      = make([]gamemap.Point, 0, len(*vehicles))
			tableData      = make(map[string]*tableRow) // map[className]data
			occupiedHashes = statez.OccupiedHashes(*vehicles)
		)

		for i := range *vehicles {
			v := (*vehicles)[i]
			label, color, _ := friendlyNameAndColor(v.Class.Name)
			isOccupied := slices.Contains(occupiedHashes, v.Hash)

			mapPoints = append(mapPoints, gamemap.Point{
				PosX:        v.Pos.X,
				PosY:        v.Pos.Y,
				Color:       color,
				TopLabel:    iff(label != "", label, v.Class.Name),
				BottomLabel: iff(isOccupied, "", "Free"),
			})

			classType := strings.ToLower(strings.TrimSpace(v.Class.Type))
			if strings.Contains(classType, "boat") || strings.Contains(classType, "motorbike") {
				continue // exclude boats/motobikes from the table, as they are not relevant for most players
			}

			if row, ok := tableData[label]; ok {
				row.total++ // increment total count for this model
				if !isOccupied {
					row.free++ // increment free count for this model
				}
			} else {
				tableData[label] = &tableRow{
					total: 1,
					free:  iff(isOccupied, 0, 1),
				}
			}
		}

		sortedLabels := slices.Sorted(maps.Keys(tableData)) // stable order

		var b strings.Builder

		b.Grow(256 + len(tableData)*32) //nolint:mnd // rough estimate

		bw(&b, "Vehicles on server `", serverName, "` *(number in parentheses = free)*:\n\n")

		// table header
		bw(&b, "|")
		for _, label := range sortedLabels { //nolint:wsl_v5
			bw(&b, " ", label, " |")
		}
		bw(&b, "\n") //nolint:wsl_v5

		// separator
		bw(&b, "|")
		for range sortedLabels { //nolint:wsl_v5
			bw(&b, ":---:|")
		}
		bw(&b, "\n") //nolint:wsl_v5

		// table data
		bw(&b, "|")
		for _, label := range sortedLabels { //nolint:wsl_v5
			row := tableData[label]
			bw(&b, " ", strconv.Itoa(row.total), " (", strconv.Itoa(row.free), ") |")
		}
		bw(&b, "\n\n") //nolint:wsl_v5

		// overview map: every vehicle plotted, colored and labeled by model; best-effort - skipped on failure
		var media []tele.InputRichMessageMedia

		if wErr != nil {
			log.Error("Failed to fetch world for vehicles map", slog.String("error", wErr.Error()))
		} else if gameMap, gmErr := gamemap.MapByName(world.MapName); gmErr == nil && gameMap != nil {
			if img, imgErr := gamemap.DrawPoints(gameMap, mapPoints); imgErr == nil {
				// stream the jpeg into the upload without buffering the whole image
				pr := gamemap.EncodeReader(img, func(w io.Writer, m image.Image) error {
					return jpeg.Encode(w, m, &jpeg.Options{Quality: 85}) //nolint:mnd
				})
				defer func() { _ = pr.Close() }()

				b.WriteString("\n![](tg://photo?id=vehicles-map)")

				media = append(media, tele.InputRichMessageMedia{
					ID:    "vehicles-map",
					Media: &tele.Photo{File: tele.FromReader(pr)},
				})
			} else {
				log.Error("Failed to render vehicles map", slog.String("map", world.MapName), slog.Any("error", imgErr))
			}
		}

		return c.Send(&tele.InputRichMessage{Markdown: b.String(), Media: media})
	}
}
