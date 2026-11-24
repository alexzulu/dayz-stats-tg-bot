package servers

import (
	"maps"
	"slices"
	"strings"

	tele "gopkg.in/telebot.v4"

	tt "github.com/alexzulu/dayz-stats-tg-bot/cmd/tg-bot/app/telegram/tools"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/config"
)

// NewManageServersHandler returns the /servers handler - a table of configured servers, each row paired with a
// "Remove" inline button that carries the server name as its callback payload.
func NewManageServersHandler(cf *config.File) tele.HandlerFunc {
	return func(c tele.Context) error {
		cfg := cf.Config()

		if len(cfg.Servers) == 0 {
			return tt.SendRichMarkdownError(c, "No servers configured yet", nil)
		}

		var (
			markup  tele.ReplyMarkup
			names   = slices.Sorted(maps.Keys(cfg.Servers)) // stable order
			buttons = make([]tele.Btn, 0, len(names))

			b strings.Builder
		)

		b.Grow(64 + (len(cfg.Servers) * 64)) //nolint:mnd // rough estimate

		b.WriteString("| Name | StateZ Base URL |\n")
		b.WriteString("|:----:|:---------------:|\n")

		// one table row and one remove button per server
		for _, serverName := range names {
			b.WriteString("| `")
			b.WriteString(serverName) // validated on adding
			b.WriteString("` | `")
			b.WriteString(cfg.Servers[serverName].BaseUrl) // validated on adding
			b.WriteString("` |\n")

			buttons = append(buttons, markup.Data("🗑️ Remove \""+serverName+"\"", RemoveServerBtn.Unique, serverName))
		}

		markup.Inline(markup.Split(2, buttons)...) //nolint:mnd

		return c.Send(&tele.InputRichMessage{Markdown: b.String()}, &markup)
	}
}
