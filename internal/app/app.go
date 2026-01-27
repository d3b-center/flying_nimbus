package app

import (
	"context"
	aws "flying_nimbus/internal/providers/aws/backend"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

type App struct {
	Logger     *slog.Logger
	AWS        *aws.AwsService
	Context    context.Context
	cancel     context.CancelFunc
	cleanupLog func() error
}

func (a App) Shutdown() error {
	a.cancel()

	if a.cleanupLog != nil {
		return a.cleanupLog()
	}

	return nil
}

func InitApp(verbose bool) (*App, error) {
	ctx, cancel := context.WithCancel(context.Background())

	logger, cleanupLog, err := InitLogger(verbose)
	if err != nil {
		cancel()
		return nil, err
	}

	slog.SetDefault(logger)

	awsSvc, err := aws.InitAwsService(ctx)
	if err != nil {
		cancel()
		return nil, err
	}
	
	slog.Debug()

	return &App{
		AWS:        awsSvc,
		Context:    ctx,
		cancel:     cancel,
		cleanupLog: cleanupLog,
	}, nil
}

func InitLogger(verbose bool) (*slog.Logger, func() error, error) {
	homeDir, err := os.UserHomeDir()

	if err != nil {
		return nil, nil, err
	}

	logDir := filepath.Join(homeDir, ".flying_nimbus", "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, nil, err
	}

	ts := time.Now().UTC().Format("20060102-150405")
	logPath := filepath.Join(logDir, fmt.Sprintf("flying-nimbus-%s.log", ts))

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, err
	}

	logLevel := slog.LevelInfo
	if verbose {
		logLevel = slog.LevelDebug
	}
	handler := slog.NewTextHandler(f, &slog.HandlerOptions{
		Level: logLevel,
	})

	logger := slog.New(handler)

	// Return a cleanup function so main() can defer it
	cleanup := func() error {
		return f.Close()
	}

	return logger, cleanup, nil
}
