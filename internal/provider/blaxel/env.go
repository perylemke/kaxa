package blaxel

import (
	"bytes"
	"context"
	"io"

	"github.com/perylemke/kaxa/internal/sandbox"
)

// writeEnv stores the sandbox environment for later commands. The file content
// is the command's stdin, and the command's own output is discarded.
func (p *Provider) writeEnv(ctx context.Context, name string, entries []string) error {
	if len(entries) == 0 {
		return nil
	}
	_, rt, _, err := p.clients()
	if err != nil {
		return err
	}
	path := sandbox.EnvFilePath(blaxelHome)
	steps := []struct {
		argv []string
		in   io.Reader
	}{
		{[]string{"mkdir", "-p", sandbox.EnvDirPath(blaxelHome)}, nil},
		{[]string{"tee", path}, bytes.NewReader(sandbox.FormatEnvFile(entries))},
		{[]string{"chmod", "600", path}, nil},
	}
	for _, step := range steps {
		code, err := rt.Run(ctx, name, runSpec{
			argv: step.argv, dir: blaxelHome, env: blaxelLayout.Env(),
			stdin: step.in, stdout: io.Discard, stderr: io.Discard,
		})
		if err != nil {
			return err
		}
		if code != 0 {
			return &sandbox.ExitError{Code: code}
		}
	}
	return nil
}

// readEnv loads the file written at create. A missing file is an empty
// environment, so a sandbox created before this feature still runs.
func (p *Provider) readEnv(ctx context.Context, name string) ([]string, error) {
	_, rt, _, err := p.clients()
	if err != nil {
		return nil, err
	}
	var stdout bytes.Buffer
	code, err := rt.Run(ctx, name, runSpec{
		argv: []string{"cat", sandbox.EnvFilePath(blaxelHome)}, dir: blaxelHome, env: blaxelLayout.Env(),
		stdout: &stdout, stderr: io.Discard,
	})
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, nil
	}
	return sandbox.ParseEnvFile(&stdout)
}
