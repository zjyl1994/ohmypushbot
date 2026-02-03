package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/zjyl1994/ohmypushbot/internal/config"
	"github.com/zjyl1994/ohmypushbot/internal/store"
)

func CommandHandler(cfg config.Config, s *store.Store) bot.HandlerFunc {
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
			token, err := s.GetOrCreateToken(update.Message.Chat.ID)
			if err != nil {
				slog.Error("issue token failed", "error", err)
				return
			}
			link := fmt.Sprintf("%s/push/%s", cfg.BaseURL, token)
			reply := "Push URL:\n" + link + "\n\nQuery params:\n- silent: send silently\n- mark: Markdown message\n\nUse /revoke to disable this link."
			if _, err := b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: update.Message.Chat.ID,
				Text:   reply,
			}); err != nil {
				slog.Warn("send start reply failed", "error", err)
			}
		case "/revoke":
			token, err := s.IssueTokenForce(update.Message.Chat.ID)
			if err != nil {
				slog.Error("issue token failed", "error", err)
				return
			}
			link := fmt.Sprintf("%s/push/%s", cfg.BaseURL, token)
			reply := "Old link revoked. New URL:\n" + link + "\n\nQuery params:\n- silent: send silently\n- mark: Markdown message"
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
