package players

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	tele "gopkg.in/telebot.v4"

	"github.com/alexzulu/dayz-stats-tg-bot/cmd/tg-bot/app/telegram/clients"
	tt "github.com/alexzulu/dayz-stats-tg-bot/cmd/tg-bot/app/telegram/tools"
	"github.com/alexzulu/dayz-stats-tg-bot/cmd/tg-bot/app/telegram/validate"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/config"
)

var ServerPlayersListBtn = tele.Btn{Text: "Players online", Unique: "players"}

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

		players, cErr := cp.Get(serverConfig).Players(ctx)
		if cErr != nil {
			return tt.SendRichMarkdownError(c, "Failed to fetch players", cErr)
		}

		if len(*players) == 0 {
			return tt.SendRichMarkdownError(c, fmt.Sprintf("No players online on server `%s`", serverName), nil)
		}

		// sort players to get stable order
		sort.SliceStable(*players, func(i, j int) bool { return (*players)[i].ID.GUID < (*players)[j].ID.GUID })

		var (
			names      = make([]string, 0, len(*players))
			namesToIDs = make(map[string]string, len(*players))

			b strings.Builder
		)

		b.Grow(256 + (len(*players) * 64)) //nolint:mnd // rough estimate

		b.WriteString(fmt.Sprintf("Players online on server `%s`:\n\n", serverName))
		b.WriteString("| Nickname | 💧  | 🍏  | 🩸  | ❤️  | Total playtime | Kills |\n")
		b.WriteString("|:--------:|:---:|:---:|:---:|:---:|:--------------:|:-----:|\n")

		for _, p := range *players {
			names, namesToIDs[p.ID.Name] = append(names, p.ID.Name), p.ID.SteamID

			b.WriteRune('`')
			b.WriteString(strings.ReplaceAll(p.ID.Name, "`", ""))
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

			playtimeDays := secondsToHours(p.Stats.PlaytimeSeconds)
			b.WriteString(strconv.Itoa(playtimeDays))
			b.WriteRune(' ')
			b.WriteString(plural(playtimeDays, "hour", "hours"))
			b.WriteString(" | ")

			if p.Stats.PlayersKilled > 0 {
				b.WriteString(strconv.Itoa(p.Stats.PlayersKilled))
				b.WriteRune(' ')
				b.WriteString(plural(p.Stats.PlayersKilled, "kill", "kills"))
				b.WriteRune(' ')
			} else {
				b.WriteString(" - ")
			}

			b.WriteRune('\n')
		}

		var (
			markup  tele.ReplyMarkup
			buttons = make([]tele.Btn, 0, len(names))
		)

		for _, name := range names {
			// payload (callback_data) will contain `<server_name>|<player_steam_id>`
			buttons = append(buttons, markup.Data("👤 "+name, ServerPlayerBtn.Unique, serverName, namesToIDs[name]))
		}

		markup.Inline(markup.Split(3, buttons)...) //nolint:mnd

		return c.Send(&tele.InputRichMessage{Markdown: b.String()}, &markup)
	}
}

func floatToPercentStr(f float64) string { return strconv.Itoa(int(math.Round(f * 100))) } //nolint:mnd

func secondsToHours(seconds float64) int { return int(math.Round(seconds / 3600)) } //nolint:mnd

func plural[T int | float64](value T, singular, plural string) string {
	if value == 1 {
		return singular
	}

	return plural
}
