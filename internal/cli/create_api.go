package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/perylemke/kaxa/internal/lifecycle"
	"github.com/perylemke/kaxa/internal/usererr"
)

const (
	defaultAPIPort  = 8000
	webtmuxHTTPPort = lifecycle.TmuxPort
)

type createFlags struct {
	Running bool
	Sync    bool
	Dir     string
	DirSet  bool
	API     bool
	APICmd  string
	APIPort int
	APIEnv  []string
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
		if len(flags.APIEnv) > 0 {
			return usererr.New("--api-env requires --api", "pass --api to publish the service")
		}
		req.Public = flags.Public
		return nil
	}
	argv, err := parseAPICommand(flags.APICmd)
	if err != nil {
		return err
	}
	if err := lifecycle.ValidateAPI(flags.Running, flags.APIPort, argv); err != nil {
		return err
	}
	env, err := lifecycle.ResolveAPIEnv(flags.APIEnv, os.LookupEnv)
	if err != nil {
		return err
	}
	req.API = true
	req.APIPort = flags.APIPort
	req.APICmd = argv
	req.APIEnv = env
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

func createdMessage(ref, url string, req createRequest) string {
	if !req.API && !req.Public {
		return fmt.Sprintf("sandbox %s is ready and will stay running for 1 hour\n%s", ref, url)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "sandbox %s is ready", ref)
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
