package telegram

import (
	"context"
	"log/slog"

	tele "gopkg.in/telebot.v4"

	"github.com/alexzulu/dayz-stats-tg-bot/cmd/tg-bot/app/telegram/clients"
	"github.com/alexzulu/dayz-stats-tg-bot/cmd/tg-bot/app/telegram/handlers/ping"
	"github.com/alexzulu/dayz-stats-tg-bot/cmd/tg-bot/app/telegram/handlers/players"
	"github.com/alexzulu/dayz-stats-tg-bot/cmd/tg-bot/app/telegram/handlers/servers"
	"github.com/alexzulu/dayz-stats-tg-bot/cmd/tg-bot/app/telegram/middleware"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/bot"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/config"
)

// RegisterHandlers wires every command and inline-button handler onto the bot, gating admin actions behind the
// auth middleware (ping stays public). It does not start polling - call bot.Start for that.
func RegisterHandlers(
	ctx context.Context,
	log *slog.Logger,
	cf *config.File,
	b *bot.Telegram,
	tgAdminIDs []int64,
) error {
	var (
		recoverMw = middleware.NewRecoverMiddleware(ctx, log)
		loggerMw  = middleware.NewLoggerMiddleware(ctx, log)
		authMw    = middleware.NewAuthMiddleware(ctx, log, tgAdminIDs)
		cp        = clients.NewPool(0) // 0 ttl - the pool falls back to the client's default cache window
	)

	const cmdAddServer = "add_server"

	// commands - advertised in the client menu, every one but ping is admin-gated
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
				Handler:     servers.NewCreateServerHandler(ctx, log, cmdAddServer, cf),
				Middleware:  []tele.MiddlewareFunc{authMw},
			},
			{
				Text:        "servers",
				Description: "Manage configured servers",
				Handler:     servers.NewManageServersHandler(cf),
				Middleware:  []tele.MiddlewareFunc{authMw},
			},
			{
				Text:        "players",
				Description: "List players online",
				Handler:     players.NewSelectServerHandler(cf),
				Middleware:  []tele.MiddlewareFunc{authMw},
			},
		},
		recoverMw,
		loggerMw,
	); err != nil {
		return err
	}

	// inline-button callbacks - AutoRespond acks every tap so the client stops showing the loading spinner
	if err := b.RegisterPrivateButtonHandlers(
		[]bot.Button{
			{
				Button:     &servers.RemoveServerBtn,
				Handler:    servers.NewRemoveServerBtnHandler(log, cf, cp),
				Middleware: []tele.MiddlewareFunc{authMw},
			},
			{
				Button:     &players.ServerPlayersListBtn,
				Handler:    players.NewServerPlayersListBtnHandler(ctx, log, cf, cp),
				Middleware: []tele.MiddlewareFunc{authMw},
			},
			{
				Button:     &players.ServerPlayerBtn,
				Handler:    players.NewServerPlayerBtnHandler(ctx, log, cf, cp),
				Middleware: []tele.MiddlewareFunc{authMw},
			},
		},
		recoverMw,
		loggerMw,
		middleware.NewAutoCallbackRespondMiddleware(),
	); err != nil {
		return err
	}

	return nil
}
