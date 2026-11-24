package tt

import (
	"strings"

	tele "gopkg.in/telebot.v4"
)

// "<" opens a tag ("<nil>" would vanish) and "&" opens an entity; neither is backslash-escapable. The
// single-pass replacer does not re-escape the "&" it inserts.
var escapeMarkdownReplacer = strings.NewReplacer("&", "&amp;", "<", "&lt;") //nolint:gochecknoglobals

// EscapeMarkdown makes s safe to embed in a tele.InputRichMessage{Markdown: ...} so it renders as the literal
// text given. Telegram Rich Markdown interprets HTML tags and decodes entities, and only honors a backslash
// before a subset of punctuation (both verified empirically), so escaping is two steps - HTML entities for the
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

// SendRichMarkdown sends a Markdown message to the user.
func SendRichMarkdown(c tele.Context, markdown string) error {
	return c.Send(&tele.InputRichMessage{Markdown: markdown, SkipEntityDetection: true})
}

// SendRichMarkdownError sends a Markdown message to the user with an error message and optional extra lines.
// Passing an empty prefix, nil error, or no extra lines is okay.
func SendRichMarkdownError(c tele.Context, prefix string, err error, extra ...string) error {
	var (
		b         strings.Builder
		hasPrefix = prefix != ""
	)

	b.Grow(len(prefix) + (len(extra) * 32) + 64) //nolint:mnd // rough estimate

	if hasPrefix {
		b.WriteString("**")
		b.WriteString(prefix)
		b.WriteString("**")
	}

	if err != nil {
		if hasPrefix {
			b.WriteString(": ")
		}

		b.WriteString(EscapeMarkdown(err.Error()))
	}

	if len(extra) > 0 && (hasPrefix || err != nil) {
		b.WriteString("\n\n")

		for i, s := range extra {
			b.WriteString(s)

			if i < len(extra)-1 {
				b.WriteString("\n")
			}
		}
	}

	return SendRichMarkdown(c, b.String())
}
