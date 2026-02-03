package vars

import (
	"github.com/go-telegram/bot"

	"github.com/zjyl1994/ohmypushbot/internal/config"
	"github.com/zjyl1994/ohmypushbot/internal/limiter"
	"github.com/zjyl1994/ohmypushbot/internal/store"
)

var (
	Bot     *bot.Bot
	Store   *store.Store
	Limiter *limiter.LRULimiter[store.PushToken]
	Config  config.Config
)
