package httpapi

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/zjyl1994/ohmypushbot/internal/store"
	"github.com/zjyl1994/ohmypushbot/internal/telegram"
	"github.com/zjyl1994/ohmypushbot/internal/vars"
)

func PushHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
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
		if len(runes) > 4000 {
			remaining := len(runes) - 4000
			text = string(runes[:4000]) + fmt.Sprintf("... (%d characters remaining)", remaining)
		}

		query := c.Request.URL.Query()
		silent := query.Has("silent")
		var parseMode models.ParseMode
		if query.Has("mark") {
			parseMode = models.ParseModeMarkdown
			text = telegram.SmartConvert(text)
		}

		b := vars.Bot
		if b == nil {
			c.String(http.StatusServiceUnavailable, "bot unavailable")
			return
		}

		_, err = b.SendMessage(c.Request.Context(), &bot.SendMessageParams{
			ChatID:              chatID,
			Text:                text,
			ParseMode:           parseMode,
			DisableNotification: silent,
		})
		if err != nil {
			c.String(http.StatusBadGateway, "send failed: "+err.Error())
			return
		}

		c.Status(http.StatusNoContent)
	}
}

func RootRedirectHandler(botUsername string) gin.HandlerFunc {
	target := "https://t.me/" + strings.TrimPrefix(botUsername, "@")
	return func(c *gin.Context) {
		c.Redirect(http.StatusFound, target)
	}
}
