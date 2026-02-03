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
)

const limiterSize = 10000

func Run(ctx context.Context, cfg config.Config, log *slog.Logger) error {
	tStore, err := store.Open(cfg.DBPath, log, cfg.Debug)
	if err != nil {
		return fmt.Errorf("open sqlite failed: %w", err)
	}
	defer tStore.Close()

	lim := limiter.NewLRULimiter(limiterSize)

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

	me, err := tg.GetMe(ctx)
	if err != nil {
		return fmt.Errorf("getMe failed: %w", err)
	}

	botUsername := strings.TrimSpace(me.Username)
	if botUsername == "" {
		return errors.New("getMe returned empty username")
	}

	router := gin.New()
	router.Use(gin.Recovery())
	router.GET("/", httpapi.RootRedirectHandler(botUsername))
	router.POST("/push/:token", httpapi.PushHandler(tg, tStore, lim))
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
		Addr:    cfg.Addr,
		Handler: router,
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
