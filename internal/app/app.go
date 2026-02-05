package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-telegram/bot"

	"github.com/zjyl1994/ohmypushbot/internal/config"
	"github.com/zjyl1994/ohmypushbot/internal/httpapi"
	"github.com/zjyl1994/ohmypushbot/internal/limiter"
	"github.com/zjyl1994/ohmypushbot/internal/store"
	"github.com/zjyl1994/ohmypushbot/internal/telegram"
	"github.com/zjyl1994/ohmypushbot/internal/vars"
)

const (
	limiterSize   = 10000
	sendWorkers   = 2
	sendQueueSize = 1024
	sendTimeout   = 10 * time.Second
)

func Run(ctx context.Context, cfg config.Config, log *slog.Logger) error {
	tStore, err := store.Open(cfg.DBPath, log, cfg.Debug)
	if err != nil {
		return fmt.Errorf("open sqlite failed: %w", err)
	}
	defer tStore.Close()
	vars.Store = tStore

	lim := limiter.NewLRULimiter[store.PushToken](limiterSize)
	vars.Limiter = lim

	botOptions := []bot.Option{
		bot.WithDefaultHandler(telegram.CommandHandler(cfg, tStore)),
	}
	if cfg.WebhookEnabled {
		botOptions = append(botOptions, bot.WithWebhookSecretToken(cfg.WebhookSecret))
	}

	tg, err := bot.New(cfg.Token, botOptions...)
	if err != nil {
		return fmt.Errorf("init bot failed: %w", err)
	}
	vars.Bot = tg
	vars.Sender = telegram.NewSender(ctx, tg, log, sendWorkers, sendQueueSize, sendTimeout)

	me, err := tg.GetMe(ctx)
	if err != nil {
		return fmt.Errorf("getMe failed: %w", err)
	}

	botUsername := strings.TrimSpace(me.Username)
	if botUsername == "" {
		return errors.New("getMe returned empty username")
	}

	gin.DefaultWriter = newSlogWriter(log, slog.LevelInfo, "gin")
	gin.DefaultErrorWriter = newSlogWriter(log, slog.LevelError, "gin")

	router := gin.New()
	router.Use(ginSlogMiddleware(log))
	router.Use(gin.RecoveryWithWriter(newSlogWriter(log, slog.LevelError, "gin")))
	router.GET("/", httpapi.RootRedirectHandler(botUsername))
	router.GET("/health", httpapi.HealthHandler())
	router.POST("/push/:token", httpapi.PushHandler())
	if cfg.WebhookEnabled {
		router.POST(cfg.WebhookPath, gin.WrapH(tg.WebhookHandler()))
	}

	if cfg.WebhookEnabled {
		if _, err := tg.SetWebhook(ctx, &bot.SetWebhookParams{
			URL:         cfg.WebhookURL + cfg.WebhookPath,
			SecretToken: cfg.WebhookSecret,
		}); err != nil {
			return fmt.Errorf("set webhook failed: %w", err)
		}
		maskedPath := cfg.WebhookPath
		if cfg.Token != "" {
			maskedPath = strings.ReplaceAll(maskedPath, cfg.Token, "***")
		}
		log.Info("webhook enabled", "url", cfg.WebhookURL, "path", maskedPath)
		go tg.StartWebhook(ctx)
	} else {
		if _, err := tg.DeleteWebhook(ctx, &bot.DeleteWebhookParams{
			DropPendingUpdates: true,
		}); err != nil {
			log.Warn("delete webhook failed", "error", err)
		}
		go tg.Start(ctx)
		log.Info("polling enabled")
	}

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()

	select {
	case <-ctx.Done():
		log.Info("shutting down server...")
	case err := <-serverErr:
		if err != nil {
			return fmt.Errorf("server error: %w", err)
		}
		return nil
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server forced to shutdown: %w", err)
	}

	if err := <-serverErr; err != nil {
		return fmt.Errorf("server error: %w", err)
	}

	log.Info("server exited")
	return nil
}
