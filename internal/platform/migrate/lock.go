package migrate

import (
	"errors"
	"fmt"
	"hash/fnv"
	"strings"
)

// StableAdvisoryLockID делает стабильный lock id из нормальных названий
// чтобы не пихать в код рандомное магическое число
//
// для одной и той же пары (namespace, resource) всегда будет один и тот же int64
// если лочим разные штуки то и названия должны быть разными
//
// пример:
//
//	StableAdvisoryLockID("telegram-bot-scraper", "schema-migrations")
func StableAdvisoryLockID(namespace string, resource string) (int64, error) {
	namespace = normalizeLockPart(namespace)
	resource = normalizeLockPart(resource)

	if namespace == "" {
		return 0, errors.New("stable advisory lock id: namespace is required")
	}
	if resource == "" {
		return 0, errors.New("stable advisory lock id: resource is required")
	}

	// собираем строку ключа в одном понятном формате, с разделителем
	// префикс v1 оставлен на будущее, если когда-нибудь поменяю схему,
	// можно будет сделать v2 и не ломать старую логику
	key := "v1:" + namespace + ":" + resource

	h := fnv.New64a()
	if _, err := h.Write([]byte(key)); err != nil {
		return 0, fmt.Errorf("stable advisory lock id: hash write: %w", err)
	}

	return int64(h.Sum64()), nil
}

func normalizeLockPart(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.Join(strings.Fields(s), "-")
	return s
}
