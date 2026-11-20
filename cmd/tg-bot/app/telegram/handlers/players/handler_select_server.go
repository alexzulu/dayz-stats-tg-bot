package players

import (
	"maps"
	"slices"

	tele "gopkg.in/telebot.v4"

	tt "github.com/alexzulu/dayz-stats-tg-bot/cmd/tg-bot/app/telegram/tools"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/config"
)

func NewSelectServerHandler(cf *config.File) tele.HandlerFunc {
	return func(c tele.Context) error {
		cfg := cf.Config()
		if len(cfg.Servers) == 0 {
			return tt.SendRichMarkdownError(c, "No servers configured yet", nil)
		}

		var (
			markup  tele.ReplyMarkup
			names   = slices.Sorted(maps.Keys(cfg.Servers)) // stable order
			buttons = make([]tele.Btn, 0, len(names))
		)

		for _, name := range names {
			buttons = append(buttons, markup.Data("🗺️ "+name, ServerPlayersListBtn.Unique, name))
		}

		markup.Inline(markup.Split(2, buttons)...) //nolint:mnd

		return c.Send(&tele.InputRichMessage{Markdown: "Select a server to list its players online:"}, &markup)
	}
}
