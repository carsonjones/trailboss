package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	StatePath       string                    `toml:"state_path"`
	SessionsPath    string                    `toml:"sessions_path"`
	PIDPath         string                    `toml:"pid_path"`
	DefaultProvider string                    `toml:"default_provider"`
	Howdy           bool                      `toml:"howdy"` // set true to enable howdy ping for faster session ID resolution
	Sources         []SourceConfig            `toml:"source"`
	Providers       map[string]ProviderConfig `toml:"provider"`
	Runtimes        map[string]RuntimeConfig  `toml:"runtime"`
}

type SourceConfig struct {
	Name            string `toml:"name"`
	Path            string `toml:"path"`
	IDField         string `toml:"id_field"`
	Provider        string `toml:"provider"`
	Runtime         string `toml:"runtime"`
	Dangerous       bool   `toml:"dangerous"` // append provider dangerous_args on dispatch
	PromptTemplate  string `toml:"prompt_template"`
	TabNameTemplate string `toml:"tab_name_template"`
}

// ProviderConfig describes how to launch and resume an AI agent CLI.
//
// launch_args, continue_args, dangerous_args, and resume_args support:
//
//	{{.Prompt}}     the prompt text
//	{{.SessionID}}  session ID (pre-assign or howdy strategies)
//	{{.CWD}}        working directory
type ProviderConfig struct {
	Command        string   `toml:"command"`
	LaunchArgs     []string `toml:"launch_args"`
	ContinueArgs   []string `toml:"continue_args"`  // howdy: args for the real background job after ping
	DangerousArgs  []string `toml:"dangerous_args"` // appended to launch/continue args by `act`
	ResumeArgs     []string `toml:"resume_args"`
	SessionFrom    string   `toml:"session_from"`     // "pre-assign" | "poll-dir" | "howdy"
	SessionDir     string   `toml:"session_dir"`      // poll-dir: dir to scan for new session files
	HowdyPrompt    string   `toml:"howdy_prompt"`     // howdy: ping prompt, default "howdy partner"
	SessionIDField string   `toml:"session_id_field"` // howdy: JSON field for session ID, default "session_id"
}

type RuntimeConfig struct{}

func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	if cfg.StatePath == "" {
		home, _ := os.UserHomeDir()
		cfg.StatePath = home + "/.local/state/trailboss/state.json"
	}
	cfg.StatePath = expandHome(cfg.StatePath)
	if cfg.SessionsPath == "" {
		cfg.SessionsPath = filepath.Join(filepath.Dir(cfg.StatePath), "sessions.jsonl")
	}
	cfg.SessionsPath = expandHome(cfg.SessionsPath)
	if cfg.PIDPath == "" {
		cfg.PIDPath = filepath.Join(filepath.Dir(cfg.StatePath), "trailboss.pid")
	}
	cfg.PIDPath = expandHome(cfg.PIDPath)
	for i := range cfg.Sources {
		cfg.Sources[i].Path = expandHome(cfg.Sources[i].Path)
	}
	for name, provider := range cfg.Providers {
		provider.SessionDir = expandHome(provider.SessionDir)
		cfg.Providers[name] = provider
	}
	return cfg, nil
}

func expandHome(path string) string {
	if path == "" {
		return ""
	}
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == "~" {
		return home
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~/"))
}
