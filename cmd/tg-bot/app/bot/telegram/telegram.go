package telegram

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"log/slog"
	"maps"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gh.tarampamp.am/dayz-statez/server/pkg/api"
	tele "gopkg.in/telebot.v4"

	"github.com/alexzulu/dayz-stats-tg-bot/internal/bot"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/config"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/gamemap"
	gameMaps "github.com/alexzulu/dayz-stats-tg-bot/internal/gamemap/maps"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/gamemap/maps/chernarusplus"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/statez"
)

type Handlers struct {
	log *slog.Logger
	cfg *config.File

	buttons struct {
		removeServer,
		players,
		player tele.Btn
	}

	clients struct {
		pool map[string]*statez.CachedClient
		ttl  time.Duration
		sync.Mutex
	}
}

func NewHandlers(log *slog.Logger, cfg *config.File) *Handlers {
	h := Handlers{
		log: log,
		cfg: cfg,
	}

	// buttons "unique" fields should be less than 31 byte and be unique across all buttons in the bot
	h.buttons.removeServer = tele.Btn{Text: "Remove server", Unique: "delsrv"}
	h.buttons.players = tele.Btn{Text: "Players online", Unique: "players"}
	h.buttons.player = tele.Btn{Text: "Player details", Unique: "player"}

	servers := cfg.Config().Servers
	h.clients.pool = make(map[string]*statez.CachedClient, len(servers))

	// burn the configured statez clients into memory so we don't have to create them on every request
	// and for caching purposes
	for name, server := range servers {
		h.clients.pool[name] = statez.NewCachedClient(server.BaseUrl, server.AuthToken, h.clients.ttl)
	}

	return &h
}

const (
	cmdAddServer = "add_server"
)

func (h *Handlers) Register(ctx context.Context, b *bot.Telegram, tgAdminIDs []int64) error {
	var (
		recoverMw = NewRecoverMiddleware(ctx, h.log)
		loggerMw  = NewLoggerMiddleware(ctx, h.log)
		authMw    = NewAuthMiddleware(ctx, h.log, tgAdminIDs)
	)

	if err := b.RegisterPrivateCommands(
		[]bot.Command{
			{
				Text:        "ping",
				Description: "Check if the bot is alive",
				Handler:     h.handlePingCommand(),
			},
			{
				Text:        cmdAddServer,
				Description: "<server_name> <statez_base_url> <statez_auth_token> - add a new server",
				Handler:     h.handleAddServerCommand(ctx),
				Middleware:  []tele.MiddlewareFunc{authMw},
			},
			{
				Text:        "servers",
				Description: "Manage configured servers",
				Handler:     h.handleManageServersCommand(),
				Middleware:  []tele.MiddlewareFunc{authMw},
			},
			{
				Text:        "players",
				Description: "List players online",
				Handler:     h.handlePlayersCommand(),
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
				Button:     &h.buttons.removeServer,
				Handler:    h.handleRemoveServerCallback(),
				Middleware: []tele.MiddlewareFunc{authMw},
			},
			{
				Button:     &h.buttons.players,
				Handler:    h.handlePlayersCallback(ctx),
				Middleware: []tele.MiddlewareFunc{authMw},
			},
			{
				Button:     &h.buttons.player,
				Handler:    h.handlePlayerCallback(ctx),
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

func (h *Handlers) getClientServerKey(server config.ConfigServer) string {
	return server.BaseUrl + "|" + server.AuthToken
}

// getClientForServer returns a cached StateZ client for the given server. If a client for the server already exists
// in the pool, it is returned. Otherwise, a new client is created, added to the pool, and then returned.
//
// To remove a client from the pool, use the forgetClientForServer method.
func (h *Handlers) getClientForServer(server config.ConfigServer) *statez.CachedClient {
	key := h.getClientServerKey(server)

	h.clients.Lock()
	defer h.clients.Unlock()

	if c, exists := h.clients.pool[key]; exists {
		return c
	}

	c := statez.NewCachedClient(server.BaseUrl, server.AuthToken, h.clients.ttl)
	h.clients.pool[key] = c

	return c
}

// forgetClientForServer removes the cached StateZ client for the given server from the pool. This is useful when a
// server is removed from the configuration, and we want to ensure that the client is no longer used.
func (h *Handlers) forgetClientForServer(server config.ConfigServer) {
	h.clients.Lock()
	delete(h.clients.pool, h.getClientServerKey(server))
	h.clients.Unlock()
}

func (h *Handlers) handlePingCommand() tele.HandlerFunc {
	return func(c tele.Context) error {
		if user := c.Sender(); user != nil {
			return c.Send(&tele.InputRichMessage{
				Markdown: fmt.Sprintf("pong, mr. with id `%d`", user.ID),
			})
		}

		return c.Send("pong")
	}
}

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

func (h *Handlers) handleManageServersCommand() tele.HandlerFunc {
	return func(c tele.Context) error {
		cfg := h.cfg.Config()

		if len(cfg.Servers) == 0 {
			return c.Send(&tele.InputRichMessage{
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

			btns = append(btns, markup.Data("🗑️ Remove \""+name+"\"", h.buttons.removeServer.Unique, name))
		}

		markup.Inline(markup.Split(2, btns)...) //nolint:mnd

		return c.Send(&tele.InputRichMessage{Markdown: b.String()}, markup)
	}
}

func (h *Handlers) handleRemoveServerCallback() tele.HandlerFunc {
	return func(c tele.Context) error {
		serverName := c.Data()

		if err := ValidateServerName(serverName); err != nil {
			return c.Send(&tele.InputRichMessage{
				Markdown:            "**Invalid server name**: " + EscapeMarkdown(err.Error()),
				SkipEntityDetection: true,
			})
		}

		var (
			found        bool
			serverConfig config.ConfigServer
		)

		h.cfg.Update(func(cfg *config.Config) {
			serverConfig, found = cfg.Servers[serverName]
			if !found {
				return
			}

			delete(cfg.Servers, serverName)
		})

		if !found {
			return c.Send(&tele.InputRichMessage{
				Markdown:            fmt.Sprintf("**Server `%s` does not exist**", serverName),
				SkipEntityDetection: true,
			})
		}

		if err := h.cfg.Save(); err != nil {
			// restore in-memory config to match disk
			h.cfg.Update(func(cfg *config.Config) { cfg.Servers[serverName] = serverConfig })

			h.log.Error("failed to save config after removing server",
				slog.String("error", err.Error()),
				slog.String("server_name", serverName),
			)

			return c.Send(&tele.InputRichMessage{
				Markdown:            "**Failed to save the config**: " + EscapeMarkdown(err.Error()),
				SkipEntityDetection: true,
			})
		}

		h.forgetClientForServer(serverConfig)

		return c.Send(&tele.InputRichMessage{
			Markdown:            fmt.Sprintf("**Server `%s` removed successfully**", serverName),
			SkipEntityDetection: true,
		})
	}
}

func (h *Handlers) handlePlayersCommand() tele.HandlerFunc {
	return func(c tele.Context) error {
		cfg := h.cfg.Config()

		if len(cfg.Servers) == 0 {
			return c.Send(&tele.InputRichMessage{
				Markdown: "**No servers configured yet**. Use /" + cmdAddServer + " to add one",
			})
		}

		var (
			markup = &tele.ReplyMarkup{}
			names  = slices.Sorted(maps.Keys(cfg.Servers)) // stable order
			btns   = make([]tele.Btn, 0, len(names))
		)

		for _, name := range names {
			btns = append(btns, markup.Data("🗺️ "+name, h.buttons.players.Unique, name))
		}

		markup.Inline(markup.Split(2, btns)...) //nolint:mnd

		return c.Send(&tele.InputRichMessage{Markdown: "Select a server to list its players online:"}, markup)
	}
}

func (h *Handlers) handlePlayersCallback(ctx context.Context) tele.HandlerFunc {
	return func(c tele.Context) error {
		serverName := c.Data()

		if err := ValidateServerName(serverName); err != nil {
			return c.Send(&tele.InputRichMessage{
				Markdown:            "**Invalid server name**: " + EscapeMarkdown(err.Error()),
				SkipEntityDetection: true,
			})
		}

		cfg := h.cfg.Config()

		serverConfig, exists := cfg.Servers[serverName]
		if !exists {
			return c.Send(&tele.InputRichMessage{
				Markdown:            fmt.Sprintf("**Server `%s` does not exist**", serverName),
				SkipEntityDetection: true,
			})
		}

		client := h.getClientForServer(serverConfig)

		players, cErr := client.Players(ctx)
		if cErr != nil {
			return c.Send(&tele.InputRichMessage{
				Markdown:            "**Failed to fetch players**: " + EscapeMarkdown(cErr.Error()),
				SkipEntityDetection: true,
			})
		}

		if len(*players) == 0 {
			return c.Send(&tele.InputRichMessage{
				Markdown: fmt.Sprintf("**No players online on server `%s`**", serverName),
			})
		}

		// sort players to get stable order
		sort.SliceStable(*players, func(i, j int) bool { return (*players)[i].ID.GUID < (*players)[j].ID.GUID })

		var (
			b          strings.Builder
			names      = make([]string, 0, len(*players))
			namesToIDs = make(map[string]string, len(*players))
		)

		b.Grow(64 + (len(*players) * 64)) //nolint:mnd // rough estimate

		b.WriteString("| Nickname | 💧  | 🍏  | 🩸  | ❤️  | Playtime | Kills |\n")
		b.WriteString("|:--------:|:---:|:---:|:---:|:---:|:--------:|:-----:|\n")

		for _, p := range *players {
			names, namesToIDs[p.ID.Name] = append(names, p.ID.Name), p.ID.SteamID

			b.WriteRune('`')
			b.WriteString(p.ID.Name)
			b.WriteRune('`')
			if p.State.Godmode {
				b.WriteString(" 👿")
			}
			b.WriteString(" | ")

			b.WriteString(strconv.Itoa(int(math.Round(p.Vitals.Water * 100)))) //nolint:mnd
			b.WriteString(" | ")

			b.WriteString(strconv.Itoa(int(math.Round(p.Vitals.Energy * 100)))) //nolint:mnd
			b.WriteString(" | ")

			b.WriteString(strconv.Itoa(int(math.Round(p.Vitals.Blood * 100)))) //nolint:mnd
			b.WriteString(" | ")

			b.WriteString(strconv.Itoa(int(math.Round(p.Vitals.Health * 100)))) //nolint:mnd
			b.WriteString(" | ")

			playtimeDays := p.Stats.PlaytimeSeconds / (60 * 60) //nolint:mnd
			b.WriteString(strconv.Itoa(int(playtimeDays)))
			b.WriteString(" hour")
			if playtimeDays != 1 {
				b.WriteString("s")
			}
			b.WriteString(" | ")

			if p.Stats.PlayersKilled > 0 {
				b.WriteString(strconv.Itoa(p.Stats.PlayersKilled))
				b.WriteString(" kill")
				if p.Stats.PlayersKilled != 1 {
					b.WriteRune('s')
				}
				b.WriteRune(' ')
			} else {
				b.WriteString(" - ")
			}

			b.WriteRune('\n')
		}

		var (
			markup = &tele.ReplyMarkup{}
			btns   = make([]tele.Btn, 0, len(names))
		)

		for _, name := range names {
			// payload (callback_data) will contain `<server_name>|<player_steam_id>`
			btns = append(btns, markup.Data("👤 "+name, h.buttons.player.Unique, serverName, namesToIDs[name]))
		}

		markup.Inline(markup.Split(3, btns)...) //nolint:mnd

		return c.Send(&tele.InputRichMessage{Markdown: b.String()}, markup)
	}
}

func (h *Handlers) handlePlayerCallback(ctx context.Context) tele.HandlerFunc {
	return func(c tele.Context) error {
		args := c.Args()
		if len(args) != 2 { //nolint:mnd
			return c.Send(&tele.InputRichMessage{Markdown: "**Invalid callback data**"})
		}

		serverName, playerSteamID := args[0], args[1]

		if err := ValidateServerName(serverName); err != nil {
			return c.Send(&tele.InputRichMessage{
				Markdown:            "**Invalid server name**: " + EscapeMarkdown(err.Error()),
				SkipEntityDetection: true,
			})
		}

		cfg := h.cfg.Config()

		serverConfig, exists := cfg.Servers[serverName]
		if !exists {
			return c.Send(&tele.InputRichMessage{
				Markdown:            fmt.Sprintf("**Server `%s` does not exist**", serverName),
				SkipEntityDetection: true,
			})
		}

		var (
			client = h.getClientForServer(serverConfig)
			wg     sync.WaitGroup

			players *api.Players
			pErr    error
			world   *api.World
			wErr    error
		)

		wg.Go(func() { players, pErr = client.Players(ctx) })
		wg.Go(func() { world, wErr = client.World(ctx) })

		wg.Wait()

		switch {
		case pErr != nil:
			return c.Send(&tele.InputRichMessage{
				Markdown:            "**Failed to fetch world info**: " + EscapeMarkdown(wErr.Error()),
				SkipEntityDetection: true,
			})
		case wErr != nil:
			return c.Send(&tele.InputRichMessage{
				Markdown:            "**Failed to fetch players**: " + EscapeMarkdown(pErr.Error()),
				SkipEntityDetection: true,
			})
		case players == nil || world == nil:
			return c.Send(&tele.InputRichMessage{Markdown: "**Fucking shit happened**"})
		}

		if len(*players) == 0 {
			return c.Send(&tele.InputRichMessage{
				Markdown: fmt.Sprintf("**No players online on server `%s`**", serverName),
			})
		}

		playerIdx := func() int {
			for i, p := range *players {
				if p.ID.SteamID == playerSteamID {
					return i
				}
			}

			return -1
		}()

		if playerIdx < 0 || playerIdx >= len(*players) {
			return c.Send(&tele.InputRichMessage{
				Markdown: fmt.Sprintf("**Player is no longer online on server `%s`**", serverName),
			})
		}

		playerData := (*players)[playerIdx]

		playerMap, mErr := func(mapName string) (image.Image, error) {
			var dayzMap gameMaps.Map

			switch mapName {
			case chernarusplus.Map.Name():
				dayzMap = chernarusplus.Map
			default:
				return nil, fmt.Errorf("unsupported map %s", mapName)
			}

			return gamemap.DrawPoint(dayzMap, gamemap.Point{
				PosX:        playerData.Pos.X,
				PosY:        playerData.Pos.Y,
				TopLabel:    playerData.ID.Name,
				BottomLabel: fmt.Sprintf("[%.2f, %.2f]", playerData.Pos.X, playerData.Pos.Y),
			})
		}(world.MapName)
		if mErr != nil {
			return c.Send(&tele.InputRichMessage{
				Markdown:            "**Failed to render player map**: " + EscapeMarkdown(mErr.Error()),
				SkipEntityDetection: true,
			})
		}

		pr, pw := io.Pipe()
		defer func() { _ = pr.Close() }() //nolint:errcheck

		go func() { _ = pw.CloseWithError(jpeg.Encode(pw, playerMap, &jpeg.Options{Quality: 90})) }() //nolint:mnd

		return c.Send(&tele.Photo{
			File:    tele.FromReader(pr),
			Caption: fmt.Sprintf("%+v", playerData),
		})
	}
}
