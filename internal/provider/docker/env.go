package docker

import (
	"bytes"
	"context"
	"errors"
	"io"

	"github.com/perylemke/kaxa/internal/sandbox"
)

// writeEnv stores the sandbox environment for later commands. The file content
// is the command's stdin, and the command's own output is discarded.
func (s *session) writeEnv(ctx context.Context, entries []string) error {
	if len(entries) == 0 {
		return nil
	}
	path := sandbox.EnvFilePath(agentHome)
	steps := []struct {
		argv []string
		in   io.Reader
	}{
		{[]string{"mkdir", "-p", sandbox.EnvDirPath(agentHome)}, nil},
		{[]string{"tee", path}, bytes.NewReader(sandbox.FormatEnvFile(entries))},
		{[]string{"chmod", "600", path}, nil},
	}
	for _, step := range steps {
		if err := s.runQuiet(ctx, step.argv, step.in); err != nil {
			return err
		}
	}
	return nil
}

// readEnv loads the file written at create. A missing file is an empty
// environment, so a sandbox created before this feature still runs.
func (s *session) readEnv(ctx context.Context) ([]string, error) {
	var stdout bytes.Buffer
	err := s.run(ctx, runSpec{
		argv: []string{"cat", sandbox.EnvFilePath(agentHome)}, dir: agentHome, env: cloudLayout.Env(),
		stdout: &stdout, stderr: io.Discard,
	})
	if err != nil {
		if _, ok := errors.AsType[*sandbox.ExitError](err); ok {
			return nil, nil
		}
		return nil, err
	}
	return sandbox.ParseEnvFile(&stdout)
}

func (s *session) runQuiet(ctx context.Context, argv []string, stdin io.Reader) error {
	return s.run(ctx, runSpec{
		argv: argv, dir: agentHome, env: cloudLayout.Env(),
		stdin: stdin, stdout: io.Discard, stderr: io.Discard,
	})
}
