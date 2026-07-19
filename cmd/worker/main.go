package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/launchpad/launchpad/internal/jobs"
	"github.com/launchpad/launchpad/internal/secrets"
	"github.com/launchpad/launchpad/internal/store"
	"github.com/launchpad/launchpad/internal/target"
	k8starget "github.com/launchpad/launchpad/internal/target/kubernetes"
	"github.com/launchpad/launchpad/internal/target/stub"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	databaseURL := envOr("LAUNCHPAD_DATABASE_URL", "file:launchpad.db?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	db, driver, err := store.Open(ctx, databaseURL)
	if err != nil {
		logger.Error("open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	st := store.New(db, driver)
	secretsBox, err := secrets.LoadFromEnv()
	if err != nil {
		logger.Error("secrets key", "error", err)
		os.Exit(1)
	}
	if secretsBox != nil {
		st.WithSecrets(secretsBox)
		logger.Info("secrets encryption enabled")
	} else {
		logger.Warn("LAUNCHPAD_SECRETS_KEY unset; encrypted secret config will fail closed")
	}
	registry := target.NewRegistry()
	registry.Register(stub.New())

	if os.Getenv("LAUNCHPAD_ENABLE_KUBERNETES") != "false" {
		if k8s, err := k8starget.NewFromEnv(); err != nil {
			logger.Warn("kubernetes target unavailable", "error", err)
		} else {
			registry.Register(k8s)
			logger.Info("kubernetes target registered")
		}
	}

	workerID := envOr("LAUNCHPAD_WORKER_ID", "worker-1")
	worker := jobs.NewWorker(st, registry, workerID, logger)
	logger.Info("worker starting", "id", workerID)
	if err := worker.Run(ctx); err != nil && err != context.Canceled {
		logger.Error("worker stopped", "error", err)
		os.Exit(1)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}