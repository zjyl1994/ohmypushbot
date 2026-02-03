package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"strings"
)

const (
	defaultAddr        = "127.0.0.1:8000"
	defaultWebhookPath = "/webhook"
)

type config struct {
	token          string
	addr           string
	baseURL        string
	webhookURL     string
	webhookPath    string
	webhookSecret  string
	webhookEnabled bool
	dbPath         string
}

func loadConfig() (config, error) {
	token := strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN"))
	if token == "" {
		return config{}, errors.New("TELEGRAM_BOT_TOKEN is required")
	}

	addr := strings.TrimSpace(os.Getenv("WEB_ADDR"))
	if addr == "" {
		addr = defaultAddr
	}

	baseURL := strings.TrimSpace(os.Getenv("PUSH_BASE_URL"))
	if baseURL == "" {
		baseURL = addr
	}
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "http://" + baseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")

	webhookEnabled := parseEnvBool(os.Getenv("WEBHOOK_ENABLED"))
	webhookSecret := strings.TrimSpace(os.Getenv("WEBHOOK_SECRET_TOKEN"))
	if webhookEnabled && webhookSecret == "" {
		sum := sha256.Sum256([]byte(token))
		webhookSecret = hex.EncodeToString(sum[:])
	}

	dbPath := strings.TrimSpace(os.Getenv("SQLITE_PATH"))
	if dbPath == "" {
		dbPath = "push.db"
	}

	cfg := config{
		token:          token,
		addr:           addr,
		baseURL:        baseURL,
		webhookURL:     "",
		webhookPath:    defaultWebhookPath + "/" + token,
		webhookSecret:  webhookSecret,
		webhookEnabled: webhookEnabled,
		dbPath:         dbPath,
	}
	if webhookEnabled {
		cfg.webhookURL = baseURL
	}
	return cfg, nil
}

func parseEnvBool(value string) bool {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}
