package telegram_test

import (
	"testing"

	"github.com/alexzulu/dayz-stats-tg-bot/cmd/tg-bot/app/bot/telegram"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/testutil/assert"
)

func TestValidateServerName(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		give        string
		wantErr     bool
		errContains string
	}{
		"letters, digits and allowed specials": {give: "chernarus_1.2-main#eu", wantErr: false},
		"only special characters is rejected":  {give: "_.#-", wantErr: true, errContains: "at least one letter or digit"},
		"empty is rejected":                    {give: "", wantErr: true, errContains: "must not be empty"},
		"whitespace is rejected":               {give: "my server", wantErr: true, errContains: "contains invalid characters"},
		"disallowed punctuation is rejected":   {give: "a/b", wantErr: true, errContains: "contains invalid characters"},
		"non-ascii letter is rejected":         {give: "café", wantErr: true, errContains: "contains invalid characters"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := telegram.ValidateServerName(tt.give)
			if tt.wantErr {
				assert.ErrorContains(t, err, tt.errContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateBaseURL(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		give        string
		wantErr     bool
		errContains string
	}{
		"https with host and port": {give: "https://1.2.3.4:8080", wantErr: false},
		"wrong scheme rejected":    {give: "ftp://host", wantErr: true, errContains: "invalid scheme"},
		"missing host rejected":    {give: "https://", wantErr: true, errContains: "missing host"},
		"missing scheme rejected":  {give: "example.com:8080", wantErr: true, errContains: "invalid scheme"},
		"unparseable url rejected": {give: "%zz", wantErr: true, errContains: "invalid URL escape"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := telegram.ValidateBaseURL(tt.give)
			if tt.wantErr {
				assert.ErrorContains(t, err, tt.errContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
