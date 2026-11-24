package servers

import (
	"fmt"
	"log/slog"
	"slices"

	tele "gopkg.in/telebot.v4"

	tt "github.com/alexzulu/dayz-stats-tg-bot/cmd/tg-bot/app/telegram/tools"
	"github.com/alexzulu/dayz-stats-tg-bot/cmd/tg-bot/app/telegram/validate"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/config"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/statez"
)

// RemoveServerBtn is the inline "Remove" button rendered by /servers; its callback payload is the name of the
// server to delete.
var RemoveServerBtn = tele.Btn{Text: "Remove server", Unique: "delsrv"} //nolint:gochecknoglobals

// NewRemoveServerBtnHandler returns the callback handler for RemoveServerBtn. It removes the server named in the
// payload.
func NewRemoveServerBtnHandler(log *slog.Logger, cf *config.File, cp *statez.CachedClientPool) tele.HandlerFunc {
	return func(c tele.Context) error {
		serverName := c.Data() // received from [RemoveServerBtn] button via `callback_data`

		if err := validate.ServerName(serverName); err != nil {
			return tt.SendRichMarkdownError(c, "Invalid server name", err)
		}

		var (
			found        bool
			serverConfig config.ConfigServer
			removedLive  []config.ConfigLiveStatusMessage // live-status bindings dropped with the server, kept for rollback
		)

		// delete under the lock, keeping the old config so Save failure can be rolled back
		cf.Update(func(cfg *config.Config) {
			serverConfig, found = cfg.Servers[serverName]
			if !found {
				return
			}

			delete(cfg.Servers, serverName)

			// drop live-status messages bound to the removed server, keeping them for a possible rollback
			cfg.LiveStatusMessages = slices.DeleteFunc(cfg.LiveStatusMessages, func(m config.ConfigLiveStatusMessage) bool {
				if m.ServerName != serverName {
					return false
				}

				removedLive = append(removedLive, m)

				return true
			})
		})

		if !found {
			return tt.SendRichMarkdownError(c, fmt.Sprintf("Server `%s` does not exist", serverName), nil)
		}

		if err := cf.Save(); err != nil {
			// restore in-memory config to match disk
			cf.Update(func(cfg *config.Config) {
				cfg.Servers[serverName] = serverConfig
				cfg.LiveStatusMessages = append(cfg.LiveStatusMessages, removedLive...)
			})

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
