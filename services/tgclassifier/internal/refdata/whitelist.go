package refdata

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"unicode"
)

const embeddedTop250File = "top250_companies.txt"

func LoadCompanies(path string) ([]string, error) {
	if strings.TrimSpace(path) != "" {
		return loadCompaniesFromOSFile(path)
	}
	return loadCompaniesFromEmbeddedFile()
}

func loadCompaniesFromEmbeddedFile() ([]string, error) {
	f, err := FS.Open(embeddedTop250File)
	if err != nil {
		return nil, fmt.Errorf("refdata: open embedded whitelist: %w", err)
	}
	defer f.Close()

	return scanCompanies(bufio.NewScanner(f))
}

func loadCompaniesFromOSFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("refdata: open whitelist file: %w", err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	return scanCompanies(sc)
}

func scanCompanies(sc *bufio.Scanner) ([]string, error) {
	out := make([]string, 0, 256)

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}

		norm := NormalizeCompany(line)
		if norm == "" {
			continue
		}
		out = append(out, norm)
	}

	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("refdata: scan whitelist: %w", err)
	}

	return uniqueStrings(out), nil
}

func NormalizeCompany(s string) string {
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

func FindCompanies(text string, whitelist []string, max int) []string {
	if len(whitelist) == 0 {
		return nil
	}
	if max <= 0 {
		max = 5
	}

	normText := NormalizeCompany(text)

	found := make([]string, 0, 4)
	for _, c := range whitelist {
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

func uniqueStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))

	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}

	return out
}
