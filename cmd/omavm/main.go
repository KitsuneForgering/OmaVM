// Command omavm is the CLI front end for the OmaVM Core. It contains no
// environment-management logic of its own: every operation is a thin
// call into core.Service, the same Core a future GUI will call.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/KitsuneSemCalda/OmaVM/internal/applog"
	"github.com/KitsuneSemCalda/OmaVM/internal/backend/box"
	"github.com/KitsuneSemCalda/OmaVM/internal/backend/container"
	"github.com/KitsuneSemCalda/OmaVM/internal/backend/distrobox"
	"github.com/KitsuneSemCalda/OmaVM/internal/backend/qemu"
	"github.com/KitsuneSemCalda/OmaVM/internal/core"
)

func main() {
	// Safe to install as the global slog default here: the CLI owns no
	// GUI toolkit whose internal messages could pollute this log.
	if logger, closeLog, err := applog.Open("omavm"); err != nil {
		fmt.Fprintln(os.Stderr, "omavm: warning: logging disabled:", err)
	} else {
		slog.SetDefault(logger)
		defer closeLog()
	}

	if err := run(os.Args[1:]); err != nil {
		slog.Error("command failed", "error", err)
		fmt.Fprintln(os.Stderr, "omavm:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		printUsage()
		return errors.New("no command given")
	}

	qemuBackend, err := qemu.New()
	if err != nil {
		return err
	}
	store, err := core.NewFileStore()
	if err != nil {
		return err
	}
	boxBackend := box.New(distrobox.New(), container.New())
	svc := core.NewService(store, boxBackend, qemuBackend)

	ctx := context.Background()
	cmd, rest := args[0], args[1:]
	slog.Info("command", "cmd", cmd, "args", rest)

	switch cmd {
	case "create":
		return cmdCreate(ctx, svc, rest)
	case "start":
		return cmdSimple(ctx, rest, "start", svc.Start)
	case "open":
		return cmdSimple(ctx, rest, "open", svc.Open)
	case "stop":
		return cmdSimple(ctx, rest, "stop", svc.Stop)
	case "restart":
		return cmdSimple(ctx, rest, "restart", svc.Restart)
	case "pause":
		return cmdSimple(ctx, rest, "pause", svc.Pause)
	case "resume":
		return cmdSimple(ctx, rest, "resume", svc.Resume)
	case "force-stop":
		return cmdSimple(ctx, rest, "force-stop", svc.ForceStop)
	case "status":
		return cmdStatus(ctx, svc, rest)
	case "integration":
		return cmdIntegration(ctx, svc, rest)
	case "settings", "configure":
		return cmdSettings(ctx, svc, rest)
	case "preview":
		return cmdPreview(ctx, svc, rest)
	case "exec":
		return cmdExec(ctx, svc, rest)
	case "rm", "remove":
		return cmdSimple(ctx, rest, "remove", svc.Remove)
	case "list", "ls":
		return cmdList(ctx, svc, rest)
	case "help", "-h", "--help":
		printUsage()
		return nil
	default:
		printUsage()
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func cmdCreate(ctx context.Context, svc *core.Service, args []string) error {
	fs := flag.NewFlagSet("create", flag.ContinueOnError)
	name := fs.String("name", "", "environment name (required)")
	kindStr := fs.String("kind", "box", `environment kind: "box" or "machine"`)
	image := fs.String("image", "", "distro image (Box) or boot ISO path (Machine)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" {
		return errors.New("create: --name is required")
	}
	kind, err := core.ParseEnvironmentKind(*kindStr)
	if err != nil {
		return err
	}

	env, err := svc.Create(ctx, core.Environment{Name: *name, Image: *image, Kind: kind})
	if err != nil {
		return err
	}
	fmt.Printf("created %s (%s, backend=%s)\n", env.Name, env.Kind, env.Backend)
	return nil
}

func cmdSimple(ctx context.Context, args []string, verb string, fn func(context.Context, string) error) error {
	if len(args) != 1 {
		return fmt.Errorf("%s: expected exactly one environment name", verb)
	}
	return fn(ctx, args[0])
}

func cmdStatus(ctx context.Context, svc *core.Service, args []string) error {
	// Not flag.FlagSet: CLAUDE.md's own CLI examples put --json after the
	// positional name (`omavm status radic --json`), and the stdlib flag
	// package stops parsing flags at the first non-flag argument, so it
	// would silently swallow a trailing --json instead of honoring it.
	args, jsonOut := extractBoolFlag(args, "json")
	if len(args) != 1 {
		return errors.New("status: expected exactly one environment name")
	}

	status, err := svc.Status(ctx, args[0])
	if err != nil {
		return err
	}
	if jsonOut {
		return json.NewEncoder(os.Stdout).Encode(status)
	}
	if status.Detail != "" {
		fmt.Printf("%s (%s)\n", status.State, status.Detail)
	} else {
		fmt.Println(status.State)
	}
	return nil
}

func cmdPreview(ctx context.Context, svc *core.Service, args []string) error {
	if len(args) != 1 {
		return errors.New("preview: expected exactly one environment name")
	}
	path, err := svc.Preview(ctx, args[0])
	if err != nil {
		return err
	}
	fmt.Println(path)
	return nil
}

func cmdIntegration(ctx context.Context, svc *core.Service, args []string) error {
	args, jsonOut := extractBoolFlag(args, "json")
	if len(args) != 1 {
		return errors.New("integration: expected exactly one environment name")
	}
	report, err := svc.Integration(ctx, args[0])
	if err != nil {
		return err
	}
	if jsonOut {
		return json.NewEncoder(os.Stdout).Encode(report)
	}
	fmt.Printf("guest agent: %s", report.GuestAgent)
	if report.Hint != "" {
		fmt.Printf(" (%s)", report.Hint)
	}
	fmt.Println()
	return nil
}

func cmdSettings(ctx context.Context, svc *core.Service, args []string) error {
	if len(args) == 0 {
		return errors.New("settings: expected an environment name")
	}
	name := args[0]
	fs := flag.NewFlagSet("settings", flag.ContinueOnError)
	description := fs.String("description", "", "human-readable description")
	cpus := fs.Int("cpus", 0, "virtual CPUs (Machines only)")
	memory := fs.Int("memory-mib", 0, "memory in MiB (Machines only)")
	sharedPath := fs.String("shared-path", "", "host directory to share (Machines only)")
	sharedReadOnly := fs.Bool("shared-read-only", false, "make the shared folder read-only")
	sharedWritable := fs.Bool("shared-writable", false, "make the shared folder writable")
	jsonOut := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("settings: unexpected positional arguments")
	}
	patch := core.SettingsPatch{}
	changed := false
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "description":
			patch.Description = description
			changed = true
		case "cpus":
			patch.CPUs = cpus
			changed = true
		case "memory-mib":
			patch.MemoryMiB = memory
			changed = true
		case "shared-path":
			patch.SharedPath = sharedPath
			changed = true
		case "shared-read-only":
			if *sharedWritable {
				return
			}
			patch.SharedReadOnly = sharedReadOnly
			changed = true
		case "shared-writable":
			patch.SharedReadOnly = new(bool)
			changed = true
		}
	})
	if *sharedReadOnly && *sharedWritable {
		return errors.New("settings: choose only one of --shared-read-only or --shared-writable")
	}
	var settings core.EnvironmentSettings
	var err error
	if changed {
		settings, err = svc.Configure(ctx, name, patch)
	} else {
		settings, err = svc.Settings(ctx, name)
	}
	if err != nil {
		return err
	}
	if *jsonOut {
		return json.NewEncoder(os.Stdout).Encode(settings)
	}
	if settings.Description != "" {
		fmt.Println("description:", settings.Description)
	}
	if settings.CPUs != 0 {
		fmt.Printf("hardware: %d CPUs, %d MiB memory\n", settings.CPUs, settings.MemoryMiB)
	}
	if settings.SharedPath != "" {
		mode := "writable"
		if settings.SharedReadOnly {
			mode = "read-only"
		}
		fmt.Printf("shared folder: %s (%s)\n", settings.SharedPath, mode)
	}
	return nil
}

func cmdExec(ctx context.Context, svc *core.Service, args []string) error {
	if len(args) < 2 {
		return errors.New("exec: usage: omavm exec <name> -- <command> [args...]")
	}
	name := args[0]
	rest := args[1:]
	if rest[0] == "--" {
		rest = rest[1:]
	}
	if len(rest) == 0 {
		return errors.New("exec: no command given")
	}
	return svc.Exec(ctx, name, rest)
}

func cmdList(ctx context.Context, svc *core.Service, args []string) error {
	_, jsonOut := extractBoolFlag(args, "json")

	envs, err := svc.List(ctx)
	if err != nil {
		return err
	}
	if jsonOut {
		if envs == nil {
			envs = []core.Environment{}
		}
		return json.NewEncoder(os.Stdout).Encode(envs)
	}
	if len(envs) == 0 {
		fmt.Println("no environments yet — try: omavm create --name <name> --kind box --image <distro>")
		return nil
	}
	for _, env := range envs {
		fmt.Printf("%s\t%s\t%s\t%s\n", env.Name, env.Kind, env.Image, env.Backend)
	}
	return nil
}

// extractBoolFlag pulls a --name boolean flag out of args regardless of
// its position, returning the remaining positional args and whether the
// flag was present. Go's flag.FlagSet can't do this: it stops parsing
// flags at the first non-flag argument, which breaks the
// "flag after the positional name" ordering CLAUDE.md's CLI examples use.
func extractBoolFlag(args []string, name string) ([]string, bool) {
	needle := "--" + name
	out := make([]string, 0, len(args))
	found := false
	for _, a := range args {
		if a == needle {
			found = true
			continue
		}
		out = append(out, a)
	}
	return out, found
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `usage: omavm <command> [arguments]

commands:
  create --name NAME --kind box|machine [--image IMAGE]   create an environment
  start NAME                                              start an environment
  open NAME                                                start (if needed) and attach
  stop NAME                                                stop an environment
  restart NAME                                             restart a Machine
  pause NAME                                               pause a Machine
  resume NAME                                              resume a Machine
  force-stop NAME                                          immediately stop a Machine
  status NAME [--json]                                     show environment status
  integration NAME [--json]                                check Machine guest tools
  settings NAME [--description TEXT] [--cpus N]            view or change settings
  preview NAME                                             capture a Machine screenshot
  exec NAME -- CMD [ARGS...]                               run a command inside a Box
  rm NAME                                                  remove an environment
  list [--json]                                            list known environments`)
}
