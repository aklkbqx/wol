package localremote

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Command describes a process invocation. Env contains additions to the
// inherited environment. Keeping secrets in Env avoids exposing them in argv.
type Command struct {
	Name string
	Args []string
	Env  []string
}

// Result is the captured output of a command.
type Result struct {
	Stdout string
	Stderr string
}

// Runner executes commands without involving a shell.
type Runner interface {
	LookPath(string) (string, error)
	Run(context.Context, Command) (Result, error)
}

type execRunner struct{}

func (execRunner) LookPath(name string) (string, error) { return exec.LookPath(name) }

func (execRunner) Run(ctx context.Context, command Command) (Result, error) {
	cmd := exec.CommandContext(ctx, command.Name, command.Args...)
	cmd.Env = append(os.Environ(), command.Env...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return Result{Stdout: stdout.String(), Stderr: stderr.String()}, err
}

func runnerOrDefault(r Runner) Runner {
	if r != nil {
		return r
	}
	return execRunner{}
}

func commandError(action string, result Result, err error) error {
	if err == nil {
		return nil
	}
	detail := strings.TrimSpace(result.Stderr)
	if detail == "" {
		detail = strings.TrimSpace(result.Stdout)
	}
	if detail == "" {
		return fmt.Errorf("%s: %w", action, err)
	}
	return fmt.Errorf("%s: %w: %s", action, err, detail)
}

func joinErrors(errs ...error) error {
	var nonNil []error
	for _, err := range errs {
		if err != nil {
			nonNil = append(nonNil, err)
		}
	}
	return errors.Join(nonNil...)
}
