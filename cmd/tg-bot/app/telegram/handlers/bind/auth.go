package bind

import (
	"slices"

	tele "gopkg.in/telebot.v4"
)

// isAdmin reports whether the message sender is in the admin allowlist.
func isAdmin(c tele.Context, adminIDs []int64) bool {
	s := c.Sender()

	return s != nil && slices.Contains(adminIDs, s.ID)
}
