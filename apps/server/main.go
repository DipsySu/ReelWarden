package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/reelwarden/reelwarden/internal/api"
	"github.com/reelwarden/reelwarden/internal/compliance"
	"github.com/reelwarden/reelwarden/internal/config"
	"github.com/reelwarden/reelwarden/internal/database"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to YAML config")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("configuration failed", "error", err)
		os.Exit(1)
	}
	gate := compliance.EvaluateTMDBAI(compliance.RuntimeInputs{TMDBEnabled: cfg.Metadata.Providers.TMDB.Enabled, AIEnabled: cfg.AI.Enabled, TMDBAIStatus: cfg.Compliance.TMDBAI})
	if !gate.Allowed {
		slog.Error("compliance gate blocked startup", "gate_id", gate.GateID, "error_code", gate.ErrorCode, "reason", gate.Reason)
		os.Exit(1)
	}

	ctx := context.Background()
	db, err := database.Open(ctx, cfg.Database.Path, cfg.Database.WAL, cfg.Database.MaxOpenConns)
	if err != nil {
		slog.Error("database failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	srv := &http.Server{Addr: cfg.Server.Listen, Handler: api.NewServer(cfg, db).Handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		slog.Info("reelwarden server listening", "addr", cfg.Server.Listen)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
