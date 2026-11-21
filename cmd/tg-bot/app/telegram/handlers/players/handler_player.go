package players

import (
	"context"
	"fmt"
	"image"
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

		// the card is a stack of 2-column markdown tables
		var b strings.Builder

		b.Grow(512) //nolint:mnd // rough estimate of the message size

		// identity
		bw(&b, "| 👤 Nickname  | `", removeBackticks(p.ID.Name), "` |\n")
		bw(&b, "|:-------------|---:|\n")
		bw(&b, "| 🎮 Steam ID  | `", removeBackticks(p.ID.SteamID), "` |\n")
		bw(&b, "| 🆔 GUID      | `", removeBackticks(p.ID.GUID), "` |\n")
		bw(&b,
			"| ", iff(strings.ToLower(strings.TrimSpace(p.Character.Gender)) == "female", "👩🏻", "👨🏻"), " Character | ",
			p.Character.Type, " ("+p.Character.Gender+") |\n",
		)
		avgPing := int(math.Round(float64(p.Network.Ping.Min+p.Network.Ping.Max) / 2))   //nolint:mnd
		bw(&b, "| 🌐 Ping | ", warnIf(avgPing > 200), strconv.Itoa(avgPing), " ms |\n\n") //nolint:mnd

		// vitals - warn glyph on dangerous levels
		bw(&b, "| ❤️ Health | ", warnIf(p.Vitals.Health < 0.5), percent(p.Vitals.Health), "% |\n") //nolint:mnd
		bw(&b, "|:---|---:|\n")
		bw(&b, "| 🩸 Blood (", p.Character.BloodType, ") | ", percent(p.Vitals.Blood), "% |\n")
		bw(&b,
			"| 🩹 Blood Loss | ",
			warnIf(p.State.BleedingSources > 0),
			iff(p.State.BleedingSources > 0, "Ongoing", "None"),
			" |\n",
		)
		bw(&b,
			"| 💧 Water / 🍏 Food | ",
			warnIf(p.Vitals.Water < 0.2 || p.Vitals.Energy < 0.2),
			percent(p.Vitals.Water), "% / ",
			percent(p.Vitals.Energy), "%",
			" |\n",
		)
		bw(&b, "| 💦 Wetness | ", fmt.Sprintf("%.0f", p.Vitals.Wetness), "% |\n")
		bw(&b,
			"| 🧠 Consciousness | (",
			warnIf(p.State.IsUnconscious),
			iff(p.State.IsUnconscious, "unconscious", "conscious"), ") ",
			percent(p.Vitals.Shock), "%",
			" |\n",
		)
		bw(&b, "| ☣️ Toxicity | ", percent(p.Vitals.Toxicity), "% |\n")
		bw(&b, "| 🦠 Pathogens | ", strconv.Itoa(p.State.AgentsActive), " |\n")
		bw(&b, "| 💊 Immunity boosted | ", iff(p.State.ImmunityBoosted, "Yes", "No"), " |\n")
		bw(&b, "| 🌡️ Body temperature | ", fixed(p.Vitals.TemperatureCelsius, 0), " °C", " |\n")
		bw(&b, "| 🎒 Carried weight | ", fixed(p.WeightGrams/1000, 2), " kg |\n") //nolint:mnd
		bw(&b, "| 🔗 Handcuffed | ", iff(p.State.IsRestrained, "Yes", "No"), " |\n\n")

		// position - coordinates link out to an interactive map
		xStr, yStr := fixed(p.Pos.X, 2), fixed(p.Pos.Y, 2) //nolint:mnd
		bw(&b,
			"| 🗺️ Coordinates | ",
			"[", xStr, " / ", yStr, "](https://dayz.xam.nu/", world.MapName, "#location=", xStr, ";", yStr, ";4)",
			" |\n",
		)
		bw(&b, "|:--------------|---:|\n")
		bw(&b, "| 📍 Altitude / Direction | ", fixed(p.Pos.Alt, 2), " m / ", fixed(p.Pos.Angle, 2), " ° |\n\n") //nolint:mnd

		// lifetime stats
		playtime := secondsToHours(p.Stats.PlaytimeSeconds)
		bw(&b,
			"| 📅 Total playtime | ",
			warnIf(playtime < 1), strconv.Itoa(playtime), " ", plural(playtime, "hour", "hours"),
			" |\n",
		)
		bw(&b, "|:-----------------|---:|\n")
		bw(&b, "| 🥾 Distance traveled | ", fixed(p.Stats.DistanceMeters/1000, 2), " km |\n") //nolint:mnd
		bw(&b, "| 🎯 Longest hit | ", fixed(p.Stats.LongestHitMeters, 2), " m |\n")           //nolint:mnd
		bw(&b, "| 🪖 Players killed | ", warnIf(p.Stats.PlayersKilled > 0), strconv.Itoa(p.Stats.PlayersKilled), " |\n")
		bw(&b, "| 🧟 Infected killed | ", strconv.Itoa(p.Stats.InfectedKilled), " |\n\n")

		// append a map crop pinning the player's position
		var media []tele.InputRichMessageMedia

		if gameMap, gmErr := gamemap.MapByName(world.MapName); gmErr == nil && gameMap != nil {
			if img, imgErr := gamemap.DrawPoint(gameMap, gamemap.Point{
				PosX:        p.Pos.X,
				PosY:        p.Pos.Y,
				TopLabel:    p.ID.Name,
				BottomLabel: fmt.Sprintf("[%.2f, %.2f]", p.Pos.X, p.Pos.Y),
			}, gamemap.WithPointsOfInterest(true)); imgErr == nil {
				// stream the jpeg into the upload without buffering the whole image
				pr := gamemap.EncodeReader(img, func(w io.Writer, m image.Image) error {
					return jpeg.Encode(w, m, &jpeg.Options{Quality: 70}) //nolint:mnd
				})
				defer func() { _ = pr.Close() }()

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
