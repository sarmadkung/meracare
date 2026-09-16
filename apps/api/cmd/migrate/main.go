// Command migrate applies the embedded PostgreSQL migrations.
//
//	go run ./cmd/migrate up      apply pending migrations
//	go run ./cmd/migrate status  list migrations and whether they are applied
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/genxcare/api/internal/config"
	"github.com/genxcare/api/internal/database"
	"github.com/genxcare/api/pkg/logging"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "genxcare-migrate: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	command := "up"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}

	if err := config.LoadDotEnv(".env"); err != nil {
		return fmt.Errorf("load .env: %w", err)
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := logging.New(os.Stdout, logging.Options{
		Level:       cfg.LogLevel,
		Development: cfg.Env.IsDevelopment(),
		ServiceName: "genxcare-migrate",
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := database.Connect(ctx, database.Options{
		URL:      cfg.DatabaseURL,
		MaxConns: 2,
	})
	if err != nil {
		return err
	}
	defer pool.Close()

	switch command {
	case "up":
		if err := database.Migrate(ctx, pool, logger); err != nil {
			return err
		}
		logger.Info("migrations up to date")
		return nil

	case "status":
		statuses, err := database.Status(ctx, pool)
		if err != nil {
			return err
		}
		for _, status := range statuses {
			state := "pending"
			if status.Applied {
				state = "applied"
			}
			fmt.Printf("%04d  %-8s  %s\n", status.Version, state, status.Name)
		}
		return nil

	default:
		return fmt.Errorf("unknown command %q (expected: up, status)", command)
	}
}
