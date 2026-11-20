package tt_test

import (
	"testing"

	tools "github.com/alexzulu/dayz-stats-tg-bot/cmd/tg-bot/app/telegram/tools"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/testutil/assert"
)

func TestEscapeMarkdown(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		give string
		want string
	}{
		"bold around a server alias": {
			give: "**" + tools.EscapeMarkdown("cs_2.eu") + "**",
			want: `**cs\_2\.eu**`,
		},
		"spoiler hiding a token": {
			give: "||" + tools.EscapeMarkdown("a|b") + "||",
			want: `||a\|b||`,
		},
		"table cell holding a pipe": {
			give: "| " + tools.EscapeMarkdown("v1|2") + " | ok |",
			want: `| v1\|2 | ok |`,
		},
		"link label with brackets": {
			give: "[" + tools.EscapeMarkdown("open [docs]") + "](https://t.me/)",
			want: `[open \[docs\]](https://t.me/)`,
		},
		"sentence with a backtick": {
			give: "Run " + tools.EscapeMarkdown("use `ls`") + " now",
			want: "Run use \\`ls\\` now",
		},
		"blockquote with a backslash path": {
			give: "> " + tools.EscapeMarkdown(`path C:\srv`) + " missing",
			want: `> path C:\\srv missing`,
		},
		// regression: ": / \" @ ," are NOT backslash-escapable in Rich Markdown, so they must stay raw
		// (escaping them was the bug that produced visible \: \/ \" in the daemon error message).
		"unescapable punctuation stays raw, dots still escaped": {
			give: tools.EscapeMarkdown(`Get "https://1.2.3.4:8080/world": deadline`),
			want: `Get "https://1\.2\.3\.4:8080/world": deadline`,
		},
		// Telegram interprets HTML tags, so "<"/"&" are neutralized via entities (a backslash would show).
		"html tag is neutralized via entities": {
			give: tools.EscapeMarkdown("<b>x</b>"),
			want: `&lt;b\>x&lt;/b\>`,
		},
		"ampersand becomes an entity": {
			give: tools.EscapeMarkdown("Tom & Jerry"),
			want: `Tom &amp; Jerry`,
		},
		// the realistic case: a Go "<nil>" in an error must survive instead of vanishing as an unknown tag.
		"go nil keeps its angle brackets": {
			give: tools.EscapeMarkdown("value: <nil>"),
			want: `value: &lt;nil\>`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, tt.give)
		})
	}
}
