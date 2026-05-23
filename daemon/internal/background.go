package internal

import (
	cryptorand "crypto/rand"
	"encoding/json"
	"fmt"
	mathrand "math/rand/v2"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

const base36 = "0123456789abcdefghijklmnopqrstuvwxyz"

func newID() string {
	b := make([]byte, 6)
	for i := range b {
		b[i] = base36[mathrand.N(len(base36))]
	}
	return string(b)
}

func newUUID() string {
	var b [16]byte
	cryptorand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func RenderArgs(args []string, prompt, sessionID, cwd string) []string {
	result := make([]string, len(args))
	r := strings.NewReplacer(
		"{{.Prompt}}", prompt,
		"{{.SessionID}}", sessionID,
		"{{.CWD}}", cwd,
	)
	for i, a := range args {
		result[i] = r.Replace(a)
	}
	return result
}

func expandDir(dir string) string {
	if dir == "" {
		return ""
	}
	home, _ := os.UserHomeDir()
	dir = strings.ReplaceAll(dir, "~", home)
	now := time.Now()
	dir = strings.ReplaceAll(dir, "{{.Year}}", now.Format("2006"))
	dir = strings.ReplaceAll(dir, "{{.Month}}", now.Format("01"))
	dir = strings.ReplaceAll(dir, "{{.Day}}", now.Format("02"))
	return dir
}

func cleanEnv() []string {
	env := os.Environ()
	out := make([]string, 0, len(env))
	for _, e := range env {
		if !strings.HasPrefix(e, "NVIM") {
			out = append(out, e)
		}
	}
	return out
}

func cwdEncoded(cwd string) string {
	return "--" + strings.ReplaceAll(strings.TrimPrefix(cwd, "/"), "/", "-") + "--"
}

func BackgroundLaunch(tabName, prompt, cwd, providerName string, provider ProviderConfig, store *SessionStore, howdy bool) error {
	id := newID()
	outputFile := fmt.Sprintf("/tmp/trailboss-output-%s.json", id)

	var (
		sessionID  string
		sessionDir string
		launchArgs []string
	)

	switch provider.SessionFrom {
	case "pre-assign":
		sessionID = newUUID()
		launchArgs = RenderArgs(provider.LaunchArgs, prompt, sessionID, cwd)

	case "poll-dir":
		dir := provider.SessionDir
		if cwd != "" {
			dir = strings.ReplaceAll(dir, "{{.CWDEncoded}}", cwdEncoded(cwd))
		}
		sessionDir = expandDir(dir)
		launchArgs = RenderArgs(provider.LaunchArgs, prompt, "", cwd)

	case "howdy":
		if !howdy {
			launchArgs = RenderArgs(provider.LaunchArgs, prompt, "", cwd)
		} else {
			howdyPrompt := provider.HowdyPrompt
			if howdyPrompt == "" {
				howdyPrompt = "howdy partner"
			}
			pingArgs := RenderArgs(provider.LaunchArgs, howdyPrompt, "", cwd)
			pingCmd := exec.Command(provider.Command, pingArgs...)
			pingCmd.Env = cleanEnv()
			if cwd != "" {
				pingCmd.Dir = cwd
			}
			out, err := pingCmd.Output()
			if err != nil {
				return fmt.Errorf("howdy ping: %w", err)
			}
			field := provider.SessionIDField
			if field == "" {
				field = "session_id"
			}
			var parsed map[string]any
			if err := json.Unmarshal(out, &parsed); err != nil {
				return fmt.Errorf("howdy parse: %w", err)
			}
			sessionID, _ = parsed[field].(string)
			if sessionID == "" {
				return fmt.Errorf("howdy: no %q in response", field)
			}
			launchArgs = RenderArgs(provider.ContinueArgs, prompt, sessionID, cwd)
		}

	default:
		launchArgs = RenderArgs(provider.LaunchArgs, prompt, sessionID, cwd)
	}

	outFile, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}

	cmd := exec.Command(provider.Command, launchArgs...)
	cmd.Stdout = outFile
	cmd.Stderr = outFile
	cmd.Env = cleanEnv()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if cwd != "" {
		cmd.Dir = cwd
	}

	if err := cmd.Start(); err != nil {
		outFile.Close()
		return fmt.Errorf("start agent: %w", err)
	}
	outFile.Close()
	go cmd.Wait()

	return store.Add(Session{
		ID:         id,
		SessionID:  sessionID,
		Name:       tabName,
		StartedAt:  time.Now(),
		OutputFile: outputFile,
		SessionDir: sessionDir,
		CWD:        cwd,
		PID:        cmd.Process.Pid,
		Provider:   providerName,
		Strategy:   provider.SessionFrom,
	})
}
