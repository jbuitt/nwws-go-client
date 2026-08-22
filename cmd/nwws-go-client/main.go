package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jbuitt/nwws-go-client/internal/config"
	"github.com/jbuitt/nwws-go-client/internal/nwwsclient"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	cfg, err := config.Load(os.Args[1:])
	if err != nil {
		logger.Error("configuration error", slog.Any("error", err))
		os.Exit(1)
	}

	panLogger := logger
	if cfg.PanRunLog != "" {
		f, err := os.OpenFile(cfg.PanRunLog, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			logger.Error("failed to open pan_run_log", slog.String("path", cfg.PanRunLog), slog.Any("error", err))
			os.Exit(1)
		}
		defer f.Close()
		panLogger = slog.New(slog.NewTextHandler(f, nil))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client := nwwsclient.New(cfg, logger, panLogger)
	if err := client.Run(ctx); err != nil {
		logger.Error("client exited with error", slog.Any("error", err))
		os.Exit(1)
	}
}
