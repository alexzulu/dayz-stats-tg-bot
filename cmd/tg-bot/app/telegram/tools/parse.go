package tt

import (
	"strings"
	"unicode"
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
