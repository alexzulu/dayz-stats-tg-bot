package players

import (
	"context"
	"fmt"
	"image/jpeg"
	"io"
	"log/slog"
	"math"
	"strconv"
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

		var b strings.Builder

		b.Grow(512) //nolint:mnd // rough estimate of the message size

		b.WriteString("| Nickname | `")
		b.WriteString(removeBackticks(playerData.ID.Name))
		b.WriteString("` |\n")
		b.WriteString("|:---|---:|\n")
		b.WriteString("| Steam ID | `")
		b.WriteString(removeBackticks(playerData.ID.SteamID))
		b.WriteString("` |\n")
		b.WriteString("| GUID | `")
		b.WriteString(removeBackticks(playerData.ID.GUID))
		b.WriteString("` |\n\n")

		b.WriteString("| Character type | ")
		b.WriteString(playerData.Character.Type)
		b.WriteString(" |\n")
		b.WriteString("|:---|---:|\n")
		b.WriteString("| Gender | ")
		b.WriteString(playerData.Character.Gender)
		b.WriteString(" |\n")
		b.WriteString("| Blood type | ")
		b.WriteString(playerData.Character.BloodType)
		b.WriteString(" |\n\n")

		b.WriteString("| ❤️ Health | ")
		if playerData.Vitals.Health < 0.5 { //nolint:mnd
			b.WriteString("⚠️ ")
		}
		b.WriteString(floatToPercentStr(playerData.Vitals.Health))
		b.WriteString("% |\n")
		b.WriteString("|:---|---:|\n")
		b.WriteString("| 💧 Water | ")
		if playerData.Vitals.Water < 0.2 { //nolint:mnd
			b.WriteString("⚠️ ")
		}
		b.WriteString(floatToPercentStr(playerData.Vitals.Water))
		b.WriteString("% |\n")
		b.WriteString("| 🍏 Food | ")
		if playerData.Vitals.Energy < 0.2 { //nolint:mnd
			b.WriteString("⚠️ ")
		}
		b.WriteString(floatToPercentStr(playerData.Vitals.Energy))
		b.WriteString("% |\n")
		b.WriteString("| 🩸 Blood | ")
		b.WriteString(floatToPercentStr(playerData.Vitals.Blood))
		b.WriteString("% |\n")
		b.WriteString("| Bleeding status | ")
		if playerData.State.BleedingSources > 0 {
			b.WriteString("⚠️ Bleeding is present")
		} else {
			b.WriteString("No bleeding")
		}
		b.WriteString(" |\n")
		b.WriteString("| ⚡ Shock | ")
		b.WriteString(floatToPercentStr(playerData.Vitals.Shock))
		b.WriteString("% |\n")
		b.WriteString("| ☣️ Toxicity | ")
		b.WriteString(floatToPercentStr(playerData.Vitals.Toxicity))
		b.WriteString("% |\n")
		b.WriteString("| 🦠 Pathogens | ")
		b.WriteString(strconv.Itoa(playerData.State.AgentsActive))
		b.WriteString("% |\n")
		b.WriteString("| 💊 Immunity boosted | ")
		if playerData.State.ImmunityBoosted {
			b.WriteString("Yes")
		} else {
			b.WriteString("No")
		}
		b.WriteString(" |\n")
		b.WriteString("| 💦 Wetness | ")
		b.WriteString(fmt.Sprintf("%.0f", playerData.Vitals.Wetness))
		b.WriteString("% |\n")
		b.WriteString("| 🌡️ Body temperature | ")
		b.WriteString(fmt.Sprintf("%.2f °C", playerData.Vitals.TemperatureCelsius))
		b.WriteString(" |\n")
		b.WriteString("| ⚖️ Carried weight | ")
		b.WriteString(fmt.Sprintf("%.2f kg", playerData.WeightGrams/1000)) //nolint:mnd
		b.WriteString(" |\n")
		b.WriteString("| In conscious | ")
		if playerData.State.IsUnconscious {
			b.WriteString("⚠️ Yes")
		} else {
			b.WriteString("No")
		}
		b.WriteString(" |\n")
		b.WriteString("| Handcuffed | ")
		if playerData.State.IsRestrained {
			b.WriteString("⚠️ Yes")
		} else {
			b.WriteString("No")
		}
		b.WriteString(" |\n\n")

		b.WriteString("| Coordinates | ")
		b.WriteString(fmt.Sprintf("x:%.2f, y:%.2f", playerData.Pos.X, playerData.Pos.Y))
		b.WriteString(" |\n")
		b.WriteString("|:---|---:|\n")
		b.WriteString("| Altitude | ")
		b.WriteString(fmt.Sprintf("%.2f m", playerData.Pos.Alt))
		b.WriteString(" |\n")
		b.WriteString("| Direction | ")
		b.WriteString(fmt.Sprintf("%.2f °", playerData.Pos.Angle))
		b.WriteString(" |\n\n")

		b.WriteString("| Average ping | ")
		avgPing := int(math.Round(float64(playerData.Network.Ping.Min+playerData.Network.Ping.Max) / 2))
		if avgPing > 200 { //nolint:mnd
			b.WriteString("⚠️ ")
		}
		b.WriteString(strconv.Itoa(avgPing))
		b.WriteString(" ms |\n")
		b.WriteString("|:---|---:|\n")
		b.WriteString("| Minimum ping | ")
		b.WriteString(strconv.Itoa(playerData.Network.Ping.Min))
		b.WriteString(" ms |\n")
		b.WriteString("| Maximum ping | ")
		b.WriteString(strconv.Itoa(playerData.Network.Ping.Max))
		b.WriteString(" ms |\n\n")

		b.WriteString("| Total playtime | ")
		b.WriteString(strconv.Itoa(secondsToHours(playerData.Stats.PlaytimeSeconds)))
		b.WriteRune(' ')
		b.WriteString(plural(secondsToHours(playerData.Stats.PlaytimeSeconds), "hour", "hours"))
		b.WriteString(" |\n")
		b.WriteString("|:---|---:|\n")
		b.WriteString("| Distance traveled | ")
		b.WriteString(fmt.Sprintf("%.2f km", playerData.Stats.DistanceMeters/1000)) //nolint:mnd
		b.WriteString(" |\n")
		b.WriteString("| Longest hit | ")
		b.WriteString(fmt.Sprintf("%.2f m", playerData.Stats.LongestHitMeters))
		b.WriteString(" |\n")
		b.WriteString("| Players killed | ")
		if playerData.Stats.PlayersKilled > 0 {
			b.WriteString("⚠️ ")
		}
		b.WriteString(strconv.Itoa(playerData.Stats.PlayersKilled))
		b.WriteString(" |\n")
		b.WriteString("| Infected killed | ")
		b.WriteString(strconv.Itoa(playerData.Stats.InfectedKilled))
		b.WriteString(" |\n\n")

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

				go func() { _ = pw.CloseWithError(jpeg.Encode(pw, img, &jpeg.Options{Quality: 70})) }() //nolint:mnd

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
