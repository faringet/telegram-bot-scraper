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
	if strings.TrimSpace(c.Token) == "" {
		return nil, errors.New("botapi: token is required")
	}
	log = log.With(slog.String("component", "tgnotifier.botapi"))

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

//func (c *Client) Username() string {
//	if c == nil || c.bot == nil || c.bot.Self.UserName == "" {
//		return ""
//	}
//	return "@" + c.bot.Self.UserName
//}

func (c *Client) SendText(ctx context.Context, chatID int64, text string, parseMode string, disablePreview bool) (messageID int, err error) {

	if chatID == 0 {
		return 0, errors.New("botapi: chatID is required")
	}
	text = strings.TrimSpace(text)

	msg := tgbotapi.NewMessage(chatID, text)
	msg.DisableWebPagePreview = disablePreview
	if parseMode != "" {
		msg.ParseMode = parseMode
	}

	type resp struct {
		m tgbotapi.Message
		e error
	}
	ch := make(chan resp, 1)

	go func() {
		m, e := c.bot.Send(msg)
		ch <- resp{m: m, e: e}
	}()

	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case r := <-ch:
		if r.e != nil {
			return 0, fmt.Errorf("botapi: send: %w", r.e)
		}
		return r.m.MessageID, nil
	}
}

func (c *Client) Ping(ctx context.Context) error {
	type resp struct {
		u tgbotapi.User
		e error
	}
	ch := make(chan resp, 1)
	go func() {
		u, e := c.bot.GetMe()
		ch <- resp{u: u, e: e}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case r := <-ch:
		if r.e != nil {
			return fmt.Errorf("botapi: getMe: %w", r.e)
		}
		c.log.Info("botapi ready",
			slog.String("username", "@"+r.u.UserName),
			slog.Int64("id", r.u.ID),
		)
		return nil
	}
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
