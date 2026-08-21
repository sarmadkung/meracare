// Command api runs the MeraCare HTTP API.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	// Embeds the IANA timezone database in the binary. Care is scheduled in
	// each senior's own timezone, and a minimal container image ships no
	// tzdata — without this every zone would silently resolve to UTC and put
	// their tasks at the wrong hour.
	_ "time/tzdata"

	"github.com/meracare/api/internal/auth"
	"github.com/meracare/api/internal/config"
	"github.com/meracare/api/internal/database"
	"github.com/meracare/api/internal/notifications"
	"github.com/meracare/api/internal/server"
	"github.com/meracare/api/pkg/logging"
)

func main() {
	if err := run(); err != nil {
		// The logger may not exist yet, so report to stderr and exit non-zero.
		fmt.Fprintf(os.Stderr, "meracare-api: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Local development convenience; ignored when the file is absent.
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
		ServiceName: "meracare-api",
	})
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := database.Connect(ctx, database.Options{
		URL:      cfg.DatabaseURL,
		MaxConns: cfg.DatabaseMaxConns,
	})
	if err != nil {
		return err
	}
	defer pool.Close()
	logger.Info("connected to database")

	verifier, err := newVerifier(ctx, cfg, logger)
	if err != nil {
		return err
	}

	deps := server.Dependencies{
		Config:   cfg,
		Logger:   logger,
		Pool:     pool,
		Verifier: verifier,
	}
	handler := server.New(deps)

	// The notification scheduler. Started before the listener so a restart
	// delivers whatever fell due while the process was down, and stopped before
	// the pool closes so a pass in flight finishes rather than failing on a
	// closed connection (plans/phase11.md §36).
	var scheduler *notifications.Scheduler
	if cfg.NotificationSchedulerEnabled {
		scheduler = server.NewNotificationScheduler(deps)
		if err := scheduler.Start(ctx); err != nil {
			return fmt.Errorf("start notification scheduler: %w", err)
		}
	} else {
		logger.Warn("notification scheduler is disabled; " +
			"no notification will be created or delivered")
	}

	httpServer := &http.Server{
		Addr:              cfg.Addr(),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("api listening",
			slog.String("addr", httpServer.Addr),
			slog.String("env", string(cfg.Env)),
		)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
	}()

	select {
	case err := <-serverErrors:
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownGracePeriod)
	defer cancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}

	// A scheduler that does not stop cleanly leaves claimed notifications
	// leased, which delays them rather than losing them — but only a crash
	// should cost that, not a deploy.
	if scheduler != nil {
		if err := scheduler.Stop(shutdownCtx); err != nil {
			logger.Warn("notification scheduler did not stop cleanly", slog.Any("error", err))
		}
	}

	logger.Info("api stopped")
	return nil
}

// newVerifier builds the access-token verifier for the configured JWT mode.
//
// Asymmetric verification is the default: the API fetches Supabase's public
// signing keys and can verify tokens without ever being able to mint one.
func newVerifier(ctx context.Context, cfg *config.Config, logger *slog.Logger) (auth.Verifier, error) {
	issuer := auth.SupabaseIssuer(cfg.SupabaseURL)

	if cfg.SupabaseJWTMode == config.JWTModeLegacyHS256 {
		logger.Warn("verifying access tokens with the legacy shared JWT secret; " +
			"migrate the Supabase project to asymmetric signing keys when possible")
		return auth.NewHS256Verifier(auth.HS256VerifierOptions{
			Secret:   cfg.SupabaseJWTSecret,
			Audience: cfg.SupabaseJWTAudience,
			Issuer:   issuer,
			Leeway:   cfg.SupabaseJWTLeeway,
		})
	}

	verifier, err := auth.NewJWKSVerifier(auth.JWKSVerifierOptions{
		JWKSURL:  auth.SupabaseJWKSURL(cfg.SupabaseURL),
		Audience: cfg.SupabaseJWTAudience,
		Issuer:   issuer,
		Leeway:   cfg.SupabaseJWTLeeway,
	})
	if err != nil {
		return nil, err
	}

	// Prime the key cache so the first sign-in is not slowed by a fetch. A
	// failure here is not fatal: the key set is retried on demand, and the API
	// should still start if Supabase is briefly unreachable.
	warmupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := verifier.Warmup(warmupCtx); err != nil {
		logger.Warn("could not preload Supabase signing keys; will retry on first request",
			slog.Any("error", err))
	} else {
		logger.Info("loaded Supabase signing keys")
	}

	return verifier, nil
}
