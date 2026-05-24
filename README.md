# Trailboss

Trailboss is a universal `launch agent with context` command.

Start work.
Stay out of the way.

It doesn't orchestrate agents.
It doesn't manage workflows.
It doesn't build another platform.

Trailboss starts work on own trails, letting you stay focused on yours.

```bash
trailboss ask "explain this code"
trailboss act "fix the failing test"
trailboss ask -p codex "add integration tests"
```

---

## Why Trailboss?

Most agent tools focus on managing agents. They have opinions about orchestration, state machines, and retry logic.

Trailboss has one opinion: **get the agent started.**

- One command for every agent
- Consistent interface across providers
- Background execution, zero blocking
- Editor integrations that stay out of your way
- Works with any CLI-based agent

---

## Trails and Riders

A **trail** is a unit of work. A prompt with context.

```bash
trailboss ask "what does the retry logic in backoff.go do?"
trailboss ask "investigate this stack trace"
trailboss act "refactor the auth middleware"
trailboss act "review this PR and fix what's broken"
```

A **rider** is the agent assigned to the trail. Configure as many as you need:

```toml
[provider.claude]
command        = "claude"
launch_args    = ["-p", "{{.Prompt}}", "--output-format", "json", "--session-id", "{{.SessionID}}"]
dangerous_args = ["--dangerously-skip-permissions"]
resume_args    = ["--resume", "{{.SessionID}}"]

[provider.codex]
command     = "codex"
launch_args = ["exec", "{{.Prompt}}", "-C", "{{.CWD}}"]
resume_args = ["resume", "--last"]
```

If it runs from a terminal, Trailboss can send it.

---

## Getting Started

From a checkout:

```sh
make install
```

That installs the `trailboss` CLI, the Zellij plugin, and the Neovim integration.
It also creates `~/.config/trailboss/config.toml` from the default config if one
doesn't already exist.
Make sure `~/.local/bin` is on your `PATH` so the `trailboss` command resolves.

From outside the repo:

```sh
go install github.com/carsonjones/trailboss/daemon/cmd/trailboss@latest
```

Optional shorthand (add to your shell rc):

```bash
alias tb="trailboss"
```

Review `~/.config/trailboss/config.toml` if you want to change providers, then start the daemon:

```bash
trailboss start
trailboss status
```

Dispatch a trail:

```bash
trailboss ask "explain the retry logic in backoff.go"
```

Check on it:

```bash
trailboss list
```

Resume a completed session interactively:

```bash
trailboss resume last
```

---

## Commands

```
trailboss start              start the daemon
trailboss stop               stop the daemon
trailboss status             daemon status
trailboss ask <prompt>       ask a question (explain, don't modify)
trailboss act <prompt>       take action (implement, fix, refactor)
trailboss trails|ls|list     list all trails
trailboss resume <id|last>   resume a completed session
trailboss rm <id>            remove a trail
trailboss clear              remove all trails
```

All commands accept `-c <path>` to use a per-project config if that suits your fancy.

### `ask`

Question mode. The agent explains, answers, and researches — but doesn't touch files.

```bash
trailboss ask "why does the auth middleware panic on nil context?"
trailboss ask -p codex "explain the retry logic in backoff.go"
```

### `act`

Action mode. The agent implements, fixes, or refactors.

```bash
trailboss act "fix the failing test in auth_test.go"
trailboss act -p codex "add integration tests for the user service"
trailboss act -s "refactor this carefully"   # skip dangerous_args for this run
```

Pass `-s` / `--safe` to suppress `dangerous_args` without changing your config.

### `resume`

Replaces the current process with the provider's resume command, dropping you straight into the session.

```bash
trailboss resume last      # most recent completed session
trailboss resume abc123    # specific session by ID
```

### `trails`

```
ID      STATUS   AGE   NAME                      RESUME
abc123  done     5m    fix: auth.go:42           claude --resume <id>
def456  running  2m    ask: explain goroutines
```

On narrower terminals, `trailboss ls` switches to a stacked layout:

```
abc123  done  5m
name:   fix: auth.go:42
resume: claude --resume <id>
---
def456  running  2m
name:   ask: explain goroutines
```

For scripts, use JSON instead of scraping the display output:

```bash
trailboss ls -json | jq '.[] | select(.status == "done") | .resume'
```

---

## Trail Sources

Beyond `trailboss ask`, Trailboss can watch JSONL queues and automatically dispatch work as entries arrive.

```json
{"id":"abc123","path":"auth.go","line":42,"body":"panic on nil pointer"}
```

Append a line. Trailboss picks it up, renders a prompt from the template, and launches a session. Each item is dispatched exactly once — IDs are deduplicated across restarts.

```toml
[[source]]
name     = "my-queue"
path     = "~/.local/share/trailboss/queue.jsonl"
id_field = "id"
provider = "claude"
runtime  = "background"
prompt_template   = "Fix the issue in {{.path}} line {{.line}}:\n\n{{.body}}"
tab_name_template = "fix: {{.path}}:{{.line}}"
```

All JSON fields are available in templates. `{{.path}}`, `{{.body}}`, `{{.type}}` — whatever's in the record.

---

## Neovim

Select code in visual mode and dispatch it as a trail without leaving the editor.

**Install** (lazy.nvim):

```lua
{
  dir = "/path/to/trailboss/nvim",
  config = function()
    require("trailboss").setup({
      source_path = "~/.local/share/trailboss/comments.jsonl",
      keys = {
        act = "<leader>tx",   -- act on selection (implement, fix, refactor)
        ask = "<leader>ta",   -- ask about selection (explain, don't modify)
      },
    })
  end,
}
```

Select some code, hit the keybind, optionally add steering context at the prompt, and Trailboss sends a rider.

Make sure your trailboss config knows about where your nvim plugin is writing orders to — see [config.example.toml](daemon/config.example.toml).

---

## Configuration Reference

### Global

| Field | Description |
|---|---|
| `state_path` | Seen-ID dedup store (default `~/.local/state/trailboss/state.json`) |
| `sessions_path` | Session store (default `~/.local/state/trailboss/sessions.jsonl`) |
| `pid_path` | Daemon PID file (default `~/.local/state/trailboss/trailboss.pid`) |
| `default_provider` | Provider used by `ask`/`act` when `-p` is not passed (default `"claude"`) |
| `howdy` | `true` to enable the howdy ping for faster session ID resolution (default `false`) |

### Source

| Field | Description |
|---|---|
| `name` | Unique source name |
| `path` | Path to the JSONL file |
| `id_field` | JSON field used as the dedup key |
| `provider` | Which `[provider.*]` to dispatch to; omit to use `default_provider` |
| `runtime` | `background` (default) or a named `[runtime.*]` |
| `dangerous` | `true` to append `dangerous_args` on dispatch (default `false`) |
| `prompt_template` | Go template; all JSON fields available |
| `tab_name_template` | Go template for the session label |

### Provider

| Field | Description |
|---|---|
| `command` | CLI binary |
| `launch_args` | Args for new sessions or the howdy ping — `{{.Prompt}}`, `{{.SessionID}}`, `{{.CWD}}` |
| `continue_args` | (howdy) args for the real background job — `{{.Prompt}}`, `{{.SessionID}}`, `{{.CWD}}` |
| `dangerous_args` | Extra args appended by `trailboss act` (e.g. `--dangerously-skip-permissions`) |
| `resume_args` | Args for `trailboss resume` — `{{.SessionID}}` |
| `session_from` | `howdy` — ping to get session ID; `pre-assign` — UUID before launch; `poll-dir` — scan dir |
| `howdy_prompt` | Ping prompt sent to the agent (default `"howdy partner"`) |
| `session_id_field` | JSON field to extract session ID from howdy response (default `"session_id"`) |
| `session_dir` | (poll-dir) dir to watch — `{{.CWD}}`, `{{.CWDEncoded}}`, `{{.Year}}`, `{{.Month}}`, `{{.Day}}` |

---

## Why not just use Claude Code?

You absolutely can.

Trailboss becomes useful when you want:

- A consistent interface across providers
- Background execution
- Editor integrations
- Queue-based dispatch
- Reusable prompt templates
- One command that works everywhere

Trailboss doesn't replace agents.

It standardizes how you launch them.

---

## Development

```bash
# build the CLI
make build-cli

# install the CLI, zellij plugin, nvim integration, and default config
make install

# create a local dev config
cp config.dev.example config.dev.toml

# run the daemon in the foreground against the dev config
make dev

# symlink the nvim plugin into your neovim config
make install-nvim
```

The dev config watches `/tmp/trailboss-dev.jsonl`. To simulate a work item:

```bash
echo '{"id":"1","type":"act","path":"foo.go","line":1,"end_line":5,"body":"do something"}' >> /tmp/trailboss-dev.jsonl
```

Then check sessions:

```bash
./bin/trailboss trails -c config.dev.toml
```
