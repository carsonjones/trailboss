package internal

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

type Session struct {
	ID         string    `json:"id"`
	SessionID  string    `json:"session_id,omitempty"`
	Name       string    `json:"name"`
	StartedAt  time.Time `json:"started_at"`
	OutputFile string    `json:"output_file,omitempty"`
	SessionDir string    `json:"session_dir,omitempty"`
	CWD        string    `json:"cwd,omitempty"`
	PID        int       `json:"pid"`
	Provider   string    `json:"provider"`
	Strategy   string    `json:"strategy"`
}

type SessionStatus struct {
	Session
	SessionID string
	Done      bool
	Failed    bool
}

type SessionStore struct {
	path string
}

func NewSessionStore(path string) *SessionStore {
	return &SessionStore{path: path}
}

func (ss *SessionStore) Add(s Session) error {
	if err := os.MkdirAll(filepath.Dir(ss.path), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	f, err := os.OpenFile(ss.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open sessions: %w", err)
	}
	defer f.Close()
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(f, "%s\n", data)
	return err
}

func (ss *SessionStore) List() ([]SessionStatus, error) {
	f, err := os.Open(ss.path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var results []SessionStatus
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var s Session
		if err := json.Unmarshal(line, &s); err != nil {
			continue
		}
		results = append(results, resolveStatus(s))
	}
	return results, scanner.Err()
}

func (ss *SessionStore) Remove(id string) error {
	sessions, err := ss.List()
	if err != nil {
		return err
	}
	f, err := os.OpenFile(ss.path, os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open sessions: %w", err)
	}
	defer f.Close()
	for _, s := range sessions {
		if s.ID == id {
			continue
		}
		data, _ := json.Marshal(s.Session)
		fmt.Fprintf(f, "%s\n", data)
	}
	return nil
}

func (ss *SessionStore) Clear() error {
	return os.Remove(ss.path)
}

func resolveStatus(s Session) SessionStatus {
	st := SessionStatus{Session: s}

	switch s.Strategy {
	case "pre-assign":
		st.SessionID = s.SessionID
		if !pidAlive(s.PID) {
			st.Done = true
		}
		return st

	case "poll-dir":
		if sid := scanSessionDir(s.SessionDir, s.StartedAt, s.CWD); sid != "" {
			st.SessionID = sid
			st.Done = true
			return st
		}
		if !pidAlive(s.PID) {
			st.Failed = true
		}
		return st

	case "howdy":
		if s.SessionID != "" {
			st.SessionID = s.SessionID
			if !pidAlive(s.PID) {
				st.Done = true
			}
			return st
		}
		fallthrough // ping disabled — parse session ID from output file

	default: // "output-json"
		data, err := os.ReadFile(s.OutputFile)
		if err != nil {
			if pidAlive(s.PID) {
				return st
			}
			st.Failed = true
			return st
		}
		var out map[string]any
		if json.Unmarshal(data, &out) == nil {
			if sid, ok := out["session_id"].(string); ok && sid != "" {
				st.SessionID = sid
				st.Done = true
				return st
			}
		}
		if pidAlive(s.PID) {
			return st
		}
		st.Failed = true
		return st
	}
}

func scanSessionDir(dir string, since time.Time, cwd string) string {
	if dir == "" {
		return ""
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil || info.ModTime().Before(since) {
			continue
		}
		if sid := extractSessionID(filepath.Join(dir, e.Name()), cwd); sid != "" {
			return sid
		}
	}
	return ""
}

func extractSessionID(path, cwd string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		return ""
	}

	var line map[string]any
	if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
		return ""
	}

	// pi: {"type":"session","id":"UUID",...}
	if line["type"] == "session" {
		if id, ok := line["id"].(string); ok {
			return id
		}
	}

	// codex: {"type":"session_meta","payload":{"id":"UUID",...}}
	if line["type"] == "session_meta" {
		if payload, ok := line["payload"].(map[string]any); ok {
			if source, _ := payload["source"].(string); source != "exec" {
				return ""
			}
			if originator, _ := payload["originator"].(string); originator != "codex_exec" {
				return ""
			}
			if cwd != "" {
				if sessionCWD, _ := payload["cwd"].(string); sessionCWD != cwd {
					return ""
				}
			}
			if id, ok := payload["id"].(string); ok {
				return id
			}
		}
	}

	return ""
}

func pidAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}
