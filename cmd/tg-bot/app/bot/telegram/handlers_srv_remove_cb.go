package telegram

import (
	"fmt"
	"log/slog"

	tele "gopkg.in/telebot.v4"

	"github.com/alexzulu/dayz-stats-tg-bot/internal/config"
)

func (h *Handlers) handleRemoveServerCallback() tele.HandlerFunc {
	return func(c tele.Context) error {
		serverName := c.Data()

		if err := ValidateServerName(serverName); err != nil {
			return c.Send(&tele.InputRichMessage{
				Markdown:            "**Invalid server name**: " + EscapeMarkdown(err.Error()),
				SkipEntityDetection: true,
			})
		}

		var (
			found        bool
			serverConfig config.ConfigServer
		)

		h.cfg.Update(func(cfg *config.Config) {
			serverConfig, found = cfg.Servers[serverName]
			if !found {
				return
			}

			delete(cfg.Servers, serverName)
		})

		if !found {
			return c.Send(&tele.InputRichMessage{
				Markdown:            fmt.Sprintf("**Server `%s` does not exist**", serverName),
				SkipEntityDetection: true,
			})
		}

		if err := h.cfg.Save(); err != nil {
			// restore in-memory config to match disk
			h.cfg.Update(func(cfg *config.Config) { cfg.Servers[serverName] = serverConfig })

			h.log.Error("failed to save config after removing server",
				slog.String("error", err.Error()),
				slog.String("server_name", serverName),
			)

			return c.Send(&tele.InputRichMessage{
				Markdown:            "**Failed to save the config**: " + EscapeMarkdown(err.Error()),
				SkipEntityDetection: true,
			})
		}

		h.forgetClientForServer(serverConfig)

		return c.Send(&tele.InputRichMessage{
			Markdown:            fmt.Sprintf("**Server `%s` removed successfully**", serverName),
			SkipEntityDetection: true,
		})
	}
}
