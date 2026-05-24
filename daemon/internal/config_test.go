package internal

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigExpandsHomePaths(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfgPath, []byte(`
state_path = "~/.local/state/trailboss/state.json"
sessions_path = "~/.local/state/trailboss/sessions.jsonl"
pid_path = "~/.local/state/trailboss/trailboss.pid"

[[source]]
name = "nvim"
path = "~/.local/share/trailboss/comments.jsonl"
id_field = "id"

[provider.codex]
command = "codex"
session_dir = "~/.codex/sessions/{{.Year}}/{{.Month}}/{{.Day}}"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatal(err)
	}

	wantSource := filepath.Join(home, ".local/share/trailboss/comments.jsonl")
	if cfg.Sources[0].Path != wantSource {
		t.Fatalf("source path = %q, want %q", cfg.Sources[0].Path, wantSource)
	}

	wantSessionDir := filepath.Join(home, ".codex/sessions/{{.Year}}/{{.Month}}/{{.Day}}")
	if cfg.Providers["codex"].SessionDir != wantSessionDir {
		t.Fatalf("session dir = %q, want %q", cfg.Providers["codex"].SessionDir, wantSessionDir)
	}
}
