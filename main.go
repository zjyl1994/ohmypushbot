package main

import (
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

func main() {
	if err := loadDotEnv(".env"); err != nil {
		logrus.WithError(err).Warn("load .env")
	}

	logrus.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
	if level := strings.TrimSpace(os.Getenv("LOG_LEVEL")); level != "" {
		parsed, err := logrus.ParseLevel(level)
		if err != nil {
			logrus.WithError(err).Warn("invalid LOG_LEVEL, using info")
		} else {
			logrus.SetLevel(parsed)
		}
	}

	cfg, err := loadConfig()
	if err != nil {
		logrus.WithError(err).Fatal("config error")
	}

	api := telegramAPI{token: cfg.token}
	store, err := openStore(cfg.dbPath)
	if err != nil {
		logrus.WithError(err).Fatal("open sqlite failed")
	}
	defer store.Close()

	router := gin.New()
	router.Use(gin.Recovery())
	router.POST("/push/:token", pushHandler(api, store))
	router.POST(cfg.webhookPath, webhookHandler(api, cfg, store))

	server := &http.Server{
		Addr:              cfg.addr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	if cfg.webhookEnabled {
		if err := api.setWebhook(cfg.webhookURL+cfg.webhookPath, cfg.webhookSecret); err != nil {
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
	} else {
		if err := api.deleteWebhook(); err != nil {
			logrus.WithError(err).Warn("delete webhook failed")
		}
		go pollUpdates(api, cfg, store)
		logrus.Info("polling enabled")
	}

	logrus.WithField("addr", cfg.addr).Info("listening")
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logrus.WithError(err).Fatal("server error")
	}
}
