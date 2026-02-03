package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-telegram/bot"
	_ "github.com/joho/godotenv/autoload"
	"github.com/sirupsen/logrus"
)

func main() {
	logrus.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
	if parseEnvBool("DEBUG") {
		logrus.SetLevel(logrus.DebugLevel)
	}

	cfg, err := loadConfig()
	if err != nil {
		logrus.WithError(err).Fatal("config error")
	}

	store, err := openStore(cfg.dbPath)
	if err != nil {
		logrus.WithError(err).Fatal("open sqlite failed")
	}
	defer store.Close()

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
		logrus.WithError(err).Fatal("init bot failed")
	}

	me, err := tg.GetMe(ctx)
	if err != nil {
		logrus.WithError(err).Fatal("getMe failed")
	}
	botUsername := strings.TrimSpace(me.Username)
	if botUsername == "" {
		logrus.Fatal("getMe returned empty username")
	}

	router := gin.New()
	router.Use(gin.Recovery())
	router.GET("/", rootRedirectHandler(botUsername))
	router.POST("/push/:token", pushHandler(tg, store))
	if cfg.webhookEnabled {
		router.POST(cfg.webhookPath, gin.WrapH(tg.WebhookHandler()))
	}

	if cfg.webhookEnabled {
		if _, err := tg.SetWebhook(ctx, &bot.SetWebhookParams{
			URL:         cfg.webhookURL + cfg.webhookPath,
			SecretToken: cfg.webhookSecret,
		}); err != nil {
			logrus.WithError(err).Fatal("set webhook failed")
		}
		maskedPath := cfg.webhookPath
		if cfg.token != "" {
			maskedPath = strings.ReplaceAll(maskedPath, cfg.token, "***")
		}
		logrus.WithFields(logrus.Fields{
			"url":  cfg.webhookURL,
			"path": maskedPath,
		}).Info("webhook enabled")
		go tg.StartWebhook(ctx)
	} else {
		if _, err := tg.DeleteWebhook(ctx, &bot.DeleteWebhookParams{
			DropPendingUpdates: true,
		}); err != nil {
			logrus.WithError(err).Warn("delete webhook failed")
		}
		go tg.Start(ctx)
		logrus.Info("polling enabled")
	}

	srv := &http.Server{
		Addr:    cfg.addr,
		Handler: router,
	}

	go func() {
		logrus.WithField("addr", cfg.addr).Info("listening")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logrus.WithError(err).Fatal("server error")
		}
	}()

	<-ctx.Done()
	logrus.Info("shutting down server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logrus.WithError(err).Fatal("server forced to shutdown")
	}

	logrus.Info("server exited")
}
