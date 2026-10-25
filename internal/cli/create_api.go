package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/perylemke/sbox/internal/project"
	"github.com/perylemke/sbox/internal/sandbox"
	"github.com/perylemke/sbox/internal/usererr"
)

const (
	defaultAPIPort  = 8000
	webtmuxHTTPPort = 8080
	spriteHomeDir   = "/home/sprite"
	spriteWorkspace = "/home/sprite/workspace"
)

type createFlags struct {
	Running bool
	Sync    bool
	Dir     string
	DirSet  bool
	API     bool
	APICmd  string
	APIPort int
	Public  bool
}

func applyAPIFlags(req *createRequest, flags createFlags, portSet bool) error {
	if !flags.API {
		if flags.APICmd != "" {
			return usererr.New("--api-cmd requires --api", "pass --api to publish the service")
		}
		if portSet {
			return usererr.New("--api-port requires --api", "pass --api to publish the service")
		}
		req.Public = flags.Public
		return nil
	}
	argv, err := parseAPICommand(flags.APICmd)
	if err != nil {
		return err
	}
	if flags.APIPort < 1 || flags.APIPort > 65535 {
		return usererr.New(
			fmt.Sprintf("invalid api port %d", flags.APIPort),
			"use a TCP port from 1 to 65535",
		)
	}
	if flags.Running && flags.APIPort == webtmuxHTTPPort {
		return usererr.New(
			"api port 8080 is used by the tmux service",
			"choose another --api-port",
		)
	}
	req.API = true
	req.APIPort = flags.APIPort
	req.APICmd = argv
	req.Public = flags.Public
	return nil
}

func parseAPICommand(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, usererr.New(
			"api command is required",
			"pass --api-cmd with comma-separated argv; the first item is the binary",
		)
	}
	parts := strings.Split(raw, ",")
	argv := make([]string, 0, len(parts))
	for _, part := range parts {
		token := strings.TrimSpace(part)
		if token == "" {
			return nil, usererr.New(
				"api command has an empty argument",
				"separate argv with commas and omit empty items",
			)
		}
		argv = append(argv, token)
	}
	return argv, nil
}

func requireAPIProject(req createRequest) error {
	if !req.API || !req.Sync {
		return nil
	}
	ok, err := project.IsAPI(req.Root)
	if err != nil {
		return err
	}
	if !ok {
		return usererr.New(
			"this repository is not an HTTP API",
			"omit --api, or run create from an API project",
		)
	}
	return nil
}

func requirePublishers(p sandbox.Provider, req createRequest) error {
	if req.API {
		if _, ok := p.(sandbox.APIPublisher); !ok {
			return usererr.New(
				"api publish is only supported for the sprites provider",
				"omit --api or pass --provider sprites",
			)
		}
	}
	if req.Public {
		if _, ok := p.(sandbox.PublicURL); !ok {
			return usererr.New(
				"public urls are only supported for the sprites provider",
				"omit --public or pass --provider sprites",
			)
		}
	}
	return nil
}

func publishAPIIfRequested(ctx context.Context, errOut io.Writer, p sandbox.Provider, id sandbox.ID, req createRequest) error {
	if !req.API {
		return nil
	}
	publisher, ok := p.(sandbox.APIPublisher)
	if !ok {
		return destroyAfterFailure(ctx, p, id, usererr.New(
			"api publish is only supported for the sprites provider",
			"omit --api or pass --provider sprites",
		), "publish api")
	}
	workDir, err := apiWorkDir(req)
	if err != nil {
		return destroyAfterFailure(ctx, p, id, err, "publish api")
	}
	err = runTaskWithSpinner(ctx, errOut, "publishing api…", func(ctx context.Context) error {
		return publisher.PublishAPI(ctx, id, workDir, req.APIPort, req.APICmd)
	})
	if err != nil {
		return destroyAfterFailure(ctx, p, id, err, "publish api")
	}
	return nil
}

func publishURLIfRequested(ctx context.Context, p sandbox.Provider, id sandbox.ID, req createRequest) error {
	if !req.Public {
		return nil
	}
	setter, ok := p.(sandbox.PublicURL)
	if !ok {
		return usererr.New(
			"public urls are only supported for the sprites provider",
			"omit --public or pass --provider sprites",
		)
	}
	if err := setter.SetURLAuth(ctx, id, sandbox.URLAuthPublic); err != nil {
		return fmt.Errorf("set public url: %w", err)
	}
	return nil
}

func apiWorkDir(req createRequest) (string, error) {
	if !req.Sync {
		return spriteHomeDir, nil
	}
	if !validRepoSegment(req.Repo) {
		return "", usererr.New(
			fmt.Sprintf("invalid repository directory %q", req.Repo),
			"use a single path segment",
		)
	}
	return spriteWorkspace + "/" + req.Repo, nil
}

func validRepoSegment(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, `/\`) && !strings.ContainsRune(name, 0)
}

func createdMessage(id sandbox.ID, url string, req createRequest) string {
	if !req.API && !req.Public {
		return fmt.Sprintf("sandbox %s is ready and will stay running for 1 hour\n%s", id, url)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "sandbox %s is ready", id)
	if req.Running {
		b.WriteString(" and will stay running for 1 hour")
	}
	fmt.Fprintf(&b, "\n%s", url)
	if req.API {
		fmt.Fprintf(&b, " forwards to port %d", req.APIPort)
	}
	switch {
	case req.Public:
		b.WriteString("\naccess is public")
	case req.API:
		b.WriteString("\nrequires a Sprites account")
	}
	return b.String()
}
