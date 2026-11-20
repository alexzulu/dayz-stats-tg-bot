package telegram_test

import (
	"testing"

	"github.com/alexzulu/dayz-stats-tg-bot/cmd/tg-bot/app/bot/telegram"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/testutil/assert"
)

func TestParseCommandArgs(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		give string
		want []string
	}{
		"spec example: mixed whitespace and quote styles": {
			give: "/any_command   Foo \n   \"Bar baz   \" \t\n\r    `some shit`   \n",
			want: []string{"Foo", "Bar baz   ", "some shit"},
		},
		"single quotes keep inner whitespace": {
			give: "/add 'a b  c'",
			want: []string{"a b  c"},
		},
		"barewords split on any run of whitespace": {
			give: "/add a\tb\n\nc   d",
			want: []string{"a", "b", "c", "d"},
		},
		"command with @bot suffix is dropped": {
			give: "/add@mybot x y",
			want: []string{"x", "y"},
		},
		"only the command yields no args": {
			give: "/add",
			want: nil,
		},
		"empty input yields no args": {
			give: "",
			want: nil,
		},
		"empty quoted value yields an empty arg": {
			give: `/add "" x`,
			want: []string{"", "x"},
		},
		"unterminated quote takes the rest verbatim": {
			give: `/add "foo bar`,
			want: []string{"foo bar"},
		},
		"no leading command: parses every token": {
			give: "foo 'bar baz'",
			want: []string{"foo", "bar baz"},
		},
		"multiline arg in quotes": {
			give: "foo 'bar \nbaz'",
			want: []string{"foo", "bar \nbaz"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.DeepEqual(t, tt.want, telegram.ParseCommandArgs(tt.give))
		})
	}
}

func TestEscapeMarkdown(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		give string
		want string
	}{
		"bold around a server alias": {
			give: "**" + telegram.EscapeMarkdown("cs_2.eu") + "**",
			want: `**cs\_2\.eu**`,
		},
		"spoiler hiding a token": {
			give: "||" + telegram.EscapeMarkdown("a|b") + "||",
			want: `||a\|b||`,
		},
		"table cell holding a pipe": {
			give: "| " + telegram.EscapeMarkdown("v1|2") + " | ok |",
			want: `| v1\|2 | ok |`,
		},
		"link label with brackets": {
			give: "[" + telegram.EscapeMarkdown("open [docs]") + "](https://t.me/)",
			want: `[open \[docs\]](https://t.me/)`,
		},
		"sentence with a backtick": {
			give: "Run " + telegram.EscapeMarkdown("use `ls`") + " now",
			want: "Run use \\`ls\\` now",
		},
		"blockquote with a backslash path": {
			give: "> " + telegram.EscapeMarkdown(`path C:\srv`) + " missing",
			want: `> path C:\\srv missing`,
		},
		// regression: ": / \" @ ," are NOT backslash-escapable in Rich Markdown, so they must stay raw
		// (escaping them was the bug that produced visible \: \/ \" in the daemon error message).
		"unescapable punctuation stays raw, dots still escaped": {
			give: telegram.EscapeMarkdown(`Get "https://1.2.3.4:8080/world": deadline`),
			want: `Get "https://1\.2\.3\.4:8080/world": deadline`,
		},
		// Telegram interprets HTML tags, so "<"/"&" are neutralized via entities (a backslash would show).
		"html tag is neutralized via entities": {
			give: telegram.EscapeMarkdown("<b>x</b>"),
			want: `&lt;b\>x&lt;/b\>`,
		},
		"ampersand becomes an entity": {
			give: telegram.EscapeMarkdown("Tom & Jerry"),
			want: `Tom &amp; Jerry`,
		},
		// the realistic case: a Go "<nil>" in an error must survive instead of vanishing as an unknown tag.
		"go nil keeps its angle brackets": {
			give: telegram.EscapeMarkdown("value: <nil>"),
			want: `value: &lt;nil\>`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, tt.give)
		})
	}
}
