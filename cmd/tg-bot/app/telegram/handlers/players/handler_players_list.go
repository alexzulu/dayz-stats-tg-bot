package players

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	tele "gopkg.in/telebot.v4"

	"github.com/alexzulu/dayz-stats-tg-bot/cmd/tg-bot/app/telegram/clients"
	tt "github.com/alexzulu/dayz-stats-tg-bot/cmd/tg-bot/app/telegram/tools"
	"github.com/alexzulu/dayz-stats-tg-bot/cmd/tg-bot/app/telegram/validate"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/config"
)

// ServerPlayersListBtn is the inline button that lists a server's online players; its callback payload is the
// server name.
var ServerPlayersListBtn = tele.Btn{Text: "Players online", Unique: "players"} //nolint:gochecknoglobals

// NewServerPlayersListBtnHandler returns the callback handler for ServerPlayersListBtn. It fetches the server's
// online players, renders a stats table, and attaches a per-player drill-down button (payload
// "<server>|<steam_id>") for every player that has a Steam ID.
func NewServerPlayersListBtnHandler(ctx context.Context, cf *config.File, cp *clients.Pool) tele.HandlerFunc {
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

		reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second) //nolint:mnd
		defer cancel()

		players, cErr := cp.Get(serverConfig).Players(reqCtx)
		if cErr != nil {
			return tt.SendRichMarkdownError(c, "Failed to fetch players", cErr)
		}

		if len(*players) == 0 {
			return tt.SendRichMarkdownError(c, fmt.Sprintf("No players online on server `%s`", serverName), nil)
		}

		// sort players to get stable order
		sort.SliceStable(*players, func(i, j int) bool { return (*players)[i].ID.GUID < (*players)[j].ID.GUID })

		type playerBtn struct{ name, steamID string }

		playerButtons := make([]playerBtn, 0, len(*players))

		var b strings.Builder

		b.Grow(256 + (len(*players) * 64)) //nolint:mnd // rough estimate

		b.WriteString("Players online on server `" + serverName + "`:\n\n")
		b.WriteString("| Nickname | 💧  | 🍏  | 🩸  | ❤️  | Total playtime | Kills |\n")
		b.WriteString("|:--------:|:---:|:---:|:---:|:---:|:--------------:|:-----:|\n")

		// one row per named player; collect a button target for each one that has a steam id
		for _, p := range *players {
			if p.ID.Name == "" {
				continue
			}

			if p.ID.SteamID != "" {
				playerButtons = append(playerButtons, playerBtn{name: p.ID.Name, steamID: p.ID.SteamID})
			}

			b.WriteString("| `")
			b.WriteString(removeBackticks(p.ID.Name))
			b.WriteRune('`')

			if p.State.Godmode {
				b.WriteString(" 👼")
			}

			b.WriteString(" | ")

			b.WriteString(floatToPercentStr(p.Vitals.Water))
			b.WriteString(" | ")

			b.WriteString(floatToPercentStr(p.Vitals.Energy))
			b.WriteString(" | ")

			b.WriteString(floatToPercentStr(p.Vitals.Blood))
			b.WriteString(" | ")

			b.WriteString(floatToPercentStr(p.Vitals.Health))
			b.WriteString(" | ")

			playtime := secondsToHours(p.Stats.PlaytimeSeconds)
			b.WriteString(strconv.Itoa(playtime))
			b.WriteRune(' ')
			b.WriteString(plural(playtime, "hour", "hours"))
			b.WriteString(" | ")

			if p.Stats.PlayersKilled > 0 {
				b.WriteString(strconv.Itoa(p.Stats.PlayersKilled))
				b.WriteRune(' ')
				b.WriteString(plural(p.Stats.PlayersKilled, "kill", "kills"))
				b.WriteRune(' ')
			} else {
				b.WriteString(" - ")
			}

			b.WriteString("|")
			b.WriteRune('\n')
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

		return c.Send(&tele.InputRichMessage{Markdown: b.String()}, &markup)
	}
}
