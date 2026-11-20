package telegram

import (
	"maps"
	"slices"
	"strings"

	tele "gopkg.in/telebot.v4"
)

func (h *Handlers) handleManageServersCommand() tele.HandlerFunc {
	return func(c tele.Context) error {
		cfg := h.cfg.Config()

		if len(cfg.Servers) == 0 {
			return c.Send(&tele.InputRichMessage{
				Markdown: "**No servers configured yet**. Use /" + cmdAddServer + " to add one",
			})
		}

		var (
			b      strings.Builder
			markup = &tele.ReplyMarkup{}
			names  = slices.Sorted(maps.Keys(cfg.Servers)) // stable order
			btns   = make([]tele.Btn, 0, len(names))
		)

		b.Grow(64 + (len(cfg.Servers) * 64)) //nolint:mnd // rough estimate

		b.WriteString("| Name | StateZ Base URL |\n")
		b.WriteString("|:----:|:---------------:|\n")

		for _, name := range names {
			b.WriteString("| `")
			b.WriteString(name) // validated on adding and clear
			b.WriteString("` | `")
			b.WriteString(cfg.Servers[name].BaseUrl) // validated on adding and clear
			b.WriteString("` |\n")

			btns = append(btns, markup.Data("🗑️ Remove \""+name+"\"", h.buttons.removeServer.Unique, name))
		}

		markup.Inline(markup.Split(2, btns)...) //nolint:mnd

		return c.Send(&tele.InputRichMessage{Markdown: b.String()}, markup)
	}
}
