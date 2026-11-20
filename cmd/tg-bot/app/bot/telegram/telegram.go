package telegram

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"time"

	tele "gopkg.in/telebot.v4"

	"github.com/alexzulu/dayz-stats-tg-bot/internal/bot"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/config"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/statez"
)

// Handlers holds the bot's command handlers and the config/admin list they operate on.
type Handlers struct {
	tgAdminIDs []int64
	cfg        *config.File
}

// NewHandlers returns handlers whose admin-only commands are restricted to the given Telegram user IDs.
func NewHandlers(tgAdminIDs []int64, cfg *config.File) *Handlers {
	return &Handlers{
		tgAdminIDs: tgAdminIDs,
		cfg:        cfg,
	}
}

const (
	cmdPing = "ping"

	cmdAddServer   = "add_server"
	cmdListServers = "servers"
)

// Register wires the private commands into b, guarded by panic recovery and request logging applied to every
// command. Admin-only commands additionally carry a whitelist middleware restricting them to h.tgAdminIDs.
func (h *Handlers) Register(ctx context.Context, log *slog.Logger, b *bot.Telegram) error {
	var (
		// buttons "unique" fields should be less than 31 byte and be unique across all buttons in the bot
		removeServerBtn = tele.Btn{
			Text:   "Remove server",
			Unique: "delsrv",
		}

		recoverMw = NewRecoverMiddleware(ctx, log)
		loggerMw  = NewLoggerMiddleware(ctx, log)
		authMw    = NewAuthMiddleware(ctx, log, h.tgAdminIDs)
	)

	if err := b.RegisterPrivateCommands(
		[]bot.Command{
			{
				Text:        cmdPing,
				Description: "Check if the bot is alive",
				Handler:     h.handlePing(),
			},
			{
				Text:        cmdAddServer,
				Description: "<name> <statez_base_url> <statez_auth_token> - add a new server",
				Handler:     h.handleAddServer(ctx, log),
				Middleware:  []tele.MiddlewareFunc{authMw},
			},
			{
				Text:        cmdListServers,
				Description: "Manage configured servers",
				Handler:     h.handleManageServers(&removeServerBtn),
				Middleware:  []tele.MiddlewareFunc{authMw},
			},
		},
		recoverMw,
		loggerMw,
	); err != nil {
		return err
	}

	if err := b.RegisterPrivateButtonHandlers(
		[]bot.Button{
			{
				Button:     &removeServerBtn,
				Handler:    h.handleRemoveServer(log),
				Middleware: []tele.MiddlewareFunc{authMw},
			},
		},
		recoverMw,
		loggerMw,
		NewAutoCallbackRespondMiddleware(),
	); err != nil {
		return err
	}

	return nil
}

func (h *Handlers) handlePing() tele.HandlerFunc {
	return func(c tele.Context) error {
		if user := c.Sender(); user != nil {
			return c.Reply(&tele.InputRichMessage{
				Markdown: fmt.Sprintf("pong, mr. with id `%d`", user.ID),
			})
		}

		return c.Reply("pong")
	}
}

func (h *Handlers) handleAddServer(ctx context.Context, log *slog.Logger) tele.HandlerFunc {
	return func(c tele.Context) error {
		// parse the raw text: telebot's payload stops at the first newline and ignores quotes
		args := ParseCommandArgs(c.Text())
		if len(args) != 3 { //nolint:mnd
			return c.Reply(&tele.InputRichMessage{Markdown: "" +
				"Adds a new server to the bot's configuration.\n\n" +
				"###### Usage: `/" + cmdAddServer + " <name> <statez_base_url> <statez_auth_token>`\n\n" +
				"| Argument            | Description                                        |\n" +
				"|--------------------:|:---------------------------------------------------|\n" +
				"| `name`              | Internal server alias                              |\n" +
				"| `statez_base_url`   | StateZ daemon address, e.g. `https://1.2.3.4:8080` |\n" +
				"| `statez_auth_token` | StateZ daemon authorization token                  |\n\n" +
				"> *Example: `/" + cmdAddServer + " chernarus https://1.2.3.4:8080 s3cr3t-token`*"})
		}

		name, baseURL, token := args[0], args[1], args[2]

		if err := ValidateServerName(name); err != nil {
			return c.Reply(&tele.InputRichMessage{
				Markdown: "**Invalid server name**: " + EscapeMarkdown(err.Error()) + "\n\n" +
					"*Allowed characters: letters, digits and* `_ . # -`",
				SkipEntityDetection: true, // the embedded error text must not be auto-linked
			})
		}

		if err := ValidateBaseURL(baseURL); err != nil {
			return c.Reply(&tele.InputRichMessage{
				Markdown: "**Invalid StateZ daemon address**: " + EscapeMarkdown(err.Error()) + "\n\n" +
					"*Expected format* `http(s)://host[:port]`",
				SkipEntityDetection: true,
			})
		}

		if err := ValidateAuthToken(token); err != nil {
			return c.Reply(&tele.InputRichMessage{
				Markdown:            "**Invalid auth token**: " + EscapeMarkdown(err.Error()),
				SkipEntityDetection: true,
			})
		}

		if _, exists := h.cfg.Config().Servers[name]; exists {
			return c.Reply(&tele.InputRichMessage{
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

			return c.Reply(&tele.InputRichMessage{
				Markdown:            "**Failed to reach the StateZ daemon**: " + EscapeMarkdown(connErr.Error()),
				SkipEntityDetection: true,
			})
		}

		h.cfg.Update(func(cfg *config.Config) {
			if cfg.Servers == nil {
				cfg.Servers = make(map[string]config.ConfigServer)
			}

			cfg.Servers[name] = config.ConfigServer{BaseUrl: baseURL, AuthToken: token}
		})

		if err := h.cfg.Save(); err != nil {
			h.cfg.Update(func(cfg *config.Config) { delete(cfg.Servers, name) }) // roll back so memory matches disk

			log.Error("failed to save config after "+cmdAddServer,
				slog.String("error", err.Error()),
				slog.String("server_name", name),
				slog.String("base_url", baseURL),
			)

			return c.Reply(&tele.InputRichMessage{
				Markdown:            "**Server verified, but saving the config failed**: " + EscapeMarkdown(err.Error()),
				SkipEntityDetection: true,
			})
		}

		defer func() { _ = react(c, reactEmojiThumbsUp) }() //nolint:errcheck

		return c.Reply(&tele.InputRichMessage{
			Markdown:            fmt.Sprintf("**Server `%s` added successfully**", name),
			SkipEntityDetection: true,
		})
	}
}

func (h *Handlers) handleManageServers(removeBtn *tele.Btn) tele.HandlerFunc {
	return func(c tele.Context) error {
		cfg := h.cfg.Config()

		if len(cfg.Servers) == 0 {
			return c.Reply(&tele.InputRichMessage{
				Markdown: "**No servers configured yet**. Use /" + cmdAddServer + " to add one",
			})
		}

		var (
			b      strings.Builder
			markup = &tele.ReplyMarkup{}
			names  = slices.Sorted(maps.Keys(cfg.Servers)) // stable order
			btns   = make([]tele.Btn, 0, len(names))
		)

		b.Grow(64 + (len(cfg.Servers) * 64)) //nolint:mnd // rough estimate

		b.WriteString("| Name | StateZ Base URL |\n")
		b.WriteString("|:----:|:---------------:|\n")

		for _, name := range names {
			b.WriteString("| `")
			b.WriteString(name) // validated on adding and clear
			b.WriteString("` | `")
			b.WriteString(cfg.Servers[name].BaseUrl) // validated on adding and clear
			b.WriteString("` |\n")

			btns = append(btns, markup.Data("🗑️ Remove \""+name+"\"", removeBtn.Unique, name))
		}

		markup.Inline(markup.Split(2, btns)...) //nolint:mnd

		return c.Reply(&tele.InputRichMessage{Markdown: b.String()}, markup)
	}
}

func (h *Handlers) handleRemoveServer(log *slog.Logger) tele.HandlerFunc {
	return func(c tele.Context) error {
		name := c.Data()

		if err := ValidateServerName(name); err != nil {
			return c.Reply(&tele.InputRichMessage{
				Markdown:            "**Invalid server name**: " + EscapeMarkdown(err.Error()),
				SkipEntityDetection: true,
			})
		}

		var (
			found    bool
			snapshot config.ConfigServer
		)

		h.cfg.Update(func(cfg *config.Config) {
			snapshot, found = cfg.Servers[name]
			if !found {
				return
			}

			delete(cfg.Servers, name)
		})

		if !found {
			return c.Reply(&tele.InputRichMessage{
				Markdown:            fmt.Sprintf("**Server `%s` does not exist**", name),
				SkipEntityDetection: true,
			})
		}

		if err := h.cfg.Save(); err != nil {
			// restore in-memory config to match disk
			h.cfg.Update(func(cfg *config.Config) { cfg.Servers[name] = snapshot })

			log.Error("failed to save config after removing server",
				slog.String("error", err.Error()),
				slog.String("server_name", name),
			)

			return c.Reply(&tele.InputRichMessage{
				Markdown:            "**Failed to save the config**: " + EscapeMarkdown(err.Error()),
				SkipEntityDetection: true,
			})
		}

		return c.Reply(&tele.InputRichMessage{
			Markdown:            fmt.Sprintf("**Server `%s` removed successfully**", name),
			SkipEntityDetection: true,
		})
	}
}
