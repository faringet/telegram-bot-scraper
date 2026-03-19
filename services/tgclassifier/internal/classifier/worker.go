package classifier

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/faringet/telegram-bot-scraper/services/tgclassifier/internal/storage"
)

// OllamaClient is a minimal interface for an Ollama chat/generate client.
type OllamaClient interface {
	// Classify must return raw model text.
	Classify(ctx context.Context, model string, prompt string) (string, error)
}

type Config struct {
	Interval        time.Duration
	BatchSize       int
	Lease           time.Duration
	WorkerID        string
	Model           string
	MaxTextRunes    int
	MaxRetries      int
	RetryBackoff    time.Duration
	OnlyUndelivered bool
	WhitelistPath   string
}

type Worker struct {
	log    *slog.Logger
	cfg    Config
	store  storage.Store
	ollama OllamaClient

	whitelist []string
}

func NewWorker(log *slog.Logger, cfg Config, st storage.Store, oc OllamaClient) (*Worker, error) {
	if log == nil {
		log = slog.Default()
	}
	if st == nil {
		return nil, errors.New("classifier worker: store is nil")
	}
	if oc == nil {
		return nil, errors.New("classifier worker: ollama client is nil")
	}

	if cfg.Interval <= 0 {
		cfg.Interval = 5 * time.Second
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 20
	}
	if cfg.Lease <= 0 {
		cfg.Lease = 2 * time.Minute
	}
	if cfg.WorkerID == "" {
		cfg.WorkerID = fmt.Sprintf("tgclassifier-%d", time.Now().UnixNano())
	}
	if cfg.Model == "" {
		cfg.Model = "qwen2.5:7b"
	}
	if cfg.MaxTextRunes <= 0 {
		cfg.MaxTextRunes = 1800
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 3
	}
	if cfg.RetryBackoff <= 0 {
		cfg.RetryBackoff = 750 * time.Millisecond
	}

	whitelist, err := buildWhitelist(cfg.WhitelistPath)
	if err != nil {
		return nil, fmt.Errorf("classifier worker: build whitelist: %w", err)
	}

	baseLog := log.With("component", "classifier.worker", "worker_id", cfg.WorkerID)
	baseLog.Info("whitelist loaded", "count", len(whitelist), "path", cfg.WhitelistPath)

	return &Worker{
		log:       baseLog,
		cfg:       cfg,
		store:     st,
		ollama:    oc,
		whitelist: whitelist,
	}, nil
}

func buildWhitelist(path string) ([]string, error) {
	if strings.TrimSpace(path) != "" {
		wl, err := loadWhitelist(path)
		if err != nil {
			return nil, err
		}
		return wl, nil
	}

	return parseWhitelistFromRaw(dirtyTop250Raw), nil
}

func loadWhitelist(path string) ([]string, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open whitelist file: %w", err)
	}
	defer f.Close()

	out := make([]string, 0, 256)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		norm := normalizeCompany(line)
		if norm == "" {
			continue
		}
		out = append(out, norm)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("scan whitelist file: %w", err)
	}

	return uniqueStrings(out), nil
}

func normalizeCompany(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.NewReplacer(`"`, " ", `“`, " ", `”`, " ", `«`, " ", `»`, " ").Replace(s)

	b := make([]rune, 0, len([]rune(s)))
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b = append(b, r)
		} else {
			b = append(b, ' ')
		}
	}

	return strings.Join(strings.Fields(string(b)), " ")
}

func uniqueStrings(in []string) []string {
	m := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := m[s]; ok {
			continue
		}
		m[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func (w *Worker) Run(ctx context.Context) error {
	w.log.Info("worker started",
		"interval", w.cfg.Interval.String(),
		"batch_size", w.cfg.BatchSize,
		"lease", w.cfg.Lease.String(),
		"model", w.cfg.Model,
		"max_retries", w.cfg.MaxRetries,
		"only_undelivered", w.cfg.OnlyUndelivered,
	)

	t := time.NewTicker(w.cfg.Interval)
	defer t.Stop()

	if err := w.tick(ctx); err != nil && !errors.Is(err, context.Canceled) {
		w.log.Warn("initial tick error", "err", err)
	}

	for {
		select {
		case <-ctx.Done():
			w.log.Info("worker stopped", "reason", ctx.Err())
			return ctx.Err()
		case <-t.C:
			if err := w.tick(ctx); err != nil && !errors.Is(err, context.Canceled) {
				w.log.Warn("tick error", "err", err)
			}
		}
	}
}

func (w *Worker) tick(ctx context.Context) error {
	hits, err := w.store.ClaimUnclassifiedHits(ctx, storage.ClaimOptions{
		Limit:           w.cfg.BatchSize,
		WorkerID:        w.cfg.WorkerID,
		Lease:           w.cfg.Lease,
		OnlyUndelivered: w.cfg.OnlyUndelivered,
	})
	if err != nil {
		return err
	}
	if len(hits) == 0 {
		return nil
	}

	for _, h := range hits {
		if err := w.classifyOne(ctx, h); err != nil {
			if errors.Is(err, storage.ErrClaimLost) {
				w.log.Warn("claim lost while classifying hit",
					"id", h.ID,
					"channel", h.Channel,
					"message_id", h.MessageID,
				)
				continue
			}

			_ = w.store.ReleaseProcessing(ctx, h.ID, w.cfg.WorkerID)
			w.log.Warn("failed to classify hit", "id", h.ID, "err", err)
			continue
		}
	}

	return nil
}

func (w *Worker) classifyOne(ctx context.Context, h storage.Hit) error {
	text := normalizeText(h.Text)
	text = truncateRunes(text, w.cfg.MaxTextRunes)

	companiesFound := w.findCompanies(text, 5)
	prompt := buildPromptStrictReason(h.Keyword, text, companiesFound)

	var lastErr error
	for attempt := 0; attempt <= w.cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			sleep := w.cfg.RetryBackoff * time.Duration(attempt)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(sleep):
			}
		}

		raw, err := w.ollama.Classify(ctx, w.cfg.Model, prompt)
		if err != nil {
			lastErr = fmt.Errorf("ollama classify: %w", err)
			continue
		}

		res, err := parseLLMJSON(raw)
		if err != nil {
			lastErr = fmt.Errorf("parse llm response: %w (raw=%q)", err, safeSnippet(raw, 240))
			continue
		}

		cat := normalizeCategory(res.Category)
		if cat == "" {
			lastErr = fmt.Errorf("invalid category in response: %q", res.Category)
			continue
		}

		reason := strings.TrimSpace(res.Reason)
		if reason == "" {
			lastErr = errors.New("llm returned empty reason")
			continue
		}
		reason = truncateRunes(reason, 140)

		cls := storage.Classification{
			Category:     cat,
			LLMModel:     w.cfg.Model,
			ClassifiedAt: time.Now().UTC(),
			Reason:       &reason,
		}

		if res.Confidence != nil {
			v := *res.Confidence
			if v < 0 {
				v = 0
			}
			if v > 1 {
				v = 1
			}
			cls.Confidence = &v
		} else {
			v := 0.5
			cls.Confidence = &v
		}

		if err := w.store.UpdateClassification(ctx, h.ID, w.cfg.WorkerID, cls); err != nil {
			lastErr = fmt.Errorf("update classification: %w", err)
			if errors.Is(err, storage.ErrClaimLost) {
				return err
			}
			continue
		}

		w.log.Info("classified",
			"id", h.ID,
			"channel", h.Channel,
			"message_id", h.MessageID,
			"keyword", h.Keyword,
			"category", cat,
			"confidence", cls.Confidence,
		)

		return nil
	}

	fallback := "LLM не вернул корректный JSON/умозаключение; требуется перепроверка"
	if lastErr != nil {
		fallback = truncateRunes(fallback+": "+lastErr.Error(), 140)
	}

	v := 0.0
	cls := storage.Classification{
		Category:     "other",
		LLMModel:     w.cfg.Model,
		ClassifiedAt: time.Now().UTC(),
		Confidence:   &v,
		Reason:       &fallback,
	}

	if err := w.store.UpdateClassification(ctx, h.ID, w.cfg.WorkerID, cls); err != nil {
		if errors.Is(err, storage.ErrClaimLost) {
			return err
		}
		w.log.Warn("failed to persist fallback classification",
			"id", h.ID,
			"err", err,
		)
	}

	if lastErr == nil {
		lastErr = errors.New("unknown classification error")
	}
	return lastErr
}

type llmResult struct {
	Category   string   `json:"category"`
	Confidence *float64 `json:"confidence,omitempty"`
	Reason     string   `json:"reason"`
}

func buildPromptStrictReason(keyword string, text string, companiesFound []string) string {
	has := "no"
	found := ""
	if len(companiesFound) > 0 {
		has = "yes"
		found = strings.Join(companiesFound, ", ")
	}

	return strings.TrimSpace(fmt.Sprintf(`
Ты классификатор новостей для мониторинга AI/автоматизации и e-commerce в России.
Верни ТОЛЬКО JSON одним объектом. Никаких пояснений, markdown и кода вне JSON.

Категории:
- hr: кадровые изменения (назначение/увольнение/отставка/смена должности/CEO/директоров) ТОЛЬКО если речь о компании из top-250.
- ai_auto: AI/автоматизация в России (контакт-центры, продажи, бизнес).
- ecommerce: e-commerce и онлайн-торговля в России. Выбирай эту категорию, если новость:
  (a) связана с российской компанией/рынком и темой e-commerce,
  ИЛИ
  (b) описывает глобальный/локальный тренд, технологию или изменение в сфере онлайн-торговли, релевантное для РФ.
- other: всё остальное.

Правила:
1) Если сомневаешься — выбирай other.
2) reason обязателен: 20–140 символов, краткое объяснение выбора категории.

Дано:
keyword: %q
has_top250_company: %s
top250_companies_found: %q
text: %q

Формат JSON:
{"category":"hr|ai_auto|ecommerce|other","reason":"текст","confidence":0.0}
`, keyword, has, found, text))
}

func parseLLMJSON(raw string) (llmResult, error) {
	raw = strings.TrimSpace(raw)

	obj := extractFirstJSONObject(raw)
	if obj == "" {
		return llmResult{}, errors.New("no JSON object found in response")
	}

	var r llmResult
	if err := json.Unmarshal([]byte(obj), &r); err != nil {
		return llmResult{}, err
	}

	r.Category = strings.TrimSpace(r.Category)
	r.Reason = strings.TrimSpace(r.Reason)

	if r.Reason == "" {
		return llmResult{}, errors.New("reason is empty")
	}

	return r, nil
}

var jsonObjectRE = regexp.MustCompile(`(?s)\{.*\}`)

func extractFirstJSONObject(s string) string {
	m := jsonObjectRE.FindString(s)
	return strings.TrimSpace(m)
}

func normalizeCategory(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	switch s {
	case "hr":
		return "hr"
	case "ai_auto", "ai-auto", "ai", "automation":
		return "ai_auto"
	case "ecommerce", "e-commerce", "e_commerce", "ecom":
		return "ecommerce"
	case "other", "misc":
		return "other"
	default:
		return ""
	}
}

func normalizeText(s string) string {
	s = strings.ReplaceAll(s, "\u00a0", " ")
	s = strings.TrimSpace(s)
	s = strings.Join(strings.Fields(s), " ")
	return s
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

func safeSnippet(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

func (w *Worker) findCompanies(text string, max int) []string {
	if len(w.whitelist) == 0 {
		return nil
	}
	if max <= 0 {
		max = 5
	}

	normText := normalizeCompany(text)

	found := make([]string, 0, 4)
	for _, c := range w.whitelist {
		if c == "" {
			continue
		}
		if strings.Contains(normText, c) {
			found = append(found, c)
			if len(found) >= max {
				break
			}
		}
	}
	return found
}

func parseWhitelistFromRaw(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	lines := strings.Split(raw, "\n")
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		norm := normalizeCompany(ln)
		if norm == "" {
			continue
		}
		out = append(out, norm)
	}
	return uniqueStrings(out)
}

const dirtyTop250Raw = `
Воксис
Ростелеком контакт-центр (МЦНТТ)
Teleperformance Russia
TeleTribe
Аудиотеле
Neovox
CallTraffic
SberCare
Телеконтакт
Next Contact
Контакт-центр ГРАН
Телесейлз Сервис
Сити Колл 
Smarter
Контакт-Сервис
Банк ВТБ
Альфа Банк
Т-Банк
Газпромбанк
ПромСвязьБанк
Россельхозбанк
Совкомбанк
Московский Кредитный Банк
Банк ДОМ.РФ
Банк «Санкт-Петербург»
МТС Банк
Банк Россия
Уралсиб
Яндекс Банк
Кубань Кредит
Банк ТКБ
УБРиР
Ак Барс Банк
Абсолют Банк
Русский Стандарт Страхование
НОВИКОМ
ОТП Банк
Банк Зенит
Азиатско-Тихоокеанский Банк
Металлинвестбанк
Экспобанк
Локо-Банк
ПримСоцБанк
Центр-Инвест
Банк Левобережный
СДМ Банк
Кредит Европа Банк
Синара Банк
ББР Банк
Национальный резервный банк
Фора-Банк
Ренессанс Банк
БыстроБанк
Банк «Солидарность»
Райффайзенбанк
БКС Банк
Озон Банк
Авангард
Wildberries Банк
ВБРР
Модульбанк
МСП Банк
Рокет Банк
Росбанк
Точка Банк
Почта России
Деловые Линии
СДЭК
ИТЕКО
ПЭК
DPD
Boxberry
5POST
Pony Express
Грузовичкоф
Озон Логистика
wildberries Логистика
Фонбет
WinLine
BetBoom
Pari
Лига Ставок
stoloto.ru
МФК Т-Финанс
МФК ОТП Финанс
МКК "А ДЕНЬГИ"
МКК "АЛЬФА ФИНАНС"
МФК Мани Мен
МФК «Займер»
МФК КарМани
МФК Фордевинд
МФК webbankir
МФК Lime Credit Group (лайм займ)
МФК Быстроденьги
МФК ВебЗайм
МФК Екапуста
Мэйджор
Рольф
Автомир
Ключавто
Агат
Автодом
ТрансТехСервис
КорсГруп
Авилон
ГК Катрен
Апрель Аптеки
Ригла 
Планета здоровья 
Имплозия
Аптечная сеть 36,6
Нео-фарм
Эркафарм/Мелодия здоровья
Фармлэнд
Вита
ИРИС
Вкусно и точка
DoDo Brands
Бургер кинг
Rostic's
X5 Retail Group
Магнит
Пятерочка
Mercury Retail Group
Лента
Красное и Белое (ООО "АЛЬФА-М")
Перекресток
ВкусВилл
Дикси
МЕТРО
Чижик
Бристоль
Ашан
Х5 Digital
Верный сеть универсамов
СПАР
Мария-РА
Магнит ОМНИ
ВинЛаб
DNS
Лемана Про
М.Видео-Эльдорадо
Комус
Fix Price
Детский Мир
Петрович ТД
Спортмастер
Золотое яблоко
Ситилинк
Санлайт
Технопарк
ОфисМаг
restore (Ланит)
HOFF
Askona
Соколов
СОГАЗ
Альфа Страхование
Ингосстрах
РЕСО-Гарантия
Сбер Страхование
САО «ВСК»
Росгосстрах
СК Согласие
Ренессанс Страхование
АО «МАКС» — Московская акционерная страховая компания
Т-Страхование
Зетта Страхование
Капитал Life
Абсолют Страхование
ГСК Югория
Энергогарант
Ак Барс Страхование
РСХБ-Страхование
Медэкспресс
Совкомбанк страхование
ПСБ Страхование
ГЕЛИОС
Русский Стандарт Страхование
Ozon Страхование
Ростелеком
МТС
Мегафон
Билайн
Tele2
Дом.ru Эр-Телеком
yota
Уфанет
Таттелеком
Мотив
Транстелеком
ПАО РЖД
Аэрофлот
АО "ФПК"
S7 Airlines
Авиакомпания Россия
Уральские авиалинии
Авиакомпания Победа
АВИАКОМПАНИЯ "ЮТЭЙР
Авиакомпания Nordwind Airlines
Делимобиль
Ситидрайв
Авиакомпания "АЗУР ЭЙР"
"ТТ-Трэвел" (Fun&Sun)
tutu.ru
Otello
Островок!
Суточно.ру
Авиасейлз
Купибилет
ПАО "Интер Рао"
АО "Мосэнергосбыт"
ПАО "Т Плюс"
ПАО "ТНС ЭНЕРГО"
ООО "Русэнергосбыт"
АО "Петербургская Сбытовая Компания"
АО "Татэнерго"
АО "РОСАТОМ ЭНЕРГОСБЫТ"
Мосводоканал
ООО "БАЙКАЛЬСКАЯ ЭНЕРГЕТИЧЕСКАЯ КОМПАНИЯ"
ООО "УРАЛЭНЕРГОСБЫТ"
ООО "Энергетическая сбытовая компания Башкортостана"
АО Новосибирскэнергосбыт
Росводоканал
АО НЭСК «Независимая энергосбытовая компания Краснодарского края»
ПАО "Красноярскэнергосбыт"
АО «Энергосбытовая компания «Восток»
КМА-Энергосбыт
ПАО "ТГК-2"
Wildberries
Ozon
Самокат
Купер
Авито
Все инструменты.ру
Русский свет rs24.ru
Аптека.ру
Lamoda
zdravcity.ru
auto.ru
Карпрайз
onlinetrade.ru
Домклик
Сантехника онлайн
Автодок
ЦИАН
AliExpress Россия
divan.ru
shoppinglive.ru
Flowwow
Ozon fresh
`
