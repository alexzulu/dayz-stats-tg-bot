package players

import (
	"context"
	"fmt"
	"image/jpeg"
	"io"
	"log/slog"
	"strings"
	"sync"

	"gh.tarampamp.am/dayz-statez/server/pkg/api"
	tele "gopkg.in/telebot.v4"

	"github.com/alexzulu/dayz-stats-tg-bot/cmd/tg-bot/app/telegram/clients"
	tt "github.com/alexzulu/dayz-stats-tg-bot/cmd/tg-bot/app/telegram/tools"
	"github.com/alexzulu/dayz-stats-tg-bot/cmd/tg-bot/app/telegram/validate"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/config"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/gamemap"
	gameMaps "github.com/alexzulu/dayz-stats-tg-bot/internal/gamemap/maps"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/gamemap/maps/chernarusplus"
)

var ServerPlayerBtn = tele.Btn{Text: "Player details", Unique: "player"}

func NewServerPlayerBtnHandler(ctx context.Context, log *slog.Logger, cf *config.File, cp *clients.Pool) tele.HandlerFunc {
	return func(c tele.Context) error {
		args := c.Args()
		if len(args) != 2 { //nolint:mnd
			return tt.SendRichMarkdownError(c, "Invalid callback data", nil)
		}

		serverName, playerSteamID := args[0], args[1] // received from [ServerPlayerBtn] button via `callback_data`

		if err := validate.ServerName(serverName); err != nil {
			return tt.SendRichMarkdownError(c, "Invalid server name", err)
		}

		cfg := cf.Config()

		serverConfig, exists := cfg.Servers[serverName]
		if !exists {
			return tt.SendRichMarkdownError(c, fmt.Sprintf("Server `%s` does not exist", serverName), nil)
		}

		var (
			client = cp.Get(serverConfig)
			wg     sync.WaitGroup

			players *api.Players
			pErr    error
			world   *api.World
			wErr    error
		)

		wg.Go(func() { players, pErr = client.Players(ctx) })
		wg.Go(func() { world, wErr = client.World(ctx) })

		wg.Wait()

		switch {
		case wErr != nil:
			return tt.SendRichMarkdownError(c, "Failed to fetch world info", pErr)
		case pErr != nil:
			return tt.SendRichMarkdownError(c, "Failed to fetch players", pErr)
		case players == nil || world == nil:
			return tt.SendRichMarkdownError(c, "Fucking shit happened", nil)
		case len(*players) == 0:
			return tt.SendRichMarkdownError(c, fmt.Sprintf("No players online on server `%s`", serverName), nil)
		}

		playerIdx := func() int {
			for i, p := range *players {
				if p.ID.SteamID == playerSteamID {
					return i
				}
			}

			return -1
		}()

		if playerIdx < 0 || playerIdx >= len(*players) {
			return tt.SendRichMarkdownError(c, fmt.Sprintf("Player is no longer online on server `%s`", serverName), nil)
		}

		playerData := (*players)[playerIdx]

		// plain text message with player info
		var b strings.Builder // TODO: fill all the fields with player info (health, blood, food, water, playtime, kills, etc.)

		b.WriteString("**Nickname**:\t")
		b.WriteString(playerData.ID.Name)
		b.WriteString("\n")
		b.WriteString("- SteamID:\t")
		b.WriteString(playerData.ID.SteamID)
		b.WriteString("\n")
		b.WriteString("- GUID:\t\t")
		b.WriteString(playerData.ID.GUID)
		b.WriteString("\n")
		b.WriteString("- Position:\t")
		b.WriteString(fmt.Sprintf("[%.2f, %.2f]", playerData.Pos.X, playerData.Pos.Y))

		var media []tele.InputRichMessageMedia

		if gameMap, gmErr := getMapByName(world.MapName); gmErr == nil && gameMap != nil {
			if img, imgErr := gamemap.DrawPoint(gameMap, gamemap.Point{
				PosX:        playerData.Pos.X,
				PosY:        playerData.Pos.Y,
				TopLabel:    playerData.ID.Name,
				BottomLabel: fmt.Sprintf("[%.2f, %.2f]", playerData.Pos.X, playerData.Pos.Y),
			}); imgErr == nil {
				pr, pw := io.Pipe()
				defer func() { _ = pr.Close() }() //nolint:errcheck

				go func() { _ = pw.CloseWithError(jpeg.Encode(pw, img, &jpeg.Options{Quality: 90})) }() //nolint:mnd

				b.WriteString("\n\n![](tg://photo?id=player-map)")
				media = append(media, tele.InputRichMessageMedia{
					ID:    "player-map",
					Media: &tele.Photo{File: tele.FromReader(pr)},
				})
			} else {
				log.Error("Failed to render player map",
					slog.String("map", world.MapName),
					slog.Any("error", imgErr),
				)
			}
		}

		return c.Send(&tele.InputRichMessage{Markdown: b.String(), Media: media})
	}
}

func getMapByName(mapName string) (gameMaps.Map, error) {
	// TODO: when in 2nd place the same code is repeated, consider refactoring it into a separate function
	//       to avoid duplication
	switch mapName {
	case chernarusplus.Map.Name():
		return chernarusplus.Map, nil
	default:
		return nil, fmt.Errorf("unsupported map %s", mapName)
	}
}
