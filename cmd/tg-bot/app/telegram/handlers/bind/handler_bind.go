package bind

import (
	"log/slog"
	"maps"
	"slices"

	tele "gopkg.in/telebot.v4"

	tt "github.com/alexzulu/dayz-stats-tg-bot/cmd/tg-bot/app/telegram/tools"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/config"
)

// NewBindHandler returns the /bind handler. In a private chat it replies with setup instructions; in a group or
// forum thread it verifies the bot's own permissions, then sends a server picker whose message becomes the
// self-updating status message once a server is confirmed.
//
// Auth is enforced inside the handler, not via middleware, because the two entry points differ - a non-admin in a
// private chat gets an error, a non-admin in a group is ignored silently.
func NewBindHandler(log *slog.Logger, cf *config.File, adminIDs []int64, me *tele.User) tele.HandlerFunc {
	return func(c tele.Context) error {
		chat := c.Chat()
		if chat == nil {
			return nil
		}

		switch chat.Type { //nolint:exhaustive // default ignores channels and any other chat type
		case tele.ChatPrivate:
			if !isAdmin(c, adminIDs) {
				return tt.SendRichMarkdownError(c, "Unauthorized: you are not an admin", nil)
			}

			return sendInstructions(c, me)
		case tele.ChatGroup, tele.ChatSuperGroup:
			if !isAdmin(c, adminIDs) {
				return nil // non-admins are ignored silently in groups
			}

			return handleGroupBind(c, log, cf, me)
		default:
			return nil
		}
	}
}

// sendInstructions tells the admin how to bind a status message from the target group.
func sendInstructions(c tele.Context, me *tele.User) error {
	botName := "<bot>"
	if me != nil && me.Username != "" {
		botName = me.Username
	}

	return tt.SendRichMarkdown(c, "**Bind a self-updating server status message**\n\n"+
		"1. Add `@"+botName+"` to the target group.\n"+
		"2. Promote it to admin with the **Pin Messages** permission.\n"+
		"3. In the target group (or forum topic), send `/bind@"+botName+"`.")
}

// handleGroupBind checks the bot's rights in the current chat and, when they suffice, sends the server picker.
func handleGroupBind(c tele.Context, log *slog.Logger, cf *config.File, me *tele.User) error {
	member, err := c.Bot().ChatMemberOf(c.Chat(), me)
	if err != nil || member == nil {
		if err != nil {
			log.Error("failed to read bot membership for /bind", slog.String("error", err.Error()))
		}

		return tt.SendRichMarkdownError(c, "I can't read my membership here - add me to this group first", nil)
	}

	switch member.Role { //nolint:exhaustive // default covers creator/administrator/member: those can send
	case tele.Left, tele.Kicked:
		return tt.SendRichMarkdownError(c, "I'm not a member of this group - add me first", nil)
	case tele.Restricted:
		if !member.CanSendMessages {
			return tt.SendRichMarkdownError(c, "I'm not allowed to send messages here - lift that restriction", nil)
		}
	default:
		// creator, administrator or plain member: sending is governed by the chat's defaults, assume it's allowed
	}

	canPin := member.Role == tele.Creator || (member.Role == tele.Administrator && member.CanPinMessages)
	if !canPin {
		return tt.SendRichMarkdownError(c,
			"I can't pin messages here - make me an admin with the Pin Messages permission", nil)
	}

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
		buttons = append(buttons, markup.Data("🗺️ "+name, BindServerBtn.Unique, name))
	}

	markup.Inline(markup.Split(2, buttons)...) //nolint:mnd

	// this very message becomes the self-updating status message once a server is confirmed
	return c.Send(&tele.InputRichMessage{Markdown: "Select a server to bind a self-updating status message:"}, &markup)
}
