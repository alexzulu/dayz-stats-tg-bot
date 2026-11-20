package telegram

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	tele "gopkg.in/telebot.v4"
)

func (h *Handlers) handlePlayersCallback(ctx context.Context) tele.HandlerFunc {
	return func(c tele.Context) error {
		serverName := c.Data()

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

		client := h.getClientForServer(serverConfig)

		players, cErr := client.Players(ctx)
		if cErr != nil {
			return c.Send(&tele.InputRichMessage{
				Markdown:            "**Failed to fetch players**: " + EscapeMarkdown(cErr.Error()),
				SkipEntityDetection: true,
			})
		}

		if len(*players) == 0 {
			return c.Send(&tele.InputRichMessage{
				Markdown: fmt.Sprintf("**No players online on server `%s`**", serverName),
			})
		}

		// sort players to get stable order
		sort.SliceStable(*players, func(i, j int) bool { return (*players)[i].ID.GUID < (*players)[j].ID.GUID })

		var (
			b          strings.Builder
			names      = make([]string, 0, len(*players))
			namesToIDs = make(map[string]string, len(*players))
		)

		b.Grow(64 + (len(*players) * 64)) //nolint:mnd // rough estimate

		b.WriteString("| Nickname | 💧  | 🍏  | 🩸  | ❤️  | Playtime | Kills |\n")
		b.WriteString("|:--------:|:---:|:---:|:---:|:---:|:--------:|:-----:|\n")

		for _, p := range *players {
			names, namesToIDs[p.ID.Name] = append(names, p.ID.Name), p.ID.SteamID

			b.WriteRune('`')
			b.WriteString(p.ID.Name)
			b.WriteRune('`')
			if p.State.Godmode {
				b.WriteString(" 👿")
			}
			b.WriteString(" | ")

			b.WriteString(strconv.Itoa(int(math.Round(p.Vitals.Water * 100)))) //nolint:mnd
			b.WriteString(" | ")

			b.WriteString(strconv.Itoa(int(math.Round(p.Vitals.Energy * 100)))) //nolint:mnd
			b.WriteString(" | ")

			b.WriteString(strconv.Itoa(int(math.Round(p.Vitals.Blood * 100)))) //nolint:mnd
			b.WriteString(" | ")

			b.WriteString(strconv.Itoa(int(math.Round(p.Vitals.Health * 100)))) //nolint:mnd
			b.WriteString(" | ")

			playtimeDays := p.Stats.PlaytimeSeconds / (60 * 60) //nolint:mnd
			b.WriteString(strconv.Itoa(int(playtimeDays)))
			b.WriteString(" hour")
			if playtimeDays != 1 {
				b.WriteString("s")
			}
			b.WriteString(" | ")

			if p.Stats.PlayersKilled > 0 {
				b.WriteString(strconv.Itoa(p.Stats.PlayersKilled))
				b.WriteString(" kill")
				if p.Stats.PlayersKilled != 1 {
					b.WriteRune('s')
				}
				b.WriteRune(' ')
			} else {
				b.WriteString(" - ")
			}

			b.WriteRune('\n')
		}

		var (
			markup = &tele.ReplyMarkup{}
			btns   = make([]tele.Btn, 0, len(names))
		)

		for _, name := range names {
			// payload (callback_data) will contain `<server_name>|<player_steam_id>`
			btns = append(btns, markup.Data("👤 "+name, h.buttons.player.Unique, serverName, namesToIDs[name]))
		}

		markup.Inline(markup.Split(3, btns)...) //nolint:mnd

		return c.Send(&tele.InputRichMessage{Markdown: b.String()}, markup)
	}
}
