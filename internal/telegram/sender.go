package telegram

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type SendJob struct {
	ChatID              int64
	Text                string
	ParseMode           models.ParseMode
	DisableNotification bool
}

type Sender struct {
	bot     *bot.Bot
	log     *slog.Logger
	ctx     context.Context
	timeout time.Duration
	jobs    chan SendJob
	wg      sync.WaitGroup
}

func NewSender(ctx context.Context, botClient *bot.Bot, log *slog.Logger, workers int, queueSize int, timeout time.Duration) *Sender {
	if log == nil {
		log = slog.Default()
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if workers <= 0 {
		workers = 1
	}
	if queueSize <= 0 {
		queueSize = 1
	}

	s := &Sender{
		bot:     botClient,
		log:     log,
		ctx:     ctx,
		timeout: timeout,
		jobs:    make(chan SendJob, queueSize),
	}
	for i := 0; i < workers; i++ {
		s.wg.Add(1)
		go s.worker()
	}
	return s
}

func (s *Sender) Enqueue(job SendJob) bool {
	select {
	case <-s.ctx.Done():
		return false
	default:
	}
	select {
	case s.jobs <- job:
		return true
	default:
		return false
	}
}

func (s *Sender) worker() {
	defer s.wg.Done()
	for {
		select {
		case <-s.ctx.Done():
			return
		case job := <-s.jobs:
			s.send(job)
		}
	}
}

func (s *Sender) send(job SendJob) {
	if s.bot == nil {
		s.log.Warn("send skipped: bot unavailable")
		return
	}

	sendCtx := s.ctx
	var cancel context.CancelFunc
	if s.timeout > 0 {
		sendCtx, cancel = context.WithTimeout(s.ctx, s.timeout)
		defer cancel()
	}

	_, err := s.bot.SendMessage(sendCtx, &bot.SendMessageParams{
		ChatID:              job.ChatID,
		Text:                job.Text,
		ParseMode:           job.ParseMode,
		DisableNotification: job.DisableNotification,
	})
	if err != nil {
		s.log.Warn("send message failed", "error", err, "chat_id", job.ChatID)
	}
}
