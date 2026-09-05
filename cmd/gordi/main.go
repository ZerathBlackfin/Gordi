package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gordi/internal/api"
	"gordi/internal/app"
	"gordi/internal/config"
	"gordi/internal/store"
	"gordi/web"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel(),
	})))

	if err := run(); err != nil {
		slog.Error("stopping on error", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := config.Load()

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer st.Close()

	a := app.New(cfg, st)
	a.MB.SetContact(a.MBContact())
	a.MB.SetLang(a.Lang())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go a.Run(ctx)
	go a.Prefetch(ctx)

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           api.Handler(a, web.FS()),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdown); err != nil {
			slog.Error("shutting down", "err", err)
		}
	}()

	slog.Info("gordi starting", "address", cfg.Addr, "input", cfg.Input, "output", cfg.Output, "mode", cfg.Mode)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	slog.Info("gordi stopped")
	return nil
}

func logLevel() slog.Level {
	if os.Getenv("GORDI_DEBUG") != "" {
		return slog.LevelDebug
	}
	return slog.LevelInfo
}
