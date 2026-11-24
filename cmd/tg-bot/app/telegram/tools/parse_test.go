package tt_test

import (
	"testing"

	tools "github.com/alexzulu/dayz-stats-tg-bot/cmd/tg-bot/app/telegram/tools"
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

			assert.DeepEqual(t, tt.want, tools.ParseCommandArgs(tt.give))
		})
	}
}
