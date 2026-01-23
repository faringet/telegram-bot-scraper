package mtproto

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"

	cfg "github.com/faringet/telegram-bot-scraper/pkg/config"
)

func authorizeIfNeeded(ctx context.Context, td *telegram.Client, c cfg.MTProto, log *slog.Logger) error {
	if td == nil {
		return errors.New("mtproto: telegram client is nil")
	}
	if log == nil {
		log = slog.Default()
	}

	phone := strings.TrimSpace(c.Phone)
	if phone == "" {
		phone = readLine("Enter phone: ")
	}
	if phone == "" {
		return errors.New("mtproto: phone is required")
	}

	codeAuth := auth.CodeAuthenticatorFunc(func(ctx context.Context, sent *tg.AuthSentCode) (string, error) {
		_ = sent
		code := readLine("Enter code: ")
		if code == "" {
			return "", errors.New("mtproto: empty code")
		}
		return code, nil
	})

	var ua auth.UserAuthenticator
	if strings.TrimSpace(c.Password) != "" {
		ua = auth.Constant(phone, c.Password, codeAuth)
	} else {
		ua = auth.CodeOnly(phone, codeAuth)
	}

	flow := auth.NewFlow(ua, auth.SendCodeOptions{})

	if err := td.Auth().IfNecessary(ctx, flow); err != nil {
		if errors.Is(err, auth.ErrPasswordNotProvided) {
			return errors.New("mtproto: 2FA password required; set mtproto.password and retry")
		}
		return fmt.Errorf("mtproto auth: %w", err)
	}

	log.Info("mtproto authorized", slog.String("session", c.Session))
	return nil
}

func readLine(prompt string) string {
	fmt.Print(prompt)
	r := bufio.NewReader(os.Stdin)
	s, _ := r.ReadString('\n')
	return strings.TrimSpace(s)
}
