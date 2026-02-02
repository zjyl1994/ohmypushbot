package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

func pushHandler(api telegramAPI, store *tokenStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := strings.TrimSpace(c.Param("token"))
		if token == "" {
			c.String(http.StatusBadRequest, "invalid push path")
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

		body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
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
		parseMode := ""
		if query.Has("mark") {
			parseMode = "Markdown"
		}

		if err := api.sendMessage(chatID, text, parseMode, silent); err != nil {
			c.String(http.StatusBadGateway, "send failed: "+err.Error())
			return
		}

		c.Status(http.StatusNoContent)
	}
}

func webhookHandler(api telegramAPI, cfg config, store *tokenStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !cfg.webhookEnabled {
			c.String(http.StatusNotFound, "webhook disabled")
			return
		}
		if cfg.webhookSecret == "" {
			c.String(http.StatusForbidden, "webhook secret not configured")
			return
		}
		if c.GetHeader("X-Telegram-Bot-Api-Secret-Token") != cfg.webhookSecret {
			c.String(http.StatusForbidden, "invalid webhook secret")
			return
		}

		body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
		if err != nil {
			c.String(http.StatusBadRequest, "read body failed")
			return
		}

		var upd update
		if err := json.Unmarshal(body, &upd); err != nil {
			c.String(http.StatusBadRequest, "invalid update")
			return
		}

		handleUpdate(api, cfg, store, upd)
		c.Status(http.StatusNoContent)
	}
}

func pollUpdates(api telegramAPI, cfg config, store *tokenStore) {
	var offset int64
	for {
		updates, err := api.getUpdates(offset)
		if err != nil {
			logrus.WithError(err).Warn("getUpdates failed")
			time.Sleep(2 * time.Second)
			continue
		}
		for _, upd := range updates {
			handleUpdate(api, cfg, store, upd)
			if upd.UpdateID >= offset {
				offset = upd.UpdateID + 1
			}
		}
	}
}

func handleUpdate(api telegramAPI, cfg config, store *tokenStore, upd update) {
	if upd.Message == nil {
		return
	}

	cmd := parseCommand(upd.Message.Text)
	if cmd == "" {
		return
	}

	switch cmd {
	case "/start":
		token, err := store.getOrCreateToken(upd.Message.Chat.ID)
		if err != nil {
			logrus.WithError(err).Error("issue token failed")
			return
		}
		link := fmt.Sprintf("%s/push/%s", cfg.baseURL, token)
		reply := "Push URL:\n" + link + "\n\nQuery params:\n- slient: send silently\n- mark: Markdown message\n\nUse /revoke to disable this link."
		if err := api.sendMessage(upd.Message.Chat.ID, reply, "", false); err != nil {
			logrus.WithError(err).Warn("send start reply failed")
		}
	case "/revoke":
		_, err := store.revokeToken(upd.Message.Chat.ID)
		if err != nil {
			logrus.WithError(err).Error("revoke token failed")
			return
		}
		token, err := store.issueTokenForce(upd.Message.Chat.ID)
		if err != nil {
			logrus.WithError(err).Error("issue token failed")
			return
		}
		link := fmt.Sprintf("%s/push/%s", cfg.baseURL, token)
		reply := "Old link revoked. New URL:\n" + link + "\n\nQuery params:\n- slient: send silently\n- mark: Markdown message"
		if err := api.sendMessage(upd.Message.Chat.ID, reply, "", false); err != nil {
			logrus.WithError(err).Warn("send revoke reply failed")
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
