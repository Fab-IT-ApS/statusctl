// Package cli is the statusctl command line: a thin layer over ops, store and render.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/Fab-IT-ApS/statusctl/model"
	"github.com/Fab-IT-ApS/statusctl/ops"
	"github.com/Fab-IT-ApS/statusctl/store"
)

// Set at release build time with -ldflags -X.
var (
	Version     = "dev"
	ReleaseBase = "" // e.g. https://github.com/<org>/statusctl/releases/download
)

type app struct {
	stdout, stderr io.Writer
	stdin          io.Reader
	interactive    bool
	getenv         func(string) string
	now            func() time.Time
	runShell       func(command string) (string, error)
	runEditor      func(editor, file string) error
	gitUserName    func() string
	httpGet        func(url string) ([]byte, error)

	profile, dir, configPath string
	jsonOut                  bool
	draft                    string // message typed in the editor, kept if the post fails
}

func Main() int {
	a := &app{
		stdout: os.Stdout, stderr: os.Stderr, stdin: os.Stdin,
		interactive: term.IsTerminal(int(os.Stdin.Fd())),
		getenv:      os.Getenv, now: time.Now, runShell: runShell,
		runEditor: runEditor, gitUserName: gitUserName, httpGet: httpGet,
	}
	return a.run(os.Args[1:])
}

func (a *app) run(args []string) int {
	root := a.rootCmd()
	if _, _, err := root.Find(args); err != nil {
		fmt.Fprintln(a.stderr, "statusctl:", err)
		return 2
	}
	root.SetArgs(args)
	root.SetOut(a.stdout)
	root.SetErr(a.stderr)
	err := root.ExecuteContext(context.Background())
	if err != nil {
		fmt.Fprintln(a.stderr, "statusctl:", err)
	}
	return ExitCode(err)
}

func (a *app) rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "statusctl",
		Short:         "Post to and render a static status page",
		Version:       Version,
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return &ops.InputError{Msg: err.Error()} })
	pf := root.PersistentFlags()
	pf.StringVar(&a.profile, "profile", "", "config profile (default: $STATUSCTL_PROFILE, then default_profile)")
	pf.StringVar(&a.dir, "dir", "", "use this local directory instead of GitHub")
	pf.StringVar(&a.configPath, "config", "", "config file (default: <user config dir>/statusctl/config.yaml)")
	pf.BoolVar(&a.jsonOut, "json", false, "machine-readable output on stdout")
	root.AddCommand(a.validateCmd(), a.renderCmd(), a.statusCmd(), a.listCmd(),
		a.incidentCmd(), a.maintenanceCmd(), a.initCmd())
	return root
}

// ExitCode maps errors to the documented exit codes.
func ExitCode(err error) int {
	var ie *ops.InputError
	var ve *model.ValidationError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &ie), errors.As(err, &ve):
		return 2
	case errors.Is(err, store.ErrConflict):
		return 3
	case errors.Is(err, store.ErrAuth), errors.Is(err, store.ErrNetwork):
		return 4
	}
	return 1
}

func exactArgs(n int, what string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) != n {
			return &ops.InputError{Msg: fmt.Sprintf("%s expects %s", cmd.CommandPath(), what)}
		}
		return nil
	}
}

func (a *app) printJSON(v any) error {
	enc := json.NewEncoder(a.stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func (a *app) localDir() string {
	if a.dir == "" {
		return "."
	}
	return a.dir
}

func runShell(command string) (string, error) {
	var c *exec.Cmd
	if runtime.GOOS == "windows" {
		c = exec.Command("cmd", "/C", command)
	} else {
		c = exec.Command("sh", "-c", command)
	}
	c.Stderr = os.Stderr
	out, err := c.Output()
	return string(out), err
}
