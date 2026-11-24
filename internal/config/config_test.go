package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alexzulu/dayz-stats-tg-bot/internal/config"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/testutil/assert"
)

func TestFileLoad(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		raw     string
		create  bool // false => file absent on disk
		wantErr bool
		want    config.Config
	}{
		"missing file is zero config": {
			want: config.Config{Servers: map[string]config.ConfigServer{}},
		},
		"valid json is parsed": {
			create: true,
			raw:    `{"servers":{"eu":{"base_url":"https://host","auth_token":"tok"}},"live_status_messages":[{"chat_id":1,"message_id":2,"server_name":"eu"}]}`,
			want: config.Config{
				Servers:            map[string]config.ConfigServer{"eu": {BaseUrl: "https://host", AuthToken: "tok"}},
				LiveStatusMessages: []config.ConfigLiveStatusMessage{{ChatID: 1, MessageID: 2, ServerName: "eu"}},
			},
		},
		"corrupt json is an error": {
			create:  true,
			raw:     `{not json`,
			wantErr: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "cfg.json")
			if tt.create {
				assert.NoError(t, os.WriteFile(path, []byte(tt.raw), 0o600))
			}

			f := config.NewFile(path)
			err := f.Load()

			if tt.wantErr {
				assert.Error(t, err)

				return
			}

			assert.NoError(t, err)
			assert.DeepEqual(t, tt.want, f.Config())
		})
	}
}

func TestFileSaveReload(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "sub", "cfg.json") // nested dir: Save must create it

	f := config.NewFile(path)
	f.Update(func(c *config.Config) {
		c.Servers = map[string]config.ConfigServer{"eu": {BaseUrl: "https://host", AuthToken: "tok"}}
		c.LiveStatusMessages = []config.ConfigLiveStatusMessage{{ChatID: 1, MessageID: 2, ServerName: "eu"}}
	})

	assert.NoError(t, f.Save())
	assert.FileExists(t, path)

	reloaded := config.NewFile(path)
	assert.NoError(t, reloaded.Load())
	assert.DeepEqual(t, f.Config(), reloaded.Config())
}

func TestFileConfigIsDeepCopy(t *testing.T) {
	t.Parallel()

	f := config.NewFile(filepath.Join(t.TempDir(), "cfg.json"))
	f.Update(func(c *config.Config) {
		c.Servers = map[string]config.ConfigServer{"eu": {BaseUrl: "u", AuthToken: "t"}}
		c.LiveStatusMessages = []config.ConfigLiveStatusMessage{{ChatID: 1}}
	})

	// mutating a returned copy must not leak into the File's shared state
	got := f.Config()
	got.Servers["eu"] = config.ConfigServer{BaseUrl: "hacked"}
	got.Servers["new"] = config.ConfigServer{}
	got.LiveStatusMessages[0].ChatID = 99

	fresh := f.Config()
	assert.DeepEqual(t, config.ConfigServer{BaseUrl: "u", AuthToken: "t"}, fresh.Servers["eu"])
	assert.Equal(t, 1, len(fresh.Servers))
	assert.Equal(t, int64(1), fresh.LiveStatusMessages[0].ChatID)
}
