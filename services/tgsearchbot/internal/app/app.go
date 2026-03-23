package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	platformpg "github.com/faringet/telegram-bot-scraper/internal/platform/postgres"
	tgcfg "github.com/faringet/telegram-bot-scraper/services/tgsearchbot/config"
	"github.com/faringet/telegram-bot-scraper/services/tgsearchbot/internal/botapi"
	"github.com/faringet/telegram-bot-scraper/services/tgsearchbot/internal/bottext"
	"github.com/faringet/telegram-bot-scraper/services/tgsearchbot/internal/storage"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type App struct {
	cfg *tgcfg.TGSearchBot
	log *slog.Logger

	store    storage.Store
	bot      *botapi.Client
	searcher *Searcher
}

func New(cfg *tgcfg.TGSearchBot, log *slog.Logger) (*App, error) {
	if cfg == nil {
		return nil, errors.New("searchbot app: config is nil")
	}
	if log == nil {
		return nil, errors.New("searchbot app: logger is nil")
	}

	rootLog := log
	appLog := log.With(
		slog.String("layer", "app"),
		slog.String("module", "searchbot.app"),
	)

	st, err := openStore(cfg)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}

	b, err := botapi.New(cfg.TelegramBot, rootLog)
	if err != nil {
		_ = st.Close()
		return nil, fmt.Errorf("create bot client: %w", err)
	}

	searcher := NewSearcher(rootLog, st, SearcherConfig{
		DefaultLookback: cfg.Search.DefaultLookback,
		MaxResults:      cfg.Search.MaxResults,
		MaxQueryRunes:   cfg.Search.MaxQueryRunes,
		MaxTextRunes:    cfg.Search.MaxTextRunes,
	})

	return &App{
		cfg:      cfg,
		log:      appLog,
		store:    st,
		bot:      b,
		searcher: searcher,
	}, nil
}

func openStore(cfg *tgcfg.TGSearchBot) (storage.Store, error) {
	if cfg == nil {
		return nil, errors.New("searchbot app: config is nil")
	}
	if cfg.Storage.Driver != "postgres" {
		return nil, fmt.Errorf("unsupported storage driver for tgsearchbot: %s", cfg.Storage.Driver)
	}

	db, err := platformpg.Open(platformpg.Config{
		DSN:             cfg.Storage.Postgres.DSN,
		MaxOpenConns:    cfg.Storage.Postgres.MaxOpenConns,
		MaxIdleConns:    cfg.Storage.Postgres.MaxIdleConns,
		ConnMaxLifetime: cfg.Storage.Postgres.ConnMaxLifetime,
		ConnMaxIdleTime: cfg.Storage.Postgres.ConnMaxIdleTime,
	})
	if err != nil {
		return nil, fmt.Errorf("open postgres db: %w", err)
	}

	st, err := storage.NewPostgres(db)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create postgres storage: %w", err)
	}
	return st, nil
}

func (a *App) Close() error {
	if a == nil || a.store == nil {
		return nil
	}
	return a.store.Close()
}

func (a *App) Run(ctx context.Context) error {
	a.log.Info("run started",
		slog.String("storage_driver", a.cfg.Storage.Driver),
		slog.Duration("default_lookback", a.cfg.Search.DefaultLookback),
		slog.Int("max_results", a.cfg.Search.MaxResults),
		slog.Int("max_query_runes", a.cfg.Search.MaxQueryRunes),
		slog.Int("max_text_runes", a.cfg.Search.MaxTextRunes),
		slog.Int("allowed_user_ids_count", len(a.cfg.Access.AllowedUserIDs)),
		slog.Int("allowed_chat_ids_count", len(a.cfg.Access.AllowedChatIDs)),
	)

	if len(a.cfg.Access.AllowedUserIDs) == 0 && len(a.cfg.Access.AllowedChatIDs) == 0 {
		a.log.Warn("searchbot access is open to everyone")
	}

	if err := a.bot.Ping(ctx); err != nil {
		return err
	}

	return a.bot.Listen(ctx, a.handleUpdate)
}

func (a *App) handleUpdate(ctx context.Context, upd tgbotapi.Update) error {
	if upd.Message == nil {
		return nil
	}

	msg := upd.Message

	var fromID int64
	if msg.From != nil {
		fromID = msg.From.ID
	}

	if !a.isAllowed(fromID, msg.Chat.ID) {
		a.log.Warn("access denied",
			slog.Int64("from_id", fromID),
			slog.Int64("chat_id", msg.Chat.ID),
			slog.String("username", safeUsername(msg)),
		)

		denyMessage := strings.TrimSpace(a.cfg.Access.DenyMessage)
		if denyMessage != "" && msg.IsCommand() {
			return a.bot.SendHTML(ctx, msg.Chat.ID, denyMessage, true)
		}
		return nil
	}

	if !msg.IsCommand() {
		return nil
	}

	switch msg.Command() {
	case "start":
		return a.replyStart(ctx, msg.Chat.ID)

	case "help":
		return a.replyHelp(ctx, msg.Chat.ID)

	case "search":
		return a.replySearch(ctx, msg.Chat.ID, msg.CommandArguments())

	default:
		return a.bot.SendHTML(ctx, msg.Chat.ID, bottext.UnknownCommand, true)
	}
}

func (a *App) replyStart(ctx context.Context, chatID int64) error {
	return a.bot.SendHTML(ctx, chatID, bottext.Start(), true)
}

func (a *App) replyHelp(ctx context.Context, chatID int64) error {
	return a.bot.SendHTML(
		ctx,
		chatID,
		bottext.Help(a.cfg.Search.DefaultLookback, a.cfg.Search.MaxResults),
		true,
	)
}

func (a *App) replySearch(ctx context.Context, chatID int64, rawQuery string) error {
	rawQuery = strings.TrimSpace(rawQuery)
	if rawQuery == "" {
		return a.bot.SendHTML(ctx, chatID, bottext.SearchUsage, true)
	}

	results, err := a.searcher.Search(ctx, rawQuery)
	if err != nil {
		a.log.Error("search failed",
			slog.String("query", rawQuery),
			slog.Any("err", err),
		)
		return a.bot.SendHTML(ctx, chatID, bottext.SearchError, true)
	}

	if len(results) == 0 {
		return a.bot.SendHTML(ctx, chatID, bottext.NotFound(rawQuery), true)
	}

	header := bottext.ResultsHeader(rawQuery, len(results))
	if err := a.bot.SendHTML(ctx, chatID, header, true); err != nil {
		return err
	}

	for _, item := range results {
		if err := a.bot.SendHTML(ctx, chatID, item, true); err != nil {
			return err
		}
	}

	return nil
}

func safeUsername(msg *tgbotapi.Message) string {
	if msg == nil || msg.From == nil || strings.TrimSpace(msg.From.UserName) == "" {
		return ""
	}
	return "@" + msg.From.UserName
}
