package prompt

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"text/template"
)

const embeddedTemplateFile = "classify_news_v1.tmpl"

type StrictReasonInput struct {
	Keyword        string
	Text           string
	CompaniesFound []string
}

func BuildStrictReasonPrompt(path string, in StrictReasonInput) (string, error) {
	raw, err := loadTemplate(path)
	if err != nil {
		return "", err
	}

	tpl, err := template.New("classify_news_v1").Parse(raw)
	if err != nil {
		return "", fmt.Errorf("prompt: parse template: %w", err)
	}

	data := struct {
		Keyword          string
		Text             string
		HasTop250Company string
		Top250Found      string
	}{
		Keyword:          in.Keyword,
		Text:             in.Text,
		HasTop250Company: "no",
		Top250Found:      "",
	}

	if len(in.CompaniesFound) > 0 {
		data.HasTop250Company = "yes"
		data.Top250Found = strings.Join(in.CompaniesFound, ", ")
	}

	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("prompt: execute template: %w", err)
	}

	return strings.TrimSpace(buf.String()), nil
}

func loadTemplate(path string) (string, error) {
	if strings.TrimSpace(path) != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("prompt: read external template: %w", err)
		}
		return string(b), nil
	}

	b, err := FS.ReadFile(embeddedTemplateFile)
	if err != nil {
		return "", fmt.Errorf("prompt: read embedded template: %w", err)
	}
	return string(b), nil
}
