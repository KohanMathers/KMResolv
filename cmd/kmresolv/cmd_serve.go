package main

import (
	"flag"
	"log"

	"github.com/kohanmathers/kmresolv/internal/config"
	"github.com/kohanmathers/kmresolv/internal/dashboard"
	"github.com/kohanmathers/kmresolv/internal/logger"
	"github.com/kohanmathers/kmresolv/internal/server"
)

func cmdServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	configPath := fs.String("config", "config.yml", "path to config file")
	fs.Parse(args)

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	logger.InitLogger(cfg.Server.LogLevel)
	logger.LogInfo("starting kmresolv on %s", cfg.Addr())

	srv := server.New(cfg)
	dashboard.Start(cfg, srv)

	if cfg.Minecraft.Enabled {
		logger.LogInfo("starting minecraft server on %s:%d", cfg.Minecraft.Listen, cfg.Minecraft.Port)
		if err := srv.StartMinecraft(); err != nil {
			logger.LogWarn("minecraft server failed to start: %v", err)
		}
	}

	if err := srv.Start(); err != nil {
		log.Fatal(err)
	}
}
