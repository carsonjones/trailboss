package internal

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/template"
	"time"
)

const scriptTmpl = `#!/usr/bin/env bash
set -euo pipefail
echo "=== trailboss ==="
echo ""
cat '{{.PromptFile}}'
echo ""
{{- if .SessionID}}
{{.LaunchCmd}}
exec {{.ResumeCmd}}
{{- else if eq .Strategy "poll-dir"}}
{{.LaunchCmd}} || true
SESSION_ID=$(python3 - <<'PY'
import json, os, sys
from pathlib import Path
session_dir = {{printf "%q" .SessionDir}}
since = float({{.SinceUnix}})
cwd = {{printf "%q" .CWD}}

def extract(path):
    try:
        line = Path(path).read_text().splitlines()[0]
        data = json.loads(line)
    except Exception:
        return ""
    if data.get("type") == "session":
        return data.get("id", "")
    if data.get("type") == "session_meta":
        payload = data.get("payload") or {}
        if payload.get("source") != "exec" or payload.get("originator") != "codex_exec":
            return ""
        if cwd and payload.get("cwd") != cwd:
            return ""
        return payload.get("id", "")
    return ""

try:
    entries = sorted(Path(session_dir).iterdir(), key=lambda p: p.stat().st_mtime)
except Exception:
    entries = []
for entry in entries:
    try:
        if entry.is_file() and entry.stat().st_mtime >= since:
            sid = extract(entry)
            if sid:
                print(sid)
                break
    except Exception:
        pass
PY
)
if [ -n "$SESSION_ID" ]; then
    exec {{.Command}} {{.ResumeFlagArgs}}{{if .AppendSessionID}} "$SESSION_ID"{{end}}
else
    exec {{.Command}}
fi
{{- else}}
OUTPUT=$({{.LaunchCmd}} 2>&1) || true
SESSION_ID=$(echo "$OUTPUT" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('session_id',''))" 2>/dev/null || true)
if [ -n "$SESSION_ID" ]; then
    exec {{.Command}} {{.ResumeFlagArgs}}{{if .AppendSessionID}} "$SESSION_ID"{{end}}
else
    exec {{.Command}}
fi
{{- end}}
`

var scriptTemplate = template.Must(template.New("script").Parse(scriptTmpl))

type scriptData struct {
	PromptFile      string
	Command         string
	LaunchCmd       string
	ResumeCmd       string
	ResumeFlagArgs  string
	AppendSessionID bool
	SessionID       string
	Strategy        string
	SessionDir      string
	SinceUnix       int64
	CWD             string
}

func ZellijLaunch(tabName, prompt, cwd string, provider ProviderConfig) error {
	id := newID()
	promptFile := fmt.Sprintf("/tmp/trailboss-prompt-%s.txt", id)
	scriptFile := fmt.Sprintf("/tmp/trailboss-launch-%s.sh", id)

	if err := os.WriteFile(promptFile, []byte(prompt), 0o644); err != nil {
		return fmt.Errorf("write prompt file: %w", err)
	}

	since := time.Now()
	var sessionID string
	if provider.SessionFrom == "pre-assign" {
		sessionID = newUUID()
	}

	sessionDir := provider.SessionDir
	if cwd != "" {
		sessionDir = strings.ReplaceAll(sessionDir, "{{.CWDEncoded}}", cwdEncoded(cwd))
	}
	sessionDir = expandDir(sessionDir)

	// Build launch command with the prompt shell-quoted into the generated script.
	launchArgs := RenderArgs(provider.LaunchArgs, prompt, sessionID, cwd)
	launchCmd := provider.Command + " " + shellJoin(launchArgs)

	// build resume command
	resumeCmd := ""
	resumeFlagArgs := ""
	appendSessionID := false
	if len(provider.ResumeArgs) > 0 {
		if sessionID != "" {
			// pre-assign: full resume command is known
			renderedResume := RenderArgs(provider.ResumeArgs, "", sessionID, cwd)
			resumeCmd = provider.Command + " " + shellJoin(renderedResume)
		} else {
			// Runtime-discovered session IDs are appended only if resume_args contains {{.SessionID}}.
			noIDArgs := make([]string, 0, len(provider.ResumeArgs))
			for _, a := range provider.ResumeArgs {
				if a == "{{.SessionID}}" {
					appendSessionID = true
					continue
				}
				noIDArgs = append(noIDArgs, a)
			}
			resumeFlagArgs = shellJoin(noIDArgs)
		}
	}

	var scriptBuf bytes.Buffer
	if err := scriptTemplate.Execute(&scriptBuf, scriptData{
		PromptFile:      promptFile,
		Command:         provider.Command,
		LaunchCmd:       launchCmd,
		ResumeCmd:       resumeCmd,
		ResumeFlagArgs:  resumeFlagArgs,
		AppendSessionID: appendSessionID,
		SessionID:       sessionID,
		Strategy:        provider.SessionFrom,
		SessionDir:      sessionDir,
		SinceUnix:       since.Unix(),
		CWD:             cwd,
	}); err != nil {
		return fmt.Errorf("render script: %w", err)
	}

	if err := os.WriteFile(scriptFile, scriptBuf.Bytes(), 0o755); err != nil {
		return fmt.Errorf("write script file: %w", err)
	}

	args := []string{"action", "new-tab", "--name", tabName}
	if cwd != "" {
		args = append(args, "--cwd", cwd)
	}
	args = append(args, "--", "bash", scriptFile)

	cmd := exec.Command("zellij", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("zellij new-tab: %w\n%s", err, out)
	}
	return nil
}

func shellJoin(args []string) string {
	quoted := make([]string, len(args))
	for i, a := range args {
		if strings.ContainsAny(a, " \t\n'\"\\$`") {
			quoted[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
		} else {
			quoted[i] = a
		}
	}
	return strings.Join(quoted, " ")
}
