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
	"github.com/KitsuneSemCalda/OmaVM/internal/backend/container"
	"github.com/KitsuneSemCalda/OmaVM/internal/backend/qemu"
	"github.com/KitsuneSemCalda/OmaVM/internal/core"
)

func main() {
	// Safe to install as the global slog default here (unlike the GUI):
	// the CLI never loads GTK, so there's no library-internal log
	// spam that could get redirected into this file.
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
	svc := core.NewService(store, container.New(), qemuBackend)

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
	case "status":
		return cmdStatus(ctx, svc, rest)
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
  status NAME [--json]                                     show environment status
  exec NAME -- CMD [ARGS...]                               run a command inside a Box
  rm NAME                                                  remove an environment
  list [--json]                                            list known environments`)
}
