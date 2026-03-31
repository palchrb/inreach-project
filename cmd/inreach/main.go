package main

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/palchrb/inreach-project/internal/command"
	"github.com/palchrb/inreach-project/internal/config"
	"github.com/palchrb/inreach-project/internal/service"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "login":
		runLogin()
	case "run":
		runService()
	case "fetch-cabins":
		runFetchCabins()
	case "version":
		fmt.Println("inreach v0.1.0")
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage: inreach <command>")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  login         Register with Garmin Messenger via SMS OTP")
	fmt.Println("  run           Start the inReach assistant service")
	fmt.Println("  fetch-cabins  Download cabin data from UT.no to data/cabins.json")
	fmt.Println("  version       Print version")
}

func loadConfig() *config.Config {
	cfgPath := "config.yaml"
	if len(os.Args) > 2 {
		cfgPath = os.Args[2]
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}
	return cfg
}

func setupLogger(cfg *config.Config) *slog.Logger {
	var level slog.Level
	switch strings.ToLower(cfg.Log.Level) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	if cfg.Log.Pretty {
		handler = slog.NewTextHandler(os.Stderr, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	}

	return slog.New(handler)
}

func runLogin() {
	cfg := loadConfig()
	logger := setupLogger(cfg)

	svc := service.New(cfg, logger)
	auth := svc.Auth()

	ctx := context.Background()

	reader := bufio.NewReader(os.Stdin)

	fmt.Printf("Requesting OTP for %s...\n", cfg.Garmin.Phone)
	otpReq, err := auth.RequestOTP(ctx, cfg.Garmin.Phone, "inreach-assistant")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error requesting OTP: %v\n", err)
		os.Exit(1)
	}

	fmt.Print("Enter OTP code from SMS: ")
	code, _ := reader.ReadString('\n')
	code = strings.TrimSpace(code)

	if err := auth.ConfirmOTP(ctx, otpReq, code); err != nil {
		fmt.Fprintf(os.Stderr, "Error confirming OTP: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Login successful! Credentials saved.")
}

func runService() {
	cfg := loadConfig()
	logger := setupLogger(cfg)

	svc := service.New(cfg, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Resume session
	if err := svc.Resume(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error resuming session: %v\n", err)
		fmt.Fprintf(os.Stderr, "Run 'inreach login' first.\n")
		os.Exit(1)
	}

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		logger.Info("Received signal, shutting down", "signal", sig)
		cancel()
	}()

	logger.Info("Starting inReach assistant",
		"phone", cfg.Garmin.Phone,
		"charLimit", cfg.CharLimit,
	)

	if err := svc.Run(ctx); err != nil {
		if ctx.Err() == nil {
			fmt.Fprintf(os.Stderr, "Service error: %v\n", err)
			os.Exit(1)
		}
	}

	logger.Info("Service stopped")
}

func runFetchCabins() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	outputPath := "data/cabins.json"
	if len(os.Args) > 2 {
		outputPath = os.Args[2]
	}

	// Ensure data directory exists
	os.MkdirAll("data", 0o755)

	fmt.Printf("Fetching cabins from UT.no to %s...\n", outputPath)
	if err := command.FetchAndCacheCabins(logger, outputPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Done!")
}
