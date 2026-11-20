package servers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	tele "gopkg.in/telebot.v4"

	tt "github.com/alexzulu/dayz-stats-tg-bot/cmd/tg-bot/app/telegram/tools"
	"github.com/alexzulu/dayz-stats-tg-bot/cmd/tg-bot/app/telegram/validate"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/config"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/statez"
)

func NewCreateServerHandler(ctx context.Context, log *slog.Logger, cmdName string, cf *config.File) tele.HandlerFunc {
	usage := "###### Usage: `/" + cmdName + " <server_name> <statez_base_url> <statez_auth_token>`\n\n" +
		"| Argument            | Description                                        |\n" +
		"|--------------------:|:---------------------------------------------------|\n" +
		"| `server_name`       | Internal server alias                              |\n" +
		"| `statez_base_url`   | StateZ daemon address, e.g. `https://1.2.3.4:8080` |\n" +
		"| `statez_auth_token` | StateZ daemon authorization token                  |\n\n" +
		"> *Example: `/" + cmdName + " chernarus https://1.2.3.4:8080 s3cr3t-token`*"

	return func(c tele.Context) error {
		args := tt.ParseCommandArgs(c.Text())
		if len(args) != 3 { //nolint:mnd
			return tt.SendRichMarkdown(c, usage)
		}

		name, baseURL, token := args[0], args[1], args[2]

		if err := validate.ServerName(name); err != nil {
			return tt.SendRichMarkdownError(c, "Invalid server name", err)
		}

		if err := validate.BaseURL(baseURL); err != nil {
			return tt.SendRichMarkdownError(c,
				"Invalid StateZ daemon address", err,
				"*Expected format* `http(s)://host[:port]`",
			)
		}

		if err := validate.AuthToken(token); err != nil {
			return tt.SendRichMarkdownError(c, "Invalid StateZ daemon auth token", err)
		}

		if _, exists := cf.Config().Servers[name]; exists {
			return tt.SendRichMarkdownError(c,
				"Server already exists", fmt.Errorf("server with name %q already exists", name),
			)
		}

		baseURL = strings.TrimRight(baseURL, "/ ") + "/" // ensure a trailing slash

		// verify the credentials reach the daemon before persisting anything
		pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second) //nolint:mnd
		defer cancel()

		// show the user that we're still alive while we wait for the ping to complete
		_ = tt.Notify(pingCtx, c, tele.FindingLocation) //nolint:errcheck

		// ping the daemon with the given credentials
		_, connErr := statez.NewClient(baseURL, token).World(pingCtx)
		if connErr != nil {
			if errors.Is(connErr, context.DeadlineExceeded) {
				connErr = fmt.Errorf("timeout while connecting to %s (daemon may be down or unreachable)", baseURL)
			}

			return tt.SendRichMarkdownError(c, "Failed to reach the StateZ daemon", connErr)
		}

		serverConfig := config.ConfigServer{BaseUrl: baseURL, AuthToken: token}

		cf.Update(func(cfg *config.Config) {
			if cfg.Servers == nil {
				cfg.Servers = make(map[string]config.ConfigServer)
			}

			cfg.Servers[name] = serverConfig
		})

		if err := cf.Save(); err != nil {
			cf.Update(func(cfg *config.Config) { delete(cfg.Servers, name) }) // roll back so memory matches disk

			log.Error("failed to save config after "+cmdName,
				slog.String("error", err.Error()),
				slog.String("server_name", name),
				slog.String("base_url", baseURL),
			)

			return tt.SendRichMarkdownError(c, "Server verified, but saving the config failed", err)
		}

		// respond first, then react to indicate success
		defer func() { _ = tt.React(c, tt.ReactEmojiThumbsUp) }() //nolint:errcheck

		return tt.SendRichMarkdown(c, fmt.Sprintf("**Server `%s` added successfully**", name))
	}
}
