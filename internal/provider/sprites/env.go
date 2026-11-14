package sprites

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"

	"github.com/perylemke/kaxa/internal/sandbox"
)

// writeSpriteEnv stores the sandbox environment for later commands. The file
// content is the command's stdin, and the command's own output is discarded.
func writeSpriteEnv(ctx context.Context, c sdkClient, name, token string, entries []string) error {
	if len(entries) == 0 {
		return nil
	}
	path := sandbox.EnvFilePath(spriteHome)
	steps := []struct {
		argv []string
		in   io.Reader
	}{
		{[]string{"mkdir", "-p", sandbox.EnvDirPath(spriteHome)}, nil},
		{[]string{"tee", path}, bytes.NewReader(sandbox.FormatEnvFile(entries))},
		{[]string{"chmod", "600", path}, nil},
	}
	for _, step := range steps {
		err := c.RunIn(ctx, name, spriteHome, spriteLayout.Env(), step.argv, step.in, io.Discard, io.Discard)
		if err != nil {
			return mapExecErr(err, token)
		}
	}
	return nil
}

// readSpriteEnv loads the file written at create. A missing file is an empty
// environment, so a sandbox created before this feature still runs.
func readSpriteEnv(ctx context.Context, c sdkClient, name, token string) ([]string, error) {
	var stdout bytes.Buffer
	err := c.RunIn(ctx, name, spriteHome, spriteLayout.Env(), []string{"cat", sandbox.EnvFilePath(spriteHome)}, nil, &stdout, io.Discard)
	if err != nil {
		mapped := mapExecErr(err, token)
		if _, ok := errors.AsType[*sandbox.ExitError](mapped); ok {
			return nil, nil
		}
		return nil, mapped
	}
	return sandbox.ParseEnvFile(strings.NewReader(stdout.String()))
}
