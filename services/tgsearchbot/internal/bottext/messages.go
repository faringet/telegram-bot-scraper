package bottext

import (
	"fmt"
	"html"
	"strings"
	"time"
)

const (
	UnknownCommand = "🤔 Не знаю такой команды.\n\nПопробуй одну из этих:\n/start\n/help\n/search &lt;текст&gt;"
	SearchUsage    = "🔎 Просто напишите ниже, что хотите найти: название компании, фамилию или ключевое слово"
	SearchError    = "⚠️ Не удалось выполнить поиск прямо сейчас. Попробуйте чуть позже."
)

func Start() string {
	return strings.Join([]string{
		"👋 Привет! Я бот для поиска новостей по собранной базе.",
		"",
		"Вот что я умею:",
		"• /start — приветствие",
		"• /help — помощь",
		"• /search — поиск по новостям",
		"",
		"",
		"✨ Я покажу найденные новости в удобном формате.",
	}, "\n")
}

func Help(defaultLookback time.Duration, maxResults int) string {
	return strings.Join([]string{
		"🆘 <b>Помощь</b>",
		"",
		"Доступные команды:",
		"• /start — приветствие",
		"• /help — показать помощь",
		"• /search — поиск по новостям",
		"",
		"📌 По умолчанию я ищу за последние " + defaultLookback.String() + ".",
		fmt.Sprintf("📦 Максимум результатов за один запрос: %d.", maxResults),
		"",
	}, "\n")
}

func NotFound(rawQuery string) string {
	return fmt.Sprintf(
		"😕 Ничего не нашёл по запросу: <b>%s</b>\n\nПопробуйте сократить запрос или использовать другую формулировку.",
		html.EscapeString(rawQuery),
	)
}

func ResultsHeader(rawQuery string, count int) string {
	return fmt.Sprintf(
		"✅ Нашёл результатов: <b>%d</b>\n🔎 Запрос: <b>%s</b>",
		count,
		html.EscapeString(rawQuery),
	)
}
