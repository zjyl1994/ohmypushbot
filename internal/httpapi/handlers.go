package httpapi

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-telegram/bot/models"

	"github.com/zjyl1994/ohmypushbot/internal/store"
	"github.com/zjyl1994/ohmypushbot/internal/telegram"
	"github.com/zjyl1994/ohmypushbot/internal/vars"
)

func PushHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		const maxMessageRunes = 3000
		tokenStr := strings.TrimSpace(c.Param("token"))
		if tokenStr == "" {
			c.String(http.StatusBadRequest, "invalid push path")
			return
		}

		token, err := store.ParsePushToken(tokenStr)
		if err != nil {
			c.String(http.StatusBadRequest, "invalid token format")
			return
		}

		lim := vars.Limiter
		if lim == nil {
			c.String(http.StatusServiceUnavailable, "rate limiter unavailable")
			return
		}
		if !lim.Allow(token) {
			c.String(http.StatusTooManyRequests, "rate limit exceeded")
			return
		}

		store := vars.Store
		if store == nil {
			c.String(http.StatusServiceUnavailable, "store unavailable")
			return
		}

		chatID, ok, err := store.ResolveChatID(token)
		if err != nil {
			c.String(http.StatusBadGateway, "token validation failed")
			return
		}
		if !ok {
			c.String(http.StatusUnauthorized, "invalid token")
			return
		}

		body, err := io.ReadAll(io.LimitReader(c.Request.Body, 64*1024))
		if err != nil {
			c.String(http.StatusBadRequest, "read body failed")
			return
		}

		text := strings.TrimSpace(string(body))
		if text == "" {
			c.String(http.StatusBadRequest, "empty body")
			return
		}

		runes := []rune(text)
		if len(runes) > maxMessageRunes {
			remaining := len(runes) - maxMessageRunes
			suffix := fmt.Sprintf("... (%d characters remaining)", remaining)
			if len(suffix) >= maxMessageRunes {
				text = string(runes[:maxMessageRunes])
			} else {
				text = string(runes[:maxMessageRunes-len(suffix)]) + suffix
			}
		}

		query := c.Request.URL.Query()
		silent := query.Has("silent")
		var parseMode models.ParseMode
		if query.Has("mark") {
			parseMode = models.ParseModeMarkdown
			text = telegram.SmartConvert(text)
		}

		sender := vars.Sender
		if sender == nil {
			c.String(http.StatusServiceUnavailable, "sender unavailable")
			return
		}
		if !sender.Enqueue(telegram.SendJob{
			ChatID:              chatID,
			Text:                text,
			ParseMode:           parseMode,
			DisableNotification: silent,
		}) {
			c.String(http.StatusServiceUnavailable, "message queue full")
			return
		}

		c.Status(http.StatusAccepted)
	}
}

func RootRedirectHandler(botUsername string) gin.HandlerFunc {
	target := "https://t.me/" + strings.TrimPrefix(botUsername, "@")
	return func(c *gin.Context) {
		c.Redirect(http.StatusFound, target)
	}
}

func HealthHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		now := time.Now()
		c.JSON(http.StatusOK, gin.H{
			"server_time": now.Format(time.RFC3339),
			"timestamp":   now.Unix(),
		})
	}
}
