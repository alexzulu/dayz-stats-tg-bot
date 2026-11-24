package players

import (
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gh.tarampamp.am/dayz-statez/server/pkg/api"
	tele "gopkg.in/telebot.v4"

	tt "github.com/alexzulu/dayz-stats-tg-bot/cmd/tg-bot/app/telegram/tools"
	"github.com/alexzulu/dayz-stats-tg-bot/cmd/tg-bot/app/telegram/validate"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/config"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/gamemap"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/statez"
)

// ServerPlayersListBtn is the inline button that lists a server's online players; its callback payload is the
// server name.
var ServerPlayersListBtn = tele.Btn{Text: "Players online", Unique: "players"} //nolint:gochecknoglobals

// NewServerPlayersListBtnHandler returns the callback handler for ServerPlayersListBtn. It fetches the server's
// online players, renders a stats table, and attaches a per-player drill-down button (payload
// "<server>|<steam_id>") for every player that has a Steam ID.
func NewServerPlayersListBtnHandler( //nolint:funlen
	ctx context.Context,
	log *slog.Logger,
	cf *config.File,
	cp *statez.CachedClientPool,
) tele.HandlerFunc {
	return func(c tele.Context) error {
		serverName := c.Data() // received from [ServerPlayersListBtn] button via `callback_data`

		if err := validate.ServerName(serverName); err != nil {
			return tt.SendRichMarkdownError(c, "Invalid server name", err)
		}

		cfg := cf.Config()

		serverConfig, exists := cfg.Servers[serverName]
		if !exists {
			return tt.SendRichMarkdownError(c, fmt.Sprintf("Server `%s` does not exist", serverName), nil)
		}

		// players are required; world is only needed for the map, so its failure is non-fatal
		var (
			client = cp.Get(serverConfig)
			wg     sync.WaitGroup

			players *api.Players
			pErr    error
			world   *api.World
			wErr    error
		)

		reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second) //nolint:mnd
		defer cancel()

		wg.Go(func() { players, pErr = client.Players(reqCtx) })
		wg.Go(func() { world, wErr = client.World(reqCtx) })

		wg.Wait()

		switch {
		case pErr != nil:
			return tt.SendRichMarkdownError(c, "Failed to fetch players", pErr)
		case len(*players) == 0:
			return tt.SendRichMarkdownError(c, fmt.Sprintf("No players online on server `%s`", serverName), nil)
		}

		// sort players to get stable order
		sort.SliceStable(*players, func(i, j int) bool { return (*players)[i].ID.GUID < (*players)[j].ID.GUID })

		type playerBtn struct{ name, steamID string }

		playerButtons := make([]playerBtn, 0, len(*players))

		var b strings.Builder

		b.Grow(256 + (len(*players) * 64)) //nolint:mnd // rough estimate

		bw(&b, "Players online on server `"+serverName+"`:\n\n")
		bw(&b, "| Nickname | 💧  | 🍏  | 🩸  | ❤️  | Total playtime | Kills |\n")
		bw(&b, "|:--------:|:---:|:---:|:---:|:---:|:--------------:|:-----:|\n")

		// one row per named player; collect a button target for each one that has a steam id
		for _, p := range *players {
			if p.ID.Name == "" {
				continue
			}

			if p.ID.SteamID != "" {
				playerButtons = append(playerButtons, playerBtn{name: p.ID.Name, steamID: p.ID.SteamID})
			}

			bw(&b, "| `", removeBackticks(p.ID.Name), "`", iff(p.State.Godmode, " 👼", ""), " ")
			bw(&b, "| ", percent(p.Vitals.Water), " ")
			bw(&b, "| ", percent(p.Vitals.Energy), " ")
			bw(&b, "| ", percent(p.Vitals.Blood), " ")
			bw(&b, "| ", percent(p.Vitals.Health), " ")
			playtime := secondsToHours(p.Stats.PlaytimeSeconds)
			bw(&b, "| ", strconv.Itoa(playtime), " ", plural(playtime, "hour", "hours"), " ")

			bw(&b, "| ")

			if p.Stats.PlayersKilled > 0 {
				bw(&b, strconv.Itoa(p.Stats.PlayersKilled), " ", plural(p.Stats.PlayersKilled, "kill", "kills"), " ")
			} else {
				bw(&b, " - ")
			}

			bw(&b, "|\n")
		}

		var (
			markup  tele.ReplyMarkup
			buttons = make([]tele.Btn, 0, len(playerButtons))
		)

		for _, pb := range playerButtons {
			// payload (callback_data) will contain `<server_name>|<player_steam_id>`
			buttons = append(buttons, markup.Data("👤 "+pb.name, ServerPlayerBtn.Unique, serverName, pb.steamID))
		}

		markup.Inline(markup.Split(3, buttons)...) //nolint:mnd

		// overview map of everyone online; best-effort - skipped on a world/map/render failure
		var media []tele.InputRichMessageMedia

		if wErr != nil {
			log.Error("Failed to fetch world for players map", slog.String("error", wErr.Error()))
		} else if gameMap, gmErr := gamemap.MapByName(world.MapName); gmErr == nil && gameMap != nil {
			points := make([]gamemap.Point, 0, len(*players))
			for _, p := range *players {
				points = append(points, gamemap.Point{PosX: p.Pos.X, PosY: p.Pos.Y, TopLabel: p.ID.Name})
			}

			if img, imgErr := gamemap.DrawPoints(gameMap, points); imgErr == nil {
				// stream the jpeg into the upload without buffering the whole image
				pr := gamemap.EncodeReader(img, func(w io.Writer, m image.Image) error {
					return jpeg.Encode(w, m, &jpeg.Options{Quality: 70}) //nolint:mnd
				})
				defer func() { _ = pr.Close() }()

				bw(&b, "\n\n![](tg://photo?id=players-map)")

				media = append(media, tele.InputRichMessageMedia{
					ID:    "players-map",
					Media: &tele.Photo{File: tele.FromReader(pr)},
				})
			} else {
				log.Error("Failed to render players map", slog.String("map", world.MapName), slog.Any("error", imgErr))
			}
		}

		return c.Send(&tele.InputRichMessage{Markdown: b.String(), Media: media}, &markup)
	}
}
