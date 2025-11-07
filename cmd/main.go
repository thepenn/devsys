package main

import (
	"context"

	_ "github.com/joho/godotenv/autoload"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"

	"github.com/thepenn/devsys/cmd/wire"
	"github.com/thepenn/devsys/internal/config"
	"github.com/thepenn/devsys/internal/logger"
	"github.com/thepenn/devsys/internal/utils"
)

func main() {
	// 读取配置
	cfg, err := config.Environ()
	if err != nil {
		log.Fatal().Err(err).Msg("get config error")
	}

	// 配置程序ctx
	ctx := utils.WithContext(context.Background())

	// 初始化日志
	if err := logger.InitLogging(cfg.Logging.Level, cfg.Logging.Pretty, true); err != nil {
		log.Fatal().Err(err).Msg("init logger error")
	}

	app, err := wire.WireApp(&cfg)
	if err != nil {
		log.Error().Err(err).Msg("WireApp error")
		return
	}
	defer func() {
		if err := app.Close(); err != nil {
			log.Error().Err(err).Msg("app close error")
		}
	}()

	if err := app.Start(ctx); err != nil {
		log.Fatal().Err(err).Msg("failed to start services")
	}

	g := errgroup.Group{}

	// 启动 HTTP Server
	g.Go(func() error {
		log.Info().Str("addr", cfg.Server.Host).Msg("Starting HTTP server")
		return app.HttpServer.ListenAndServe(ctx)
	})

	if err := g.Wait(); err != nil {
		log.Error().Err(err).Msg("Server stop error")
	}
}
