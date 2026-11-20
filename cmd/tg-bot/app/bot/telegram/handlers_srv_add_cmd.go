package telegram

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	tele "gopkg.in/telebot.v4"

	"github.com/alexzulu/dayz-stats-tg-bot/internal/config"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/statez"
)

func (h *Handlers) handleAddServerCommand(ctx context.Context) tele.HandlerFunc {
	return func(c tele.Context) error {
		// parse the raw text: telebot's payload stops at the first newline and ignores quotes
		args := ParseCommandArgs(c.Text())
		if len(args) != 3 { //nolint:mnd
			return c.Send(&tele.InputRichMessage{Markdown: "" +
				"Adds a new server to the bot's configuration.\n\n" +
				"###### Usage: `/" + cmdAddServer + " <server_name> <statez_base_url> <statez_auth_token>`\n\n" +
				"| Argument            | Description                                        |\n" +
				"|--------------------:|:---------------------------------------------------|\n" +
				"| `server_name`       | Internal server alias                              |\n" +
				"| `statez_base_url`   | StateZ daemon address, e.g. `https://1.2.3.4:8080` |\n" +
				"| `statez_auth_token` | StateZ daemon authorization token                  |\n\n" +
				"> *Example: `/" + cmdAddServer + " chernarus https://1.2.3.4:8080 s3cr3t-token`*"})
		}

		name, baseURL, token := args[0], args[1], args[2]

		if err := ValidateServerName(name); err != nil {
			return c.Send(&tele.InputRichMessage{
				Markdown: "**Invalid server name**: " + EscapeMarkdown(err.Error()) + "\n\n" +
					"*Allowed characters: letters, digits and* `_ . # -`",
				SkipEntityDetection: true, // the embedded error text must not be auto-linked
			})
		}

		if err := ValidateBaseURL(baseURL); err != nil {
			return c.Send(&tele.InputRichMessage{
				Markdown: "**Invalid StateZ daemon address**: " + EscapeMarkdown(err.Error()) + "\n\n" +
					"*Expected format* `http(s)://host[:port]`",
				SkipEntityDetection: true,
			})
		}

		if err := ValidateAuthToken(token); err != nil {
			return c.Send(&tele.InputRichMessage{
				Markdown:            "**Invalid auth token**: " + EscapeMarkdown(err.Error()),
				SkipEntityDetection: true,
			})
		}

		if _, exists := h.cfg.Config().Servers[name]; exists {
			return c.Send(&tele.InputRichMessage{
				Markdown:            fmt.Sprintf("**Server %s already exists**", name),
				SkipEntityDetection: true,
			})
		}

		baseURL = strings.TrimRight(baseURL, "/ ") + "/" // ensure a trailing slash

		// verify the credentials reach the daemon before persisting anything
		pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second) //nolint:mnd
		defer cancel()

		// show the user that we're still alive while we wait for the ping to complete
		_ = notify(pingCtx, c, tele.FindingLocation) //nolint:errcheck

		// ping the daemon with the given credentials
		_, connErr := statez.NewClient(baseURL, token).World(pingCtx)
		if connErr != nil {
			if errors.Is(connErr, context.DeadlineExceeded) {
				connErr = fmt.Errorf("timeout while connecting to %s (daemon may be down or unreachable)", baseURL)
			}

			return c.Send(&tele.InputRichMessage{
				Markdown:            "**Failed to reach the StateZ daemon**: " + EscapeMarkdown(connErr.Error()),
				SkipEntityDetection: true,
			})
		}

		serverConfig := config.ConfigServer{BaseUrl: baseURL, AuthToken: token}

		h.cfg.Update(func(cfg *config.Config) {
			if cfg.Servers == nil {
				cfg.Servers = make(map[string]config.ConfigServer)
			}

			cfg.Servers[name] = serverConfig
		})

		if err := h.cfg.Save(); err != nil {
			h.cfg.Update(func(cfg *config.Config) { delete(cfg.Servers, name) }) // roll back so memory matches disk

			h.log.Error("failed to save config after "+cmdAddServer,
				slog.String("error", err.Error()),
				slog.String("server_name", name),
				slog.String("base_url", baseURL),
			)

			return c.Send(&tele.InputRichMessage{
				Markdown:            "**Server verified, but saving the config failed**: " + EscapeMarkdown(err.Error()),
				SkipEntityDetection: true,
			})
		}

		defer func() { _ = react(c, reactEmojiThumbsUp) }() //nolint:errcheck

		return c.Send(&tele.InputRichMessage{
			Markdown:            fmt.Sprintf("**Server `%s` added successfully**", name),
			SkipEntityDetection: true,
		})
	}
}
