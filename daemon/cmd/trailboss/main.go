package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/rockorager/trailboss/internal"
	"golang.org/x/sys/unix"
)

var version = "dev"

const usage = `Usage:
  trailboss <command> [flags]

Commands:
  start        start the daemon in the background
  stop         stop the daemon
  status       show daemon status
  daemon       run in the foreground (used by start, launchd, brew services)
  trails, ls, list
               list background sessions (--last N, default 10)
  rm <id>      remove a session by ID
  clear        remove all sessions
  ask          send a question to a background agent (explain, don't modify)
  act          send an action to a background agent (implement, fix, refactor)
  resume <id>  resume a completed background session

Flags:
  -c, -config <path>    config file (default ~/.config/trailboss/config.toml)
  -v, -version          show version
  -h, -help             show this help
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		return
	}
	var err error
	switch os.Args[1] {
	case "start":
		err = runStart(os.Args[2:])
	case "stop":
		err = runStop(os.Args[2:])
	case "status":
		err = runStatus(os.Args[2:])
	case "daemon":
		err = runDaemon(os.Args[2:])
	case "trails", "ls", "list":
		err = runLS(os.Args[2:])
	case "rm":
		err = runRM(os.Args[2:])
	case "clear":
		err = runClear(os.Args[2:])
	case "ask":
		err = runAsk(os.Args[2:])
	case "act":
		err = runAct(os.Args[2:])
	case "resume":
		err = runResume(os.Args[2:])
	case "-v", "--version", "version":
		fmt.Println(version)
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", os.Args[1], usage)
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runStart(args []string) error {
	fs := flagSet("start")
	cfgPath := configFlag(fs)
	fs.Parse(args)

	cfg, err := internal.LoadConfig(*cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if pid, err := readPID(cfg.PIDPath); err == nil {
		if pidAlive(pid) {
			return fmt.Errorf("daemon already running (pid %d)", pid)
		}
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find executable: %w", err)
	}

	cmd := exec.Command(exe, "daemon", "-c", *cfgPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start daemon: %w", err)
	}

	if err := writePID(cfg.PIDPath, cmd.Process.Pid); err != nil {
		return err
	}
	fmt.Printf("daemon started (pid %d)\n", cmd.Process.Pid)
	return nil
}

func runStop(args []string) error {
	fs := flagSet("stop")
	cfgPath := configFlag(fs)
	fs.Parse(args)

	cfg, err := internal.LoadConfig(*cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	pid, err := readPID(cfg.PIDPath)
	if err != nil {
		return fmt.Errorf("daemon not running (no pidfile)")
	}

	proc, err := os.FindProcess(pid)
	if err != nil || !pidAlive(pid) {
		os.Remove(cfg.PIDPath)
		return fmt.Errorf("daemon not running")
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("signal daemon: %w", err)
	}
	os.Remove(cfg.PIDPath)
	fmt.Printf("daemon stopped (pid %d)\n", pid)
	return nil
}

func runStatus(args []string) error {
	fs := flagSet("status")
	cfgPath := configFlag(fs)
	fs.Parse(args)

	cfg, err := internal.LoadConfig(*cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	pid, err := readPID(cfg.PIDPath)
	if err != nil || !pidAlive(pid) {
		fmt.Println("stopped")
		return nil
	}
	fmt.Printf("running (pid %d)\n", pid)
	return nil
}

func runDaemon(args []string) error {
	fs := flagSet("daemon")
	cfgPath := configFlag(fs)
	fs.Parse(args)

	cfg, err := internal.LoadConfig(*cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	state, err := internal.LoadState(cfg.StatePath)
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	sessions := internal.NewSessionStore(cfg.SessionsPath)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create watcher: %w", err)
	}
	defer watcher.Close()

	for _, src := range cfg.Sources {
		src = sourceWithDefaults(src, cfg)
		if err := watcher.Add(src.Path); err != nil {
			slog.Warn("watch source", "path", src.Path, "err", err)
		}
		slog.Info("watching", "source", src.Name, "path", src.Path)

		launch := launchFn(src, cfg, sessions)
		if err := internal.ProcessSource(src, state, launch, cfg.Providers); err != nil {
			slog.Warn("initial scan", "source", src.Name, "err", err)
		}
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	slog.Info("trailboss ready", "version", version)
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) {
				continue
			}
			for _, src := range cfg.Sources {
				src = sourceWithDefaults(src, cfg)
				if src.Path != event.Name {
					continue
				}
				launch := launchFn(src, cfg, sessions)
				if err := internal.ProcessSource(src, state, launch, cfg.Providers); err != nil {
					slog.Error("process source", "source", src.Name, "err", err)
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			slog.Error("watcher", "err", err)
		case <-stop:
			slog.Info("shutting down")
			return nil
		}
	}
}

func runLS(args []string) error {
	fs := flagSet("trails")
	cfgPath := configFlag(fs)
	last := fs.Int("last", 10, "max trails to show")
	jsonOutput := fs.Bool("json", false, "output trails as JSON")
	fs.Parse(args)

	cfg, err := internal.LoadConfig(*cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	sessions := internal.NewSessionStore(cfg.SessionsPath)
	list, err := sessions.List()
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}

	if len(list) == 0 {
		if *jsonOutput {
			fmt.Println("[]")
			return nil
		}
		fmt.Println("no sessions")
		return nil
	}

	total := len(list)
	truncated := *last > 0 && total > *last
	if truncated {
		list = list[total-*last:]
	}

	if *jsonOutput {
		return printTrailJSON(os.Stdout, list, cfg)
	}

	if truncated {
		fmt.Printf("Showing last %d of %d trails\n", *last, total)
	}

	if terminalWidth(os.Stdout) < 100 {
		printTrailBlocks(os.Stdout, list, cfg)
		return nil
	}
	return printTrailTable(os.Stdout, list, cfg)
}

func trailStatus(s internal.SessionStatus) string {
	switch {
	case s.Done:
		return "done"
	case s.Failed:
		return "failed"
	default:
		return "running"
	}
}

func trailResume(s internal.SessionStatus, cfg internal.Config) string {
	if !s.Done {
		return ""
	}
	provider, ok := cfg.Providers[s.Provider]
	if !ok {
		return ""
	}
	resumeArgs := internal.RenderArgs(provider.ResumeArgs, "", s.SessionID, "")
	return provider.Command + " " + strings.Join(resumeArgs, " ")
}

type trailOutput struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	StartedAt time.Time `json:"started_at"`
	Provider  string    `json:"provider"`
	SessionID string    `json:"session_id,omitempty"`
	CWD       string    `json:"cwd,omitempty"`
	Resume    string    `json:"resume,omitempty"`
}

func printTrailJSON(out *os.File, list []internal.SessionStatus, cfg internal.Config) error {
	trails := make([]trailOutput, 0, len(list))
	for _, s := range list {
		trails = append(trails, trailOutput{
			ID:        s.ID,
			Name:      s.Name,
			Status:    trailStatus(s),
			StartedAt: s.StartedAt,
			Provider:  s.Provider,
			SessionID: s.SessionID,
			CWD:       s.CWD,
			Resume:    trailResume(s, cfg),
		})
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(trails)
}

func printTrailTable(out *os.File, list []internal.SessionStatus, cfg internal.Config) error {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tSTATUS\tAGE\tNAME\tRESUME")
	for _, s := range list {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", s.ID, trailStatus(s), age(s.StartedAt), s.Name, trailResume(s, cfg))
	}
	return w.Flush()
}

func printTrailBlocks(out *os.File, list []internal.SessionStatus, cfg internal.Config) {
	for i, s := range list {
		if i > 0 {
			fmt.Fprintln(out, "---")
		}
		fmt.Fprintf(out, "%s  %s  %s\n", s.ID, trailStatus(s), age(s.StartedAt))
		fmt.Fprintf(out, "name:   %s\n", s.Name)
		if resume := trailResume(s, cfg); resume != "" {
			fmt.Fprintf(out, "resume: %s\n", resume)
		}
	}
}

func terminalWidth(out *os.File) int {
	ws, err := unix.IoctlGetWinsize(int(out.Fd()), unix.TIOCGWINSZ)
	if err != nil || ws.Col == 0 {
		return 120
	}
	return int(ws.Col)
}

func runRM(args []string) error {
	fs := flagSet("rm")
	cfgPath := configFlag(fs)
	fs.Parse(args)

	if fs.NArg() == 0 {
		return fmt.Errorf("usage: trailboss rm <id>")
	}
	id := fs.Arg(0)

	cfg, err := internal.LoadConfig(*cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	sessions := internal.NewSessionStore(cfg.SessionsPath)
	if err := sessions.Remove(id); err != nil {
		return fmt.Errorf("remove session: %w", err)
	}
	fmt.Printf("removed %s\n", id)
	return nil
}

func runClear(args []string) error {
	fs := flagSet("clear")
	cfgPath := configFlag(fs)
	fs.Parse(args)

	fmt.Print("clear all sessions? [y/N] ")
	var answer string
	fmt.Scanln(&answer)
	if answer != "y" && answer != "Y" {
		fmt.Println("cancelled")
		return nil
	}

	cfg, err := internal.LoadConfig(*cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	sessions := internal.NewSessionStore(cfg.SessionsPath)
	if err := sessions.Clear(); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("clear sessions: %w", err)
	}
	fmt.Println("cleared")
	return nil
}

func runAsk(args []string) error {
	fs := flagSet("ask")
	cfgPath := configFlag(fs)
	providerName := fs.String("p", "", "provider to use")
	fs.Parse(args)

	prompt := strings.Join(fs.Args(), " ")
	if prompt == "" {
		return fmt.Errorf("usage: trailboss ask [-p provider] <prompt>")
	}

	cfg, err := internal.LoadConfig(*cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if *providerName == "" {
		*providerName = cfg.DefaultProvider
	}
	if *providerName == "" {
		*providerName = "claude"
	}

	provider, ok := cfg.Providers[*providerName]
	if !ok {
		return fmt.Errorf("unknown provider %q", *providerName)
	}

	name := prompt
	if len(name) > 50 {
		name = name[:47] + "..."
	}

	cwd, _ := os.Getwd()
	sessions := internal.NewSessionStore(cfg.SessionsPath)
	if err := internal.BackgroundLaunch("ask: "+name, prompt, cwd, *providerName, provider, sessions, cfg.Howdy); err != nil {
		return fmt.Errorf("launch: %w", err)
	}
	fmt.Println("queued")
	return nil
}

func runAct(args []string) error {
	fs := flagSet("act")
	cfgPath := configFlag(fs)
	providerName := fs.String("p", "", "provider to use")
	safe := fs.Bool("safe", false, "skip dangerous_args even if configured")
	fs.BoolVar(safe, "s", false, "skip dangerous_args even if configured")
	fs.Parse(args)

	prompt := strings.Join(fs.Args(), " ")
	if prompt == "" {
		return fmt.Errorf("usage: trailboss act [-p provider] [-s] <prompt>")
	}

	cfg, err := internal.LoadConfig(*cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if *providerName == "" {
		*providerName = cfg.DefaultProvider
	}
	if *providerName == "" {
		*providerName = "claude"
	}

	provider, ok := cfg.Providers[*providerName]
	if !ok {
		return fmt.Errorf("unknown provider %q", *providerName)
	}

	name := prompt
	if len(name) > 50 {
		name = name[:47] + "..."
	}

	if !*safe && len(provider.DangerousArgs) > 0 {
		p := provider
		if provider.SessionFrom == "howdy" {
			p.ContinueArgs = append(append([]string{}, provider.ContinueArgs...), provider.DangerousArgs...)
		} else {
			p.LaunchArgs = append(append([]string{}, provider.LaunchArgs...), provider.DangerousArgs...)
		}
		provider = p
	}

	cwd, _ := os.Getwd()
	sessions := internal.NewSessionStore(cfg.SessionsPath)
	if err := internal.BackgroundLaunch("act: "+name, prompt, cwd, *providerName, provider, sessions, cfg.Howdy); err != nil {
		return fmt.Errorf("launch: %w", err)
	}
	fmt.Println("queued")
	return nil
}

func runResume(args []string) error {
	fs := flagSet("resume")
	cfgPath := configFlag(fs)
	fs.Parse(args)

	if fs.NArg() == 0 {
		return fmt.Errorf("usage: trailboss resume <id>")
	}
	id := fs.Arg(0)

	cfg, err := internal.LoadConfig(*cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	sessions := internal.NewSessionStore(cfg.SessionsPath)
	list, err := sessions.List()
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}

	var target *internal.SessionStatus
	if id == "last" {
		for i := len(list) - 1; i >= 0; i-- {
			if list[i].Done {
				target = &list[i]
				break
			}
		}
		if target == nil {
			return fmt.Errorf("no completed sessions found")
		}
	} else {
		for i := range list {
			if list[i].ID == id {
				target = &list[i]
				break
			}
		}
		if target == nil {
			return fmt.Errorf("session %q not found", id)
		}
	}
	if !target.Done {
		return fmt.Errorf("session %q is not done (status: running or failed)", id)
	}

	provider, ok := cfg.Providers[target.Provider]
	if !ok {
		return fmt.Errorf("provider %q not found in config", target.Provider)
	}

	exe, err := exec.LookPath(provider.Command)
	if err != nil {
		return fmt.Errorf("find %q: %w", provider.Command, err)
	}

	resumeArgs := internal.RenderArgs(provider.ResumeArgs, "", target.SessionID, "")
	if target.CWD != "" {
		if err := os.Chdir(target.CWD); err != nil {
			return fmt.Errorf("chdir %q: %w", target.CWD, err)
		}
	}
	return syscall.Exec(exe, append([]string{provider.Command}, resumeArgs...), os.Environ())
}

func launchFn(src internal.SourceConfig, cfg internal.Config, sessions *internal.SessionStore) func(string, string, string, internal.ProviderConfig) error {
	return func(tabName, prompt, cwd string, provider internal.ProviderConfig) error {
		if src.Dangerous && len(provider.DangerousArgs) > 0 {
			p := provider
			if provider.SessionFrom == "howdy" {
				p.ContinueArgs = append(append([]string{}, provider.ContinueArgs...), provider.DangerousArgs...)
			} else {
				p.LaunchArgs = append(append([]string{}, provider.LaunchArgs...), provider.DangerousArgs...)
			}
			provider = p
		}
		switch src.Runtime {
		case "background", "":
			return internal.BackgroundLaunch(tabName, prompt, cwd, src.Provider, provider, sessions, cfg.Howdy)
		default:
			runtime, ok := cfg.Runtimes[src.Runtime]
			if !ok {
				return fmt.Errorf("unknown runtime %q", src.Runtime)
			}
			return internal.ZellijLaunch(tabName, prompt, provider, runtime)
		}
	}
}

func sourceWithDefaults(src internal.SourceConfig, cfg internal.Config) internal.SourceConfig {
	if src.Provider == "" {
		src.Provider = cfg.DefaultProvider
	}
	if src.Provider == "" {
		src.Provider = "claude"
	}
	return src
}

func flagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	fs.Usage = func() { fmt.Print(usage) }
	return fs
}

func configFlag(fs *flag.FlagSet) *string {
	p := fs.String("config", defaultConfigPath(), "config file path")
	fs.StringVar(p, "c", defaultConfigPath(), "config file path")
	return p
}

func readPID(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

func writePID(path string, pid int) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir pid dir: %w", err)
	}
	return os.WriteFile(path, []byte(strconv.Itoa(pid)), 0o644)
}

func pidAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

func age(t time.Time) string {
	d := time.Since(t).Round(time.Second)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
}

func defaultConfigPath() string {
	home, _ := os.UserHomeDir()
	return home + "/.config/trailboss/config.toml"
}
