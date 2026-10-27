package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/perylemke/kaxa/internal/lifecycle"
	"github.com/perylemke/kaxa/internal/registry"
	"github.com/perylemke/kaxa/internal/sandbox"
)

func newCreateCmd(reg *registry.Registry) *cobra.Command {
	var keepRunning bool
	var syncWorkspace bool
	var syncDir string
	var exposeAPI bool
	var apiCmd string
	var apiPort int
	var publicURL bool

	cmd := &cobra.Command{
		Use:     "create [name]",
		Aliases: []string{"new"},
		Short:   "Create a sandbox",
		Long: `Create a sandbox on the selected provider.

--sync copies this Git repository into /home/sprite/workspace/<repo> and installs Go, Rust, Python, or Node dependencies found at the root. --dir selects that repository; without it, sync uses the current directory. A subdirectory still copies the whole Git repository. A failed copy or install deletes the sandbox.
--running keeps the sandbox up for one hour and opens a tmux session on its URL.
--api publishes one HTTP service on that URL after the copy. It requires --api-cmd and works only with Sprites. With --sync, the repository must be an HTTP API; a CLI project is rejected before the sandbox is created. The process must listen on 0.0.0.0 and on --api-port (8000 unless you change it). Together with --running, port 8080 stays with tmux and the URL forwards to the API. A failed publish deletes the sandbox. Create waits until the service is running. If it fails or does not become running within 30s, the sandbox is deleted.
--public makes the Sprites URL reachable without an account. A failed auth change leaves the sandbox in place.
Without a name, the form asks for the same choices. When API is yes, it asks for the command and the port.`,
		Example: `  kaxa create --sync my-sprite
  kaxa create --sync --dir ~/projects/api-python my-sprite
  kaxa create --sync --api --public --api-cmd .venv/bin/uvicorn,app.main:app,--host,0.0.0.0,--port,8000 my-sprite`,
		Args: atMostOneName("create"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreate(cmd, reg, args, createFlags{
				Running: keepRunning,
				Sync:    syncWorkspace,
				Dir:     syncDir,
				DirSet:  cmd.Flags().Changed("dir"),
				API:     exposeAPI,
				APICmd:  apiCmd,
				APIPort: apiPort,
				Public:  publicURL,
			})
		},
	}
	cmd.Flags().BoolVar(&keepRunning, "running", false, "keep the sandbox running for 1 hour and open a tmux terminal in the browser")
	cmd.Flags().BoolVar(&syncWorkspace, "sync", false, "copy the local git repository into the sandbox workspace")
	cmd.Flags().StringVar(&syncDir, "dir", "", "git repository to copy with --sync (default: current directory)")
	cmd.Flags().BoolVar(&exposeAPI, "api", false, "publish an HTTP service on the Sprites URL (requires --api-cmd)")
	cmd.Flags().StringVar(&apiCmd, "api-cmd", "", "comma-separated argv; first item is the binary (example: .venv/bin/uvicorn,app.main:app,--host,0.0.0.0,--port,8000)")
	cmd.Flags().IntVar(&apiPort, "api-port", defaultAPIPort, "port for --api; the process must listen on 0.0.0.0 (8080 is reserved with --running)")
	cmd.Flags().BoolVar(&publicURL, "public", false, "make the Sprites URL reachable without authentication")
	return cmd
}

func runCreate(cmd *cobra.Command, reg *registry.Registry, args []string, flags createFlags) error {
	req, err := resolveCreateRequest(cmd, reg, args, flags, cmd.Flags().Changed("api-port"))
	if err != nil {
		return cancelledOrErr(cmd, err)
	}
	p, err := providerFromCmd(cmd, reg)
	if err != nil {
		return err
	}
	errOut := cmd.ErrOrStderr()
	res, err := lifecycle.Create(cmd.Context(), p, req, func(ctx context.Context, label string, task func(context.Context) error) error {
		return runTaskWithSpinner(ctx, errOut, label, task)
	})
	if err != nil {
		return err
	}
	return writeCreated(cmd, p, res.ID, req)
}

func cancelledOrErr(cmd *cobra.Command, err error) error {
	if !errors.Is(err, errCreateCancelled) {
		return err
	}
	_, err = fmt.Fprintln(cmd.OutOrStdout(), "create cancelled")
	return err
}

func resolveCreateRequest(cmd *cobra.Command, reg *registry.Registry, args []string, flags createFlags, portSet bool) (createRequest, error) {
	if len(args) == 1 {
		req := createRequest{Name: args[0], Running: flags.Running, Sync: flags.Sync}
		if err := applyAPIFlags(&req, flags, portSet); err != nil {
			return createRequest{}, err
		}
		return withRepo(req, flags)
	}
	cfg, err := configFromContext(cmd.Context())
	if err != nil {
		return createRequest{}, err
	}
	form, err := collectCreateForm(cmd.InOrStdin(), cmd.ErrOrStderr(), reg.Names(), cfg.Provider, flags)
	if err != nil {
		return createRequest{}, err
	}
	req := createRequest{
		Name:    form.Name,
		Running: form.Running,
		Sync:    form.Sync,
		API:     form.API,
		APIPort: form.APIPort,
		APICmd:  form.APICmd,
		Public:  form.Public,
	}
	cfg.Provider = form.Provider
	cmd.SetContext(withConfig(cmd.Context(), cfg))
	return withRepo(req, flags)
}

func writeCreated(cmd *cobra.Command, p sandbox.Provider, id sandbox.ID, req createRequest) error {
	msg := fmt.Sprintf("sandbox %s is ready", id)
	if req.Running || req.API || req.Public {
		detail, err := p.Inspect(cmd.Context(), id)
		if err != nil {
			return err
		}
		msg = createdMessage(id, detail.URL, req)
		if req.Running && !req.API {
			_ = openBrowserFn(detail.URL)
		}
	}
	return WriteSuccess(cmd.OutOrStdout(), "created", msg)
}
