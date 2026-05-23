package internal

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"text/template"
)

func ProcessSource(src SourceConfig, state *State, launch func(tabName, prompt, cwd string, provider ProviderConfig) error, providers map[string]ProviderConfig) error {
	f, err := os.Open(src.Path)
	if err != nil {
		return fmt.Errorf("open %s: %w", src.Path, err)
	}
	defer f.Close()

	provider, ok := providers[src.Provider]
	if !ok {
		return fmt.Errorf("unknown provider %q", src.Provider)
	}

	promptTmpl, err := template.New("prompt").Parse(src.PromptTemplate)
	if err != nil {
		return fmt.Errorf("parse prompt_template: %w", err)
	}
	tabTmpl, err := template.New("tab").Parse(src.TabNameTemplate)
	if err != nil {
		return fmt.Errorf("parse tab_name_template: %w", err)
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		var item map[string]any
		if err := json.Unmarshal(line, &item); err != nil {
			slog.Warn("parse line", "source", src.Name, "err", err)
			continue
		}

		id, _ := item[src.IDField].(string)
		if id == "" {
			continue
		}
		if state.HasSeen(src.Name, id) {
			continue
		}

		prompt, err := renderTmpl(promptTmpl, item)
		if err != nil {
			slog.Error("render prompt", "source", src.Name, "id", id, "err", err)
			continue
		}
		tabName, err := renderTmpl(tabTmpl, item)
		if err != nil {
			slog.Error("render tab name", "source", src.Name, "id", id, "err", err)
			continue
		}

		cwd, _ := item["cwd"].(string)
		if cwd == "" {
			p, ok := item["path"].(string)
			if ok && p != "" {
			cwd = filepath.Dir(p)
			}
		}

		if err := launch(tabName, prompt, cwd, provider); err != nil {
			slog.Error("launch", "source", src.Name, "id", id, "err", err)
			continue
		}

		state.Mark(src.Name, id)
		if err := state.Save(); err != nil {
			slog.Warn("save state", "err", err)
		}
	}
	return scanner.Err()
}

func renderTmpl(t *template.Template, data map[string]any) (string, error) {
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
