package botapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	cfg "github.com/faringet/telegram-bot-scraper/pkg/config"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Client struct {
	log         *slog.Logger
	bot         *tgbotapi.BotAPI
	pollTimeout int
}

func New(c cfg.TelegramBot, log *slog.Logger) (*Client, error) {
	if log == nil {
		log = slog.Default()
	}
	if strings.TrimSpace(c.Token) == "" {
		return nil, errors.New("searchbot botapi: token is required")
	}

	log = log.With(
		slog.String("layer", "transport"),
		slog.String("module", "searchbot.botapi"),
	)

	bot, err := tgbotapi.NewBotAPI(c.Token)
	if err != nil {
		return nil, fmt.Errorf("searchbot botapi: init: %w", err)
	}
	bot.Debug = c.Debug

	timeout := int(c.PollTimeout.Seconds())
	if timeout <= 0 {
		timeout = 30
	}

	return &Client{
		log:         log,
		bot:         bot,
		pollTimeout: timeout,
	}, nil
}

func (c *Client) Ping(ctx context.Context) error {
	if c == nil || c.bot == nil {
		return errors.New("searchbot botapi: client is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	u, err := c.bot.GetMe()
	if err != nil {
		return fmt.Errorf("searchbot botapi: getMe: %w", err)
	}

	c.log.Info("botapi ready",
		slog.String("username", "@"+u.UserName),
		slog.Int64("id", u.ID),
	)
	return nil
}

func (c *Client) SendHTML(ctx context.Context, chatID int64, text string, disablePreview bool) error {
	if c == nil || c.bot == nil {
		return errors.New("searchbot botapi: client is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if chatID == 0 {
		return errors.New("searchbot botapi: chatID is required")
	}

	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("searchbot botapi: text is required")
	}

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "HTML"
	msg.DisableWebPagePreview = disablePreview

	if _, err := c.bot.Send(msg); err != nil {
		return fmt.Errorf("searchbot botapi: send: %w", err)
	}
	return nil
}

func (c *Client) Listen(ctx context.Context, handler func(context.Context, tgbotapi.Update) error) error {
	if c == nil || c.bot == nil {
		return errors.New("searchbot botapi: client is nil")
	}
	if handler == nil {
		return errors.New("searchbot botapi: handler is nil")
	}

	u := tgbotapi.NewUpdate(0)
	u.Timeout = c.pollTimeout

	updates := c.bot.GetUpdatesChan(u)
	defer c.bot.StopReceivingUpdates()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case upd, ok := <-updates:
			if !ok {
				return nil
			}
			if err := handler(ctx, upd); err != nil {
				c.log.Warn("update handler failed", slog.Any("err", err))
			}
		}
	}
}
