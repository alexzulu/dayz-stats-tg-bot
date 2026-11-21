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
	"time"

	"gh.tarampamp.am/dayz-statez/server/pkg/api"
	tele "gopkg.in/telebot.v4"

	"github.com/alexzulu/dayz-stats-tg-bot/cmd/tg-bot/app/telegram/clients"
	tt "github.com/alexzulu/dayz-stats-tg-bot/cmd/tg-bot/app/telegram/tools"
	"github.com/alexzulu/dayz-stats-tg-bot/cmd/tg-bot/app/telegram/validate"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/config"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/gamemap"
	gameMaps "github.com/alexzulu/dayz-stats-tg-bot/internal/gamemap/maps"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/gamemap/maps/chernarusplus"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/gamemap/maps/nasdara"
)

// ServerPlayerBtn is the inline button for a single player's details; its callback payload is
// "<server_name>|<player_steam_id>".
var ServerPlayerBtn = tele.Btn{Text: "Player details", Unique: "player"} //nolint:gochecknoglobals

// NewServerPlayerBtnHandler returns the callback handler for ServerPlayerBtn. It renders one player's full
// card (identity, vitals, position, lifetime stats) plus a map crop pinning their current position.
func NewServerPlayerBtnHandler( //nolint:funlen
	ctx context.Context,
	log *slog.Logger,
	cf *config.File,
	cp *clients.Pool,
) tele.HandlerFunc {
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

		// players and world come from the same daemon - fetch them concurrently
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
		case wErr != nil:
			return tt.SendRichMarkdownError(c, "Failed to fetch world info", wErr)
		case pErr != nil:
			return tt.SendRichMarkdownError(c, "Failed to fetch players", pErr)
		case players == nil || world == nil:
			return tt.SendRichMarkdownError(c, "Fucking shit happened", nil)
		case len(*players) == 0:
			return tt.SendRichMarkdownError(c, fmt.Sprintf("No players online on server `%s`", serverName), nil)
		}

		// locate the requested player in the live snapshot - they may have logged off since the list was shown
		playerIdx := func() int {
			for i, p := range *players {
				if p.ID.SteamID == playerSteamID {
					return i
				}
			}

			return -1
		}()

		if playerIdx < 0 {
			return tt.SendRichMarkdownError(c, fmt.Sprintf("Player is no longer online on server `%s`", serverName), nil)
		}

		p := (*players)[playerIdx]

		// the card is a stack of 2-column markdown tables; these local helpers keep each row
		var (
			b  strings.Builder
			bw = func(s ...string) {
				for _, v := range s {
					if v == "" {
						continue
					}

					b.WriteString(v)
				}
			}
			warnIf = func(condition bool) string {
				if condition {
					return " ⚠️ "
				}

				return ""
			}
			trnry = func(condition bool, trueVal, falseVal string) string {
				if condition {
					return trueVal
				}

				return falseVal
			}
		)

		b.Grow(512) //nolint:mnd // rough estimate of the message size

		// identity
		bw("| 👤 Nickname  | `", removeBackticks(p.ID.Name), "` |\n")
		bw("|:-------------|---:|\n")
		bw("| 🎮 Steam ID  | `", removeBackticks(p.ID.SteamID), "` |\n")
		bw("| 🆔 GUID      | `", removeBackticks(p.ID.GUID), "` |\n")
		bw(
			"| ", trnry(strings.ToLower(strings.TrimSpace(p.Character.Gender)) == "female", "👩🏻", "👨🏻"), " Character | ",
			p.Character.Type, " ("+p.Character.Gender+") |\n",
		)
		avgPing := int(math.Round(float64(p.Network.Ping.Min+p.Network.Ping.Max) / 2)) //nolint:mnd
		bw("| 🌐 Ping | ", warnIf(avgPing > 200), strconv.Itoa(avgPing), " ms |\n\n")   //nolint:mnd

		// vitals - warn glyph on dangerous levels
		bw("| ❤️ Health | ", warnIf(p.Vitals.Health < 0.5), floatToPercentStr(p.Vitals.Health), "% |\n") //nolint:mnd
		bw("|:---|---:|\n")
		bw("| 🩸 Blood (", p.Character.BloodType, ") | ", floatToPercentStr(p.Vitals.Blood), "% |\n")
		bw(
			"| 🩹 Blood Loss | ",
			warnIf(p.State.BleedingSources > 0),
			trnry(p.State.BleedingSources > 0, "Ongoing", "None"),
			" |\n",
		)
		bw(
			"| 💧 Water / 🍏 Food | ",
			warnIf(p.Vitals.Water < 0.2 || p.Vitals.Energy < 0.2),
			floatToPercentStr(p.Vitals.Water), "% / ",
			floatToPercentStr(p.Vitals.Energy), "%",
			" |\n",
		)
		bw("| 💦 Wetness | ", fmt.Sprintf("%.0f", p.Vitals.Wetness), "% |\n")
		bw(
			"| 🧠 Consciousness | (",
			warnIf(p.State.IsUnconscious),
			trnry(p.State.IsUnconscious, "unconscious", "conscious"), ") ",
			floatToPercentStr(p.Vitals.Shock), "%",
			" |\n",
		)
		bw("| ☣️ Toxicity | ", floatToPercentStr(p.Vitals.Toxicity), "% |\n")
		bw("| 🦠 Pathogens | ", strconv.Itoa(p.State.AgentsActive), " |\n")
		bw("| 💊 Immunity boosted | ", trnry(p.State.ImmunityBoosted, "Yes", "No"), " |\n")
		bw("| 🌡️ Body temperature | ", floatToIntStr(p.Vitals.TemperatureCelsius), " °C", " |\n")
		bw("| 🎒 Carried weight | ", floatRound2ToStr(p.WeightGrams/1000), " kg |\n") //nolint:mnd
		bw("| 🔗 Handcuffed | ", trnry(p.State.IsRestrained, "Yes", "No"), " |\n\n")

		// position - coordinates link out to an interactive map
		xStr, yStr := floatRound2ToStr(p.Pos.X), floatRound2ToStr(p.Pos.Y)
		bw(
			"| 🗺️ Coordinates | ",
			"[", xStr, " / ", yStr, "](https://dayz.xam.nu/", world.MapName, "#location=", xStr, ";", yStr, ";4)",
			" |\n",
		)
		bw("|:--------------|---:|\n")
		bw("| 📍 Altitude / Direction | ", floatRound2ToStr(p.Pos.Alt), " m / ", floatRound2ToStr(p.Pos.Angle), " ° |\n\n")

		// lifetime stats
		playtime := secondsToHours(p.Stats.PlaytimeSeconds)
		bw(
			"| 📅 Total playtime | ",
			warnIf(playtime < 1), strconv.Itoa(playtime), " ", plural(playtime, "hour", "hours"),
			" |\n",
		)
		bw("|:-----------------|---:|\n")
		bw("| 🥾 Distance traveled | ", floatRound2ToStr(p.Stats.DistanceMeters/1000), " km |\n") //nolint:mnd
		bw("| 🎯 Longest hit | ", floatRound2ToStr(p.Stats.LongestHitMeters), " m |\n")
		bw("| 🪖 Players killed | ", warnIf(p.Stats.PlayersKilled > 0), strconv.Itoa(p.Stats.PlayersKilled), " |\n")
		bw("| 🧟 Infected killed | ", strconv.Itoa(p.Stats.InfectedKilled), " |\n\n")

		// append a map crop pinning the player's position; best-effort - an unsupported map or a render
		// error just drops the image and the card is still sent
		var media []tele.InputRichMessageMedia

		if gameMap, gmErr := getMapByName(world.MapName); gmErr == nil && gameMap != nil {
			if img, imgErr := gamemap.DrawPoint(gameMap, gamemap.Point{
				PosX:        p.Pos.X,
				PosY:        p.Pos.Y,
				TopLabel:    p.ID.Name,
				BottomLabel: fmt.Sprintf("[%.2f, %.2f]", p.Pos.X, p.Pos.Y),
			}, gamemap.WithPointsOfInterest(true)); imgErr == nil {
				// stream the jpeg into the upload through a pipe instead of buffering the whole image
				pr, pw := io.Pipe()
				defer func() { _ = pr.Close() }()

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

// getMapByName resolves a StateZ world name to its embedded game map, or an error for an unsupported map.
func getMapByName(mapName string) (gameMaps.Map, error) {
	// TODO: when in 2nd place the same code is repeated, consider refactoring it into a separate function
	//       to avoid duplication
	switch mapName {
	case chernarusplus.Map.Name():
		return chernarusplus.Map, nil
	case nasdara.Map.Name():
		return nasdara.Map, nil
	default:
		return nil, fmt.Errorf("unsupported map %s", mapName)
	}
}
