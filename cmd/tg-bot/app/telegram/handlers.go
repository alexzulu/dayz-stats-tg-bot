package telegram

import (
	"context"
	"log/slog"
	"sync"
	"time"

	tele "gopkg.in/telebot.v4"

	"github.com/alexzulu/dayz-stats-tg-bot/cmd/tg-bot/app/telegram/handlers/ping"
	"github.com/alexzulu/dayz-stats-tg-bot/cmd/tg-bot/app/telegram/handlers/servers"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/bot"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/config"
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
				Handler:     ping.NewHandler(),
			},
			{
				Text:        cmdAddServer,
				Description: "<server_name> <statez_base_url> <statez_auth_token> - add a new server",
				Handler:     servers.NewCreateServerHandler(ctx, h.log, cmdAddServer, h.cfg),
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
