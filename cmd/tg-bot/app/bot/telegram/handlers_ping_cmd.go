package telegram

import (
	"fmt"

	tele "gopkg.in/telebot.v4"
)

func (h *Handlers) handlePingCommand() tele.HandlerFunc {
	return func(c tele.Context) error {
		if user := c.Sender(); user != nil {
			return c.Send(&tele.InputRichMessage{
				Markdown: fmt.Sprintf("pong, mr. with id `%d`", user.ID),
			})
		}

		return c.Send("pong")
	}
}
