package modal

import (
	"bytes"
	"context"
	"errors"
	"io"

	"github.com/perylemke/kaxa/internal/sandbox"
)

// writeSandboxEnv stores the sandbox environment where a later command can read
// it. Modal's create map does not survive runuser, so exec reads this file.
// The content goes through stdin and the command output is discarded.
func writeSandboxEnv(ctx context.Context, c sdkClient, name string, entries []string) error {
	if len(entries) == 0 {
		return nil
	}
	path := sandbox.EnvFilePath(modalHome)
	steps := []struct {
		argv []string
		in   io.Reader
	}{
		{[]string{"mkdir", "-p", sandbox.EnvDirPath(modalHome)}, nil},
		{[]string{"tee", path}, bytes.NewReader(sandbox.FormatEnvFile(entries))},
		{[]string{"chmod", "600", path}, nil},
	}
	for _, step := range steps {
		if _, err := runQuiet(ctx, c, name, step.argv, step.in); err != nil {
			return err
		}
	}
	return nil
}

// readSandboxEnv loads the file written at create. A missing file is an empty
// environment, so a sandbox created before this feature still runs.
func readSandboxEnv(ctx context.Context, c sdkClient, name string) ([]string, error) {
	out, err := runQuiet(ctx, c, name, []string{"cat", sandbox.EnvFilePath(modalHome)}, nil)
	if err != nil {
		if _, ok := errors.AsType[*sandbox.ExitError](err); ok {
			return nil, nil
		}
		return nil, err
	}
	return sandbox.ParseEnvFile(bytes.NewReader(out))
}

// runQuiet runs argv and returns stdout. Stderr is discarded so a value in the
// environment file is not logged.
func runQuiet(ctx context.Context, c sdkClient, name string, argv []string, stdin io.Reader) ([]byte, error) {
	proc, err := c.Exec(ctx, name, argv, ExecParams{})
	if err != nil {
		return nil, err
	}
	defer proc.Close()
	if stdin == nil {
		stdin = bytes.NewReader(nil)
	}
	if _, err := io.Copy(proc.Stdin(), stdin); err != nil {
		return nil, err
	}
	if err := proc.Stdin().Close(); err != nil {
		return nil, err
	}
	var stdout bytes.Buffer
	if _, err := io.Copy(&stdout, proc.Stdout()); err != nil {
		return nil, err
	}
	if _, err := io.Copy(io.Discard, proc.Stderr()); err != nil {
		return nil, err
	}
	code, err := proc.Wait(ctx)
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, &sandbox.ExitError{Code: code}
	}
	return stdout.Bytes(), nil
}
