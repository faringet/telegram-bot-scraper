package botapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	cfg "github.com/faringet/telegram-bot-scraper/pkg/config"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Client struct {
	log *slog.Logger
	bot *tgbotapi.BotAPI
}

func New(c cfg.TelegramBot, log *slog.Logger) (*Client, error) {
	if log == nil {
		log = slog.Default()
	}
	if strings.TrimSpace(c.Token) == "" {
		return nil, errors.New("botapi: token is required")
	}

	log = log.With(
		slog.String("layer", "transport"),
		slog.String("module", "notifier.botapi"),
	)

	bot, err := tgbotapi.NewBotAPI(c.Token)
	if err != nil {
		return nil, fmt.Errorf("botapi: init: %w", err)
	}

	bot.Debug = c.Debug

	return &Client{
		log: log,
		bot: bot,
	}, nil
}

func (c *Client) SendText(ctx context.Context, chatID int64, text string, parseMode string, disablePreview bool) (int, error) {
	if c == nil || c.bot == nil {
		return 0, errors.New("botapi: client is nil")
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if chatID == 0 {
		return 0, errors.New("botapi: chatID is required")
	}

	text = strings.TrimSpace(text)
	if text == "" {
		return 0, errors.New("botapi: text is required")
	}

	msg := tgbotapi.NewMessage(chatID, text)
	msg.DisableWebPagePreview = disablePreview
	if parseMode != "" {
		msg.ParseMode = parseMode
	}

	sent, err := c.bot.Send(msg)
	if err != nil {
		return 0, fmt.Errorf("botapi: send: %w", err)
	}

	return sent.MessageID, nil
}

func (c *Client) Ping(ctx context.Context) error {
	if c == nil || c.bot == nil {
		return errors.New("botapi: client is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	u, err := c.bot.GetMe()
	if err != nil {
		return fmt.Errorf("botapi: getMe: %w", err)
	}

	c.log.Info("botapi ready",
		slog.String("username", "@"+u.UserName),
		slog.Int64("id", u.ID),
	)
	return nil
}

func SleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}

	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
