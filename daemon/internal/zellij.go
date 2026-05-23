package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/template"
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
{{- else}}
OUTPUT=$({{.LaunchCmd}} 2>&1) || true
SESSION_ID=$(echo "$OUTPUT" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('session_id',''))" 2>/dev/null || true)
if [ -n "$SESSION_ID" ]; then
    exec {{.Command}} {{.ResumeFlagArgs}} "$SESSION_ID"
else
    exec {{.Command}}
fi
{{- end}}
`

var scriptTemplate = template.Must(template.New("script").Parse(scriptTmpl))

type pipeMsg struct {
	Action string `json:"action"`
	Name   string `json:"name"`
	Script string `json:"script"`
}

type scriptData struct {
	PromptFile    string
	Command       string
	LaunchCmd     string
	ResumeCmd     string
	ResumeFlagArgs string
	SessionID     string
}

func ZellijLaunch(tabName, prompt string, provider ProviderConfig, runtime RuntimeConfig) error {
	promptFile := fmt.Sprintf("/tmp/trailboss-prompt-%d.txt", os.Getpid())
	scriptFile := fmt.Sprintf("/tmp/trailboss-launch-%d.sh", os.Getpid())

	if err := os.WriteFile(promptFile, []byte(prompt), 0o644); err != nil {
		return fmt.Errorf("write prompt file: %w", err)
	}

	var sessionID string
	if provider.SessionFrom == "pre-assign" {
		sessionID = newUUID()
	}

	// build launch command: render args, replacing {{.Prompt}} with $(cat 'file')
	launchArgs := RenderArgs(provider.LaunchArgs, fmt.Sprintf("$(cat '%s')", promptFile), sessionID, "")
	launchCmd := provider.Command + " " + shellJoin(launchArgs)

	// build resume command
	resumeCmd := ""
	resumeFlagArgs := ""
	if len(provider.ResumeArgs) > 0 {
		if sessionID != "" {
			// pre-assign: full resume command is known
			renderedResume := RenderArgs(provider.ResumeArgs, "", sessionID, "")
			resumeCmd = provider.Command + " " + shellJoin(renderedResume)
		} else {
			// output-json: resume args without session_id (appended by script at runtime)
			noIDArgs := make([]string, 0, len(provider.ResumeArgs))
			for _, a := range provider.ResumeArgs {
				if a != "{{.SessionID}}" {
					noIDArgs = append(noIDArgs, a)
				}
			}
			resumeFlagArgs = shellJoin(noIDArgs)
		}
	}

	var scriptBuf bytes.Buffer
	if err := scriptTemplate.Execute(&scriptBuf, scriptData{
		PromptFile:    promptFile,
		Command:       provider.Command,
		LaunchCmd:     launchCmd,
		ResumeCmd:     resumeCmd,
		ResumeFlagArgs: resumeFlagArgs,
		SessionID:     sessionID,
	}); err != nil {
		return fmt.Errorf("render script: %w", err)
	}

	if err := os.WriteFile(scriptFile, scriptBuf.Bytes(), 0o755); err != nil {
		return fmt.Errorf("write script file: %w", err)
	}

	msg, err := json.Marshal(pipeMsg{Action: "new_tab", Name: tabName, Script: scriptFile})
	if err != nil {
		return fmt.Errorf("marshal pipe msg: %w", err)
	}

	cmd := exec.Command("zellij", "pipe", "--plugin", runtime.PluginPath, "--", string(msg))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("zellij pipe: %w\n%s", err, out)
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
