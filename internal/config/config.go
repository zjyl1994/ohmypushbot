package config

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

// Config holds all runtime settings sourced from environment variables.
type Config struct {
	Debug          bool
	Token          string
	Addr           string
	BaseURL        string
	DBPath         string
	WebhookURL     string
	WebhookPath    string
	WebhookSecret  string
	WebhookEnabled bool
}

// Load reads configuration from environment variables and applies defaults.
func Load() (Config, error) {
	token := strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN"))
	if token == "" {
		return Config{}, errors.New("TELEGRAM_BOT_TOKEN is required")
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

	dbPath := strings.TrimSpace(os.Getenv("SQLITE_PATH"))
	if dbPath == "" {
		dbPath = "ohmypushbot.db"
	}

	webhookEnabled := parseEnvBool(os.Getenv("WEBHOOK_ENABLED"))
	webhookSecret := strings.TrimSpace(os.Getenv("WEBHOOK_SECRET_TOKEN"))
	if webhookEnabled && webhookSecret == "" {
		sum := sha256.Sum256([]byte(token))
		webhookSecret = hex.EncodeToString(sum[:])
	}

	cfg := Config{
		Debug:          parseEnvBool(os.Getenv("DEBUG")),
		Token:          token,
		Addr:           addr,
		BaseURL:        baseURL,
		DBPath:         dbPath,
		WebhookURL:     "",
		WebhookPath:    defaultWebhookPath + "/" + token,
		WebhookSecret:  webhookSecret,
		WebhookEnabled: webhookEnabled,
	}
	if webhookEnabled {
		cfg.WebhookURL = baseURL
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
