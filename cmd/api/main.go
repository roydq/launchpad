package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/launchpad/launchpad/internal/api"
	"github.com/launchpad/launchpad/internal/auth"
	"github.com/launchpad/launchpad/internal/service"
	"github.com/launchpad/launchpad/internal/store"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	ctx := context.Background()

	databaseURL := envOr("LAUNCHPAD_DATABASE_URL", "file:launchpad.db?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	db, driver, err := store.Open(ctx, databaseURL)
	if err != nil {
		logger.Error("open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if os.Getenv("LAUNCHPAD_AUTO_MIGRATE") == "true" {
		if err := store.Migrate(ctx, db, driver); err != nil {
			logger.Error("migrate", "error", err)
			os.Exit(1)
		}
	}

	st := store.New(db, driver)
	authSvc := auth.NewService(st, os.Getenv("LAUNCHPAD_BOOTSTRAP_TOKEN"))
	projectSvc := service.NewProjectService(st)
	configSvc := service.NewConfigService(st, projectSvc)
	releaseSvc := service.NewReleaseService(st, projectSvc)
	changesetSvc := service.NewChangesetService(st, projectSvc, releaseSvc)
	server := api.NewServer(projectSvc, configSvc, releaseSvc, changesetSvc, authSvc, st)

	addr := envOr("LAUNCHPAD_API_ADDR", ":8080")
	httpServer := &http.Server{Addr: addr, Handler: server.Routes()}

	go func() {
		logger.Info("api listening", "addr", addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("listen", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}