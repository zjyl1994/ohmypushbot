package httpapi

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/zjyl1994/ohmypushbot/internal/limiter"
	"github.com/zjyl1994/ohmypushbot/internal/store"
	"github.com/zjyl1994/ohmypushbot/tgmd"
)

func PushHandler(b *bot.Bot, s *store.Store, limiter *limiter.LRULimiter) gin.HandlerFunc {
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

		if !limiter.Allow(uint64(token)) {
			c.String(http.StatusTooManyRequests, "rate limit exceeded")
			return
		}

		chatID, ok, err := s.ResolveChatID(token)
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
		silent := query.Has("slient")
		var parseMode models.ParseMode
		if query.Has("mark") {
			parseMode = models.ParseModeMarkdown
			text = tgmd.SmartConvert(text)
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
