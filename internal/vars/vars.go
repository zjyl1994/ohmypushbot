package vars

import (
	"github.com/go-telegram/bot"

	"github.com/zjyl1994/ohmypushbot/internal/config"
	"github.com/zjyl1994/ohmypushbot/internal/limiter"
	"github.com/zjyl1994/ohmypushbot/internal/store"
	"github.com/zjyl1994/ohmypushbot/internal/telegram"
)

var (
	Bot     *bot.Bot
	Store   *store.Store
	Limiter *limiter.LRULimiter[store.PushToken]
	Sender  *telegram.Sender
	Config  config.Config
)
