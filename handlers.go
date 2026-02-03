package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func pushHandler(b *bot.Bot, store *tokenStore, limiter *LRULimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr := strings.TrimSpace(c.Param("token"))
		if tokenStr == "" {
			c.String(http.StatusBadRequest, "invalid push path")
			return
		}

		token, err := ParsePushToken(tokenStr)
		if err != nil {
			c.String(http.StatusBadRequest, "invalid token format")
			return
		}

		if !limiter.Allow(uint64(token)) {
			c.String(http.StatusTooManyRequests, "rate limit exceeded")
			return
		}

		chatID, ok, err := store.resolveChatID(token)
		if err != nil {
			c.String(http.StatusBadGateway, "token validation failed")
			return
		}
		if !ok {
			c.String(http.StatusUnauthorized, "invalid token")
			return
		}

		body, err := io.ReadAll(io.LimitReader(c.Request.Body, 10*1024))
		if err != nil {
			c.String(http.StatusBadRequest, "read body failed")
			return
		}

		text := strings.TrimSpace(string(body))
		if text == "" {
			c.String(http.StatusBadRequest, "empty body")
			return
		}

		query := c.Request.URL.Query()
		silent := query.Has("slient")
		var parseMode models.ParseMode
		if query.Has("mark") {
			parseMode = models.ParseModeMarkdown
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

func rootRedirectHandler(botUsername string) gin.HandlerFunc {
	target := "https://t.me/" + strings.TrimPrefix(botUsername, "@")
	return func(c *gin.Context) {
		c.Redirect(http.StatusFound, target)
	}
}

func botUpdateHandler(cfg config, store *tokenStore) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		if update == nil || update.Message == nil {
			return
		}

		cmd := parseCommand(update.Message.Text)
		if cmd == "" {
			return
		}

		switch cmd {
		case "/start":
			token, err := store.getOrCreateToken(update.Message.Chat.ID)
			if err != nil {
				slog.Error("issue token failed", "error", err)
				return
			}
			link := fmt.Sprintf("%s/push/%s", cfg.baseURL, token)
			reply := "Push URL:\n" + link + "\n\nQuery params:\n- slient: send silently\n- mark: Markdown message\n\nUse /revoke to disable this link."
			if _, err := b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: update.Message.Chat.ID,
				Text:   reply,
			}); err != nil {
				slog.Warn("send start reply failed", "error", err)
			}
		case "/revoke":
			_, err := store.revokeToken(update.Message.Chat.ID)
			if err != nil {
				slog.Error("revoke token failed", "error", err)
				return
			}
			token, err := store.issueTokenForce(update.Message.Chat.ID)
			if err != nil {
				slog.Error("issue token failed", "error", err)
				return
			}
			link := fmt.Sprintf("%s/push/%s", cfg.baseURL, token)
			reply := "Old link revoked. New URL:\n" + link + "\n\nQuery params:\n- slient: send silently\n- mark: Markdown message"
			if _, err := b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: update.Message.Chat.ID,
				Text:   reply,
			}); err != nil {
				slog.Warn("send revoke reply failed", "error", err)
			}
		}
	}
}

func parseCommand(text string) string {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) == 0 {
		return ""
	}
	cmd := fields[0]
	if strings.HasPrefix(cmd, "/") {
		if at := strings.Index(cmd, "@"); at != -1 {
			cmd = cmd[:at]
		}
	}
	return cmd
}
