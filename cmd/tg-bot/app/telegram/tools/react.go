package tt

import "gopkg.in/telebot.v4"

type reactEmoji rune

const (
	ReactEmojiThumbsUp   reactEmoji = '👍'
	ReactEmojiThumbsDown reactEmoji = '👎'
	ReactEmojiFire       reactEmoji = '🔥'
	ReactEmojiClap       reactEmoji = '👏'
	ReactEmojiGrin       reactEmoji = '😁'
	ReactEmojiParty      reactEmoji = '🎉'
	ReactEmojiStarStruck reactEmoji = '🤩'
	ReactEmojiMindBlown  reactEmoji = '🤯'
	ReactEmojiHundred    reactEmoji = '💯'
	ReactEmojiPray       reactEmoji = '🙏'
	ReactEmojiOk         reactEmoji = '👌'
	ReactEmojiHeartEyes  reactEmoji = '😍'
	ReactEmojiThinking   reactEmoji = '🤔'
	ReactEmojiCry        reactEmoji = '😢'
	ReactEmojiEyes       reactEmoji = '👀'
)

// React adds a reactions to the message that triggered the context. To remove all reactions, call react(c) with
// no emoji.
func React(c telebot.Context, emoji ...reactEmoji) error {
	if c == nil || c.Bot() == nil || c.Sender() == nil || c.Message() == nil {
		return nil
	}

	reactions := make([]telebot.Reaction, 0, len(emoji))

	if len(emoji) > 0 {
		for _, e := range emoji {
			reactions = append(reactions, telebot.Reaction{Type: telebot.ReactionTypeEmoji, Emoji: string(e)})
		}
	}

	return c.Bot().React(c.Sender(), c.Message(), telebot.Reactions{Reactions: reactions})
}
