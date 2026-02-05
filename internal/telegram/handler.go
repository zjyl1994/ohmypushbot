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

		msg := update.Message
		cmd := parseCommand(msg.Text)
		if cmd == "" {
			return
		}

		switch cmd {
		case "/start":
			handleStart(ctx, b, msg, cfg, s)
		case "/revoke":
			handleRevoke(ctx, b, msg, cfg, s)
		case "/ping":
			handlePing(ctx, b, msg)
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

func handleStart(ctx context.Context, b *bot.Bot, msg *models.Message, cfg config.Config, s *store.Store) {
	token, err := s.GetOrCreateToken(msg.Chat.ID)
	if err != nil {
		slog.Error("issue token failed", "error", err)
		return
	}
	link := fmt.Sprintf("%s/push/%s", cfg.BaseURL, token)
	reply := "Push URL:\n" + link + "\n\nQuery params:\n- silent: send silently\n- mark: Markdown message\n\nUse /revoke to disable this link."
	if _, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   reply,
	}); err != nil {
		slog.Warn("send start reply failed", "error", err)
	}
}

func handleRevoke(ctx context.Context, b *bot.Bot, msg *models.Message, cfg config.Config, s *store.Store) {
	token, err := s.IssueTokenForce(msg.Chat.ID)
	if err != nil {
		slog.Error("issue token failed", "error", err)
		return
	}
	link := fmt.Sprintf("%s/push/%s", cfg.BaseURL, token)
	reply := "Old link revoked. New URL:\n" + link + "\n\nQuery params:\n- silent: send silently\n- mark: Markdown message"
	if _, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   reply,
	}); err != nil {
		slog.Warn("send revoke reply failed", "error", err)
	}
}

func handlePing(ctx context.Context, b *bot.Bot, msg *models.Message) {
	if _, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   "Pong!",
		ReplyParameters: &models.ReplyParameters{
			MessageID: msg.ID,
			ChatID:    msg.Chat.ID,
		},
	}); err != nil {
		slog.Warn("send ping reply failed", "error", err)
	}
}
