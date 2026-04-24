package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"text/tabwriter"

	"github.com/Vaniell0/wallforge/internal/apply"
	"github.com/Vaniell0/wallforge/internal/config"
	"github.com/Vaniell0/wallforge/internal/workspace"
)

// cmdWorkspace is the top-level dispatcher for `wallforge workspace *`.
// The subcommands are plain verbs — no Cobra — because the surface is
// small and CLI stability matters more than feature density.
func cmdWorkspace(cfg config.Config, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("workspace: expected a subcommand (bind|unbind|list|daemon)")
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "bind":
		return cmdWorkspaceBind(rest)
	case "unbind":
		return cmdWorkspaceUnbind(rest)
	case "list":
		return cmdWorkspaceList()
	case "daemon":
		return cmdWorkspaceDaemon(cfg)
	default:
		return fmt.Errorf("workspace: unknown subcommand %q", sub)
	}
}

func cmdWorkspaceBind(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("workspace bind: expected <workspace> <path|id>, got %d args", len(args))
	}
	ws, input := args[0], args[1]
	b, err := workspace.Load()
	if err != nil {
		return err
	}
	b.ByWorkspace[ws] = input
	if err := workspace.Save(b); err != nil {
		return err
	}
	fmt.Printf("wallforge: bound workspace %q → %s\n", ws, input)
	return nil
}

func cmdWorkspaceUnbind(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("workspace unbind: expected <workspace>, got %d args", len(args))
	}
	ws := args[0]
	b, err := workspace.Load()
	if err != nil {
		return err
	}
	if _, ok := b.ByWorkspace[ws]; !ok {
		fmt.Fprintf(os.Stderr, "wallforge: no binding for workspace %q — nothing to do\n", ws)
		return nil
	}
	delete(b.ByWorkspace, ws)
	if err := workspace.Save(b); err != nil {
		return err
	}
	fmt.Printf("wallforge: removed binding for workspace %q\n", ws)
	return nil
}

func cmdWorkspaceList() error {
	b, err := workspace.Load()
	if err != nil {
		return err
	}
	if len(b.ByWorkspace) == 0 {
		fmt.Println("No workspace bindings.")
		return nil
	}
	keys := make([]string, 0, len(b.ByWorkspace))
	for k := range b.ByWorkspace {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "WORKSPACE\tINPUT")
	for _, k := range keys {
		fmt.Fprintf(w, "%s\t%s\n", k, b.ByWorkspace[k])
	}
	return w.Flush()
}

func cmdWorkspaceDaemon(cfg config.Config) error {
	conn, err := workspace.Dial()
	if err != nil {
		return err
	}
	defer conn.Close()

	ctx, cancel := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer cancel()

	fmt.Fprintln(os.Stderr, "wallforge: workspace daemon listening on Hyprland event socket")
	runner := workspace.NewRunner(func(input string) error {
		_, err := apply.ByInput(cfg, input)
		if err != nil {
			fmt.Fprintf(os.Stderr, "wallforge: workspace apply %s failed: %v\n", input, err)
		}
		return err
	})
	return runner.Run(ctx, conn)
}
