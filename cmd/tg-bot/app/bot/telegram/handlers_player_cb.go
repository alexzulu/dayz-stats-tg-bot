package telegram

import (
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"sync"

	"gh.tarampamp.am/dayz-statez/server/pkg/api"
	tele "gopkg.in/telebot.v4"

	"github.com/alexzulu/dayz-stats-tg-bot/internal/gamemap"
	gameMaps "github.com/alexzulu/dayz-stats-tg-bot/internal/gamemap/maps"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/gamemap/maps/chernarusplus"
)

func (h *Handlers) handlePlayerCallback(ctx context.Context) tele.HandlerFunc {
	return func(c tele.Context) error {
		args := c.Args()
		if len(args) != 2 { //nolint:mnd
			return c.Send(&tele.InputRichMessage{Markdown: "**Invalid callback data**"})
		}

		serverName, playerSteamID := args[0], args[1]

		if err := ValidateServerName(serverName); err != nil {
			return c.Send(&tele.InputRichMessage{
				Markdown:            "**Invalid server name**: " + EscapeMarkdown(err.Error()),
				SkipEntityDetection: true,
			})
		}

		cfg := h.cfg.Config()

		serverConfig, exists := cfg.Servers[serverName]
		if !exists {
			return c.Send(&tele.InputRichMessage{
				Markdown:            fmt.Sprintf("**Server `%s` does not exist**", serverName),
				SkipEntityDetection: true,
			})
		}

		var (
			client = h.getClientForServer(serverConfig)
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
		case pErr != nil:
			return c.Send(&tele.InputRichMessage{
				Markdown:            "**Failed to fetch world info**: " + EscapeMarkdown(wErr.Error()),
				SkipEntityDetection: true,
			})
		case wErr != nil:
			return c.Send(&tele.InputRichMessage{
				Markdown:            "**Failed to fetch players**: " + EscapeMarkdown(pErr.Error()),
				SkipEntityDetection: true,
			})
		case players == nil || world == nil:
			return c.Send(&tele.InputRichMessage{Markdown: "**Fucking shit happened**"})
		}

		if len(*players) == 0 {
			return c.Send(&tele.InputRichMessage{
				Markdown: fmt.Sprintf("**No players online on server `%s`**", serverName),
			})
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
			return c.Send(&tele.InputRichMessage{
				Markdown: fmt.Sprintf("**Player is no longer online on server `%s`**", serverName),
			})
		}

		playerData := (*players)[playerIdx]

		playerMap, mErr := func(mapName string) (image.Image, error) {
			var dayzMap gameMaps.Map

			switch mapName {
			case chernarusplus.Map.Name():
				dayzMap = chernarusplus.Map
			default:
				return nil, fmt.Errorf("unsupported map %s", mapName)
			}

			return gamemap.DrawPoint(dayzMap, gamemap.Point{
				PosX:        playerData.Pos.X,
				PosY:        playerData.Pos.Y,
				TopLabel:    playerData.ID.Name,
				BottomLabel: fmt.Sprintf("[%.2f, %.2f]", playerData.Pos.X, playerData.Pos.Y),
			})
		}(world.MapName)
		if mErr != nil {
			return c.Send(&tele.InputRichMessage{
				Markdown:            "**Failed to render player map**: " + EscapeMarkdown(mErr.Error()),
				SkipEntityDetection: true,
			})
		}

		pr, pw := io.Pipe()
		defer func() { _ = pr.Close() }() //nolint:errcheck

		go func() { _ = pw.CloseWithError(jpeg.Encode(pw, playerMap, &jpeg.Options{Quality: 90})) }() //nolint:mnd

		return c.Send(&tele.Photo{
			File:    tele.FromReader(pr),
			Caption: fmt.Sprintf("%+v", playerData),
		})
	}
}
