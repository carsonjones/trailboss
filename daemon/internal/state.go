package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type State struct {
	Seen map[string]map[string]bool `json:"seen"` // source name -> id -> true
	path string
}

func LoadState(path string) (*State, error) {
	s := &State{
		Seen: make(map[string]map[string]bool),
		path: path,
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}
	s.path = path
	return s, nil
}

func (s *State) HasSeen(source, id string) bool {
	return s.Seen[source][id]
}

func (s *State) Mark(source, id string) {
	if s.Seen[source] == nil {
		s.Seen[source] = make(map[string]bool)
	}
	s.Seen[source][id] = true
}

func (s *State) Save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("mkdir state dir: %w", err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	return os.WriteFile(s.path, data, 0o644)
}
