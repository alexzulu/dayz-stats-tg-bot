package telegram

import (
	"context"
	"strings"
	"time"
	"unicode"

	"gopkg.in/telebot.v4"
)

// ParseCommandArgs splits a raw Telegram command message into its arguments.
//
// A leading "/command" (or "/command@bot") token is dropped. The rest is split on any run of whitespace
// (spaces, tabs, newlines, CR). An argument may be wrapped in single ('), double (") or backtick (`) quotes to
// keep its inner whitespace; the surrounding quotes are stripped.
//
// It parses the raw message text on purpose: telebot's own payload stops at the first newline and ignores quotes.
func ParseCommandArgs(s string) []string {
	s = strings.TrimLeftFunc(s, unicode.IsSpace)

	// drop the leading command: the first whitespace-delimited run starting with '/'
	if strings.HasPrefix(s, "/") {
		if i := strings.IndexFunc(s, unicode.IsSpace); i >= 0 {
			s = s[i:]
		} else {
			return nil // message carried nothing but the command
		}
	}

	var (
		args  []string
		cur   strings.Builder
		quote rune // active quote char, 0 when outside quotes
		inTok bool // a token is being built (stays true across a closed quote, so "" yields an empty arg)
	)

	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"' || r == '`':
			quote, inTok = r, true
		case unicode.IsSpace(r):
			if inTok {
				args = append(args, cur.String())
				cur.Reset()

				inTok = false
			}
		default:
			cur.WriteRune(r)

			inTok = true
		}
	}

	if inTok {
		args = append(args, cur.String())
	}

	return args
}

// "<" opens a tag ("<nil>" would vanish) and "&" opens an entity; neither is backslash-escapable. The
// single-pass replacer does not re-escape the "&" it inserts.
var escapeMarkdownReplacer = strings.NewReplacer("&", "&amp;", "<", "&lt;") //nolint:gochecknoglobals

// EscapeMarkdown makes s safe to embed in a tele.InputRichMessage{Markdown: ...} so it renders as the literal
// text given. Telegram Rich Markdown interprets HTML tags and decodes entities, and only honors a backslash
// before a subset of punctuation (both verified empirically), so escaping is two steps: HTML entities for the
// characters a backslash can't neutralize, then a backslash before the Markdown-significant ones.
//
// It does NOT stop entity auto-detection (URLs, @mentions, #tags, emails, phones, /commands) - send the message
// with SkipEntityDetection: true for that.
func EscapeMarkdown(s string) string {
	s = escapeMarkdownReplacer.Replace(s)

	// characters Telegram honors after a backslash; ">" here neutralizes a line-start blockquote
	const markdownSpecial = "!#$()*+-.=>[\\]_`{|}~"

	var b strings.Builder

	b.Grow(len(s) + len(s)/8) // small headroom for escape backslashes

	for _, r := range s {
		if strings.ContainsRune(markdownSpecial, r) {
			b.WriteByte('\\')
		}

		b.WriteRune(r)
	}

	return b.String()
}

// notify sends chat action act once synchronously, then re-sends it every 4 seconds (chat actions last at most
// ~5s) in a background goroutine until ctx is canceled. It returns the error of the first, synchronous send;
// the background re-sends are best-effort and just stop the loop on error.
func notify(ctx context.Context, c telebot.Context, act telebot.ChatAction) error {
	if err := c.Notify(act); err != nil {
		return err
	}

	go func() {
		t := time.NewTicker(4 * time.Second) //nolint:mnd // AFAIK, we need to repeat the action every ~5s to keep it alive
		defer t.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := c.Notify(act); err != nil {
					return
				}
			}
		}
	}()

	return nil
}

type reactEmoji rune

const (
	reactEmojiThumbsUp   reactEmoji = '👍'
	reactEmojiThumbsDown reactEmoji = '👎'
	reactEmojiFire       reactEmoji = '🔥'
	reactEmojiClap       reactEmoji = '👏'
	reactEmojiGrin       reactEmoji = '😁'
	reactEmojiParty      reactEmoji = '🎉'
	reactEmojiStarStruck reactEmoji = '🤩'
	reactEmojiMindBlown  reactEmoji = '🤯'
	reactEmojiHundred    reactEmoji = '💯'
	reactEmojiPray       reactEmoji = '🙏'
	reactEmojiOk         reactEmoji = '👌'
	reactEmojiHeartEyes  reactEmoji = '😍'
	reactEmojiThinking   reactEmoji = '🤔'
	reactEmojiCry        reactEmoji = '😢'
	reactEmojiEyes       reactEmoji = '👀'
)

// react adds a reactions to the message that triggered the context. To remove all reactions, call react(c) with
// no emoji.
func react(c telebot.Context, emoji ...reactEmoji) error {
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
