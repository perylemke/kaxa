package cli

import (
	"context"
	"errors"
	"fmt"
	"os"

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
	var apiEnv []string
	var publicURL bool
	var template string

	cmd := &cobra.Command{
		Use:     "create [name]",
		Aliases: []string{"new"},
		Short:   "Create a sandbox",
		Long: `Create a sandbox on the selected provider.

--sync copies this Git repository into /home/sprite/workspace/<repo> on Sprites, /home/modal/workspace/<repo> on Modal, /home/user/workspace/<repo> on E2B, or /home/agent/workspace/<repo> on Docker, and installs Go, Rust, Python, Node, Ruby (Gemfile), or Clojure (deps.edn) dependencies found at the root. --dir selects that repository; without it, sync uses the current directory. A subdirectory still copies the whole Git repository. A failed copy or install deletes the sandbox.
--running keeps the sandbox up for one hour and opens a tmux session on its URL.
--api publishes one HTTP service on that URL after the copy. It requires --api-cmd and works with Sprites, E2B, and Docker. With --sync, the repository must be an HTTP API; a CLI project is rejected before the sandbox is created. The process must listen on 0.0.0.0 and on --api-port (8000 unless you change it). Together with --running, port 8080 stays with tmux and the URL forwards to the API. A failed publish deletes the sandbox. Create waits until the service is running. If it fails or does not become running within 90s, the sandbox is deleted.
--api-env sets a variable for that service: KEY=VALUE, or KEY to copy the value from your shell so a secret stays out of the command line. Repeat it for more variables. Values cannot contain commas. HOME, USER, PATH, and KAXA_ names are reserved.
--public makes the Sprites URL reachable without an account. A failed auth change leaves the sandbox in place.
--template starts a sandbox from a template. On E2B it is a template you built with the E2B CLI or SDK, for example one with more memory, and it must have the same user and home as the base template (user, /home/user); without it, kaxa uses KAXA_E2B_TEMPLATE, then base. On Docker it is an image reference that Docker can pull, and it must have the same user and home as the Docker agent images (agent, /home/agent); without it, kaxa uses KAXA_DOCKER_TEMPLATE, then the image of the shell agent.
Without a name, the form asks for the same choices. When API is yes, it asks for the command and the port.`,
		Example: `  kaxa create --sync my-sprite
  kaxa create --sync --dir ~/projects/api-python my-sprite
  kaxa create --sync --api --public --api-cmd .venv/bin/uvicorn,app.main:app,--host,0.0.0.0,--port,8000 my-sprite
  kaxa create --sync --api --api-env DATABASE_URL --api-cmd go,run,./cmd/api my-sprite
  kaxa create --provider e2b --template my-big-template --sync my-box
  kaxa create --provider docker --sync my-box
  kaxa create --provider docker --sync --api --api-cmd .venv/bin/uvicorn,app.main:app,--host,0.0.0.0,--port,8000 my-box`,
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
				APIEnv:  apiEnv,
				Public:  publicURL,

				Template: template,
			})
		},
	}
	cmd.Flags().BoolVar(&keepRunning, "running", false, "keep the sandbox running for 1 hour and open a tmux terminal in the browser")
	cmd.Flags().BoolVar(&syncWorkspace, "sync", false, "copy the local git repository into the sandbox workspace")
	cmd.Flags().StringVar(&syncDir, "dir", "", "git repository to copy with --sync (default: current directory)")
	cmd.Flags().BoolVar(&exposeAPI, "api", false, "publish an HTTP service on the sandbox URL (requires --api-cmd; Sprites, E2B and Docker)")
	cmd.Flags().StringVar(&apiCmd, "api-cmd", "", "comma-separated argv; first item is the binary (example: .venv/bin/uvicorn,app.main:app,--host,0.0.0.0,--port,8000)")
	cmd.Flags().IntVar(&apiPort, "api-port", defaultAPIPort, "port for --api; the process must listen on 0.0.0.0 (8080 is reserved with --running)")
	cmd.Flags().StringArrayVar(&apiEnv, "api-env", nil, "environment variable for the --api service: KEY=VALUE, or KEY to copy it from this shell; repeat for more (no commas in values)")
	cmd.Flags().StringVar(&template, "template", "", "template to start the sandbox from (E2B and Docker; default: KAXA_E2B_TEMPLATE or KAXA_DOCKER_TEMPLATE, then base)")
	cmd.Flags().BoolVar(&publicURL, "public", false, "make the Sprites URL reachable without authentication")
	markProviderFlags(cmd, "running", "api", "api-cmd", "api-port", "api-env", "template", "public")
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
		req := createRequest{Name: args[0], Running: flags.Running, Sync: flags.Sync, Template: flags.Template}
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

		Template: flags.Template,
	}
	if form.API {
		if req.APIEnv, err = lifecycle.ResolveAPIEnv(flags.APIEnv, os.LookupEnv); err != nil {
			return createRequest{}, err
		}
	}
	cfg.Provider = form.Provider
	cmd.SetContext(withConfig(cmd.Context(), cfg))
	return withRepo(req, flags)
}

func writeCreated(cmd *cobra.Command, p sandbox.Provider, id sandbox.ID, req createRequest) error {
	ref := sandboxLabel(p.Name(), id)
	msg := fmt.Sprintf("sandbox %s is ready", ref)
	if req.Running || req.API || req.Public {
		detail, err := p.Inspect(cmd.Context(), id)
		if err != nil {
			return err
		}
		msg = createdMessage(ref, p.Name(), detail, req)
		if req.Running && !req.API {
			_ = openBrowserFn(detail.URL)
		}
	}
	return WriteSuccess(cmd.OutOrStdout(), "created", msg)
}
