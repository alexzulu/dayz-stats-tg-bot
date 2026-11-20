package servers

import (
	"fmt"
	"log/slog"

	tele "gopkg.in/telebot.v4"

	"github.com/alexzulu/dayz-stats-tg-bot/cmd/tg-bot/app/telegram/clients"
	tt "github.com/alexzulu/dayz-stats-tg-bot/cmd/tg-bot/app/telegram/tools"
	"github.com/alexzulu/dayz-stats-tg-bot/cmd/tg-bot/app/telegram/validate"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/config"
)

var RemoveServerBtn = tele.Btn{Text: "Remove server", Unique: "delsrv"}

func NewRemoveServerBtnHandler(log *slog.Logger, cf *config.File, cp *clients.Pool) tele.HandlerFunc {
	return func(c tele.Context) error {
		serverName := c.Data() // received from [RemoveServerBtn] button via `callback_data`

		if err := validate.ServerName(serverName); err != nil {
			return tt.SendRichMarkdownError(c, "Invalid server name", err)
		}

		var (
			found        bool
			serverConfig config.ConfigServer
		)

		cf.Update(func(cfg *config.Config) {
			serverConfig, found = cfg.Servers[serverName]
			if !found {
				return
			}

			delete(cfg.Servers, serverName)
		})

		if !found {
			return tt.SendRichMarkdownError(c, fmt.Sprintf("Server `%s` does not exist", serverName), nil)
		}

		if err := cf.Save(); err != nil {
			// restore in-memory config to match disk
			cf.Update(func(cfg *config.Config) { cfg.Servers[serverName] = serverConfig })

			log.Error("failed to save config after removing server",
				slog.String("error", err.Error()),
				slog.String("server_name", serverName),
			)

			return tt.SendRichMarkdownError(c, "Failed to save the config", err)
		}

		cp.Forget(serverConfig)

		return tt.SendRichMarkdown(c, fmt.Sprintf("Server `%s` removed successfully", serverName))
	}
}
