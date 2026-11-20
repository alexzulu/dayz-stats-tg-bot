package telegram

import (
	"maps"
	"slices"

	tele "gopkg.in/telebot.v4"
)

func (h *Handlers) handlePlayersCommand() tele.HandlerFunc {
	return func(c tele.Context) error {
		cfg := h.cfg.Config()

		if len(cfg.Servers) == 0 {
			return c.Send(&tele.InputRichMessage{
				Markdown: "**No servers configured yet**. Use /" + cmdAddServer + " to add one",
			})
		}

		var (
			markup = &tele.ReplyMarkup{}
			names  = slices.Sorted(maps.Keys(cfg.Servers)) // stable order
			btns   = make([]tele.Btn, 0, len(names))
		)

		for _, name := range names {
			btns = append(btns, markup.Data("🗺️ "+name, h.buttons.players.Unique, name))
		}

		markup.Inline(markup.Split(2, btns)...) //nolint:mnd

		return c.Send(&tele.InputRichMessage{Markdown: "Select a server to list its players online:"}, markup)
	}
}
