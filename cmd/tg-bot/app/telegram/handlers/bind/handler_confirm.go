package bind

import (
	"fmt"
	"log/slog"
	"slices"

	tele "gopkg.in/telebot.v4"

	tt "github.com/alexzulu/dayz-stats-tg-bot/cmd/tg-bot/app/telegram/tools"
	"github.com/alexzulu/dayz-stats-tg-bot/cmd/tg-bot/app/telegram/validate"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/config"
)

// BindServerBtn is the inline server-picker button rendered by /bind in a group; its callback payload is the name
// of the server to bind.
var BindServerBtn = tele.Btn{Text: "Bind server", Unique: "bindsrv"} //nolint:gochecknoglobals

// NewBindServerBtnHandler returns the callback handler for BindServerBtn. It persists the (chat, thread, message,
// server) binding, swaps the picker for a placeholder, and pins the message. Auth is enforced here (non-admins
// are ignored silently) rather than via middleware, to match the silent group behavior of /bind.
func NewBindServerBtnHandler(log *slog.Logger, cf *config.File, adminIDs []int64) tele.HandlerFunc {
	return func(c tele.Context) error {
		if !isAdmin(c, adminIDs) {
			return nil // non-admins are ignored silently
		}

		serverName := c.Data() // received from [BindServerBtn] via `callback_data`
		if err := validate.ServerName(serverName); err != nil {
			return tt.SendRichMarkdownError(c, "Invalid server name", err)
		}

		if _, ok := cf.Config().Servers[serverName]; !ok {
			return tt.SendRichMarkdownError(c, fmt.Sprintf("Server `%s` no longer exists", serverName), nil)
		}

		msg := c.Message()
		if msg == nil || msg.Chat == nil {
			return nil
		}

		entry := config.ConfigLiveStatusMessage{
			ChatID:     msg.Chat.ID,
			ThreadID:   msg.ThreadID,
			MessageID:  msg.ID,
			ServerName: serverName,
		}

		cf.Update(func(cfg *config.Config) {
			cfg.LiveStatusMessages = append(cfg.LiveStatusMessages, entry)
		})

		if err := cf.Save(); err != nil {
			// roll back the just-appended binding (last element equal to entry) so memory matches disk
			cf.Update(func(cfg *config.Config) {
				for i, m := range slices.Backward(cfg.LiveStatusMessages) {
					if m == entry {
						cfg.LiveStatusMessages = slices.Delete(cfg.LiveStatusMessages, i, i+1)

						break
					}
				}
			})

			log.Error("failed to save config after binding a status message",
				slog.String("error", err.Error()),
				slog.String("server_name", serverName),
			)

			return tt.SendRichMarkdownError(c, "Failed to save the config", err)
		}

		// swap the picker for the placeholder and drop the buttons (empty markup clears the inline keyboard)
		if err := c.Edit(&tele.InputRichMessage{Markdown: "Waiting for updates"}, &tele.ReplyMarkup{}); err != nil {
			log.Error("failed to render the bound status message placeholder", slog.String("error", err.Error()))
		}

		// pinning is best-effort: the binding is already persisted, the updater will reconcile from the config
		if err := c.Bot().Pin(msg); err != nil {
			log.Error("failed to pin the bound status message", slog.String("error", err.Error()))
		}

		return nil
	}
}
