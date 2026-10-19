// Command runwell-runner does work Runwell hands it, on a machine you
// control: it pulls a job (a piece of work and what to do about it), clones
// the repository into a fresh directory, runs your coding agent's CLI on it
// (Claude Code, Codex, aider, a script), streams the agent's output back to
// Runwell as it goes, and finishes with a branch, a pull request and a
// summary for a person to approve. Runwell never runs commands itself.
//
//	runwell-runner init --url https://runwell.example.com --token rwm_… --repos acme/website
//	runwell-runner doctor
//	runwell-runner run
//
// It only touches repositories on its own allowlist, whatever Runwell asks.
// Run it in a container or a VM: the agent it runs can do whatever its
// user can. See docs/runners.md.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

// version is set at build time (-X main.version=…).
var version = "dev"

func main() {
	os.Exit(exitCode())
}

// exitCode runs the command, answering 2 for a usage mistake, 1 for any
// other error.
func exitCode() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, stop, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "runwell-runner:", err)
		if _, ok := errors.AsType[usageError](err); ok {
			return 2
		}
		return 1
	}
	return 0
}

type usageError struct{ error }

func run(ctx context.Context, stop func(), args []string, out, errOut io.Writer) error {
	if len(args) == 0 {
		usage(errOut)
		return usageError{errors.New("say what to do")}
	}
	switch args[0] {
	case "init":
		return initCmd(args[1:], out)
	case "doctor":
		return doctorCmd(ctx, args[1:], out)
	case "run":
		return runCmd(ctx, stop, args[1:], errOut)
	case "version", "--version":
		fmt.Fprintln(out, "runwell-runner", version)
		return nil
	case "help", "-h", "--help":
		usage(out)
		return nil
	}
	usage(errOut)
	return usageError{fmt.Errorf("unknown command %q", args[0])}
}

func usage(w io.Writer) {
	fmt.Fprint(w, `runwell-runner: does the work Runwell hands it, on this machine.

  runwell-runner init --url URL --token rwm_… --repos owner/name[,owner/*]
        [--work-dir DIR] [--concurrency 1] [--agent 'claude -p … < {prompt_file}']
        [--timeout 60] [--config FILE] [--force]
      Writes the config file (~/.config/runwell-runner/config.json unless --config).

  runwell-runner doctor [--config FILE]
      Checks git, gh (and that it's signed in), rg, mise and the agent command,
      and that Runwell answers.

  runwell-runner run [--config FILE] [--once] [--no-push] [--no-pr] [--log-format text|json]
      Pulls jobs and does them until stopped (SIGTERM or Ctrl-C finish cleanly).
      --no-push keeps the commit local (nothing leaves the machine);
      --no-pr pushes the branch but opens no pull request (a local or non-GitHub
      remote, or a dry run). --once does one job and exits.

The agent command is run with sh -c in the checkout, with {prompt_file},
{workdir}, {branch} and {job_id} replaced (shell-quoted). It may write a summary
to $RUNWELL_SUMMARY_FILE; otherwise its last message is the summary.
`)
}

// fail is the flag set's own error, as a usage error.
func parseFlags(fs *flag.FlagSet, args []string) error {
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		return usageError{err}
	}
	if fs.NArg() > 0 {
		return usageError{fmt.Errorf("unexpected %q", fs.Arg(0))}
	}
	return nil
}

func newLogger(format string, w io.Writer) (*slog.Logger, error) {
	switch format {
	case "", "text":
		return slog.New(slog.NewTextHandler(w, nil)), nil
	case "json":
		return slog.New(slog.NewJSONHandler(w, nil)), nil
	}
	return nil, usageError{fmt.Errorf("--log-format is text or json, not %q", format)}
}
