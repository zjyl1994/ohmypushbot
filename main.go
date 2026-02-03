package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-telegram/bot"
	_ "github.com/joho/godotenv/autoload"
)

func main() {
	logLevel := slog.LevelInfo
	if parseEnvBool("DEBUG") {
		logLevel = slog.LevelDebug
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				a.Value = slog.StringValue(a.Value.Time().Format(time.RFC3339))
			}
			return a
		},
	}))
	slog.SetDefault(logger)

	cfg, err := loadConfig()
	if err != nil {
		slog.Error("config error", "error", err)
		os.Exit(1)
	}

	store, err := openStore(cfg.dbPath)
	if err != nil {
		slog.Error("open sqlite failed", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	limiter := NewLRULimiter(10000)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	botOptions := []bot.Option{
		bot.WithDefaultHandler(botUpdateHandler(cfg, store)),
	}
	if cfg.webhookEnabled {
		botOptions = append(botOptions, bot.WithWebhookSecretToken(cfg.webhookSecret))
	}
	tg, err := bot.New(cfg.token, botOptions...)
	if err != nil {
		slog.Error("init bot failed", "error", err)
		os.Exit(1)
	}

	me, err := tg.GetMe(ctx)
	if err != nil {
		slog.Error("getMe failed", "error", err)
		os.Exit(1)
	}
	botUsername := strings.TrimSpace(me.Username)
	if botUsername == "" {
		slog.Error("getMe returned empty username")
		os.Exit(1)
	}

	router := gin.New()
	router.Use(gin.Recovery())
	router.GET("/", rootRedirectHandler(botUsername))
	router.POST("/push/:token", pushHandler(tg, store, limiter))
	if cfg.webhookEnabled {
		router.POST(cfg.webhookPath, gin.WrapH(tg.WebhookHandler()))
	}

	if cfg.webhookEnabled {
		if _, err := tg.SetWebhook(ctx, &bot.SetWebhookParams{
			URL:         cfg.webhookURL + cfg.webhookPath,
			SecretToken: cfg.webhookSecret,
		}); err != nil {
			slog.Error("set webhook failed", "error", err)
			os.Exit(1)
		}
		maskedPath := cfg.webhookPath
		if cfg.token != "" {
			maskedPath = strings.ReplaceAll(maskedPath, cfg.token, "***")
		}
		slog.Info("webhook enabled", "url", cfg.webhookURL, "path", maskedPath)
		go tg.StartWebhook(ctx)
	} else {
		if _, err := tg.DeleteWebhook(ctx, &bot.DeleteWebhookParams{
			DropPendingUpdates: true,
		}); err != nil {
			slog.Warn("delete webhook failed", "error", err)
		}
		go tg.Start(ctx)
		slog.Info("polling enabled")
	}

	srv := &http.Server{
		Addr:    cfg.addr,
		Handler: router,
	}

	go func() {
		slog.Info("listening", "addr", cfg.addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server forced to shutdown", "error", err)
		os.Exit(1)
	}

	slog.Info("server exited")
}
