package cli

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/spf13/cobra"

	"github.com/perylemke/kaxa/internal/lifecycle"
	"github.com/perylemke/kaxa/internal/registry"
	"github.com/perylemke/kaxa/internal/sandbox"
)

func newCreateCmd(reg *registry.Registry) *cobra.Command {
	var keepRunning bool
	var syncWorkspace bool
	var noInstall bool
	var syncDir string
	var exposeAPI bool
	var apiCmd string
	var apiPort int
	var apiEnv []string
	var publicURL bool
	var template string
	var cpu float64
	var memoryMiB int

	cmd := &cobra.Command{
		Use:     "create [name]",
		Aliases: []string{"new"},
		Short:   "Create a sandbox",
		Long: `Create a sandbox on the selected provider.

--sync copies one or more Git repositories into workspace/<repo> on the provider and installs Go, Rust, Python, Node, Ruby (Gemfile), or Clojure (deps.edn) dependencies found at each root. --dir is comma-separated paths; without it, sync uses the current directory once. A subdirectory still copies the whole Git repository. Duplicates or two repos with the same directory name are rejected. A failed copy or install deletes the sandbox.
--running keeps the sandbox up for one hour and opens a tmux session on its URL.
--api publishes one HTTP service on that URL after the copy. It requires --api-cmd and works with Sprites, E2B, Docker, and Daytona. With --sync, the repository must be an HTTP API; a CLI project is rejected before the sandbox is created. The process must listen on 0.0.0.0 and on --api-port (8000 unless you change it). Together with --running, port 8080 stays with tmux and the URL forwards to the API. A failed publish deletes the sandbox. Create waits until the service is running. If it fails or does not become running within 90s, the sandbox is deleted.
--api-env sets a variable for that service: KEY=VALUE, or KEY to copy the value from your shell so a secret stays out of the command line. Repeat it for more variables. Values cannot contain commas. HOME, USER, PATH, and KAXA_ names are reserved.
--public makes the Sprites URL reachable without an account. A failed auth change leaves the sandbox in place.
--template starts a sandbox from a template. On E2B it is a template you built with the E2B CLI or SDK, for example one with more memory, and it must have the same user and home as the base template (user, /home/user); without it, kaxa uses KAXA_E2B_TEMPLATE, then base. On Docker it is an image reference that Docker can pull, and it must have the same user and home as the Docker agent images (agent, /home/agent); without it, kaxa uses KAXA_DOCKER_TEMPLATE, then the image of the shell agent. On Daytona it is an image (or snapshot name) that Daytona can start, and it must have the same user and home as the Daytona default image (daytona, /home/daytona); without it, kaxa uses KAXA_DAYTONA_TEMPLATE, then the Daytona default.
--cpu and --memory request compute for the sandbox (empty means the provider default). Modal accepts fractional CPU and memory in MiB; Docker accepts both with integer CPU only; Blaxel accepts memory only. Other providers reject these flags.
Without a name, a wizard asks for the provider and name, then only the options that provider supports, then sync and API details when those are enabled. In a terminal it uses the alternate screen so the form does not mix with your scrollback; only the result is printed when it finishes.`,
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
				Running:   keepRunning,
				Sync:      syncWorkspace,
				NoInstall: noInstall,
				Dir:       syncDir,
				DirSet:    cmd.Flags().Changed("dir"),
				API:       exposeAPI,
				APICmd:    apiCmd,
				APIPort:   apiPort,
				APIEnv:    apiEnv,
				Public:    publicURL,

				Template:  template,
				CPU:       cpu,
				MemoryMiB: memoryMiB,
			})
		},
	}
	cmd.Flags().BoolVar(&keepRunning, "running", false, "keep the sandbox running for 1 hour and open a tmux terminal in the browser")
	cmd.Flags().BoolVar(&syncWorkspace, "sync", false, "copy the local git repository into the sandbox workspace")
	cmd.Flags().BoolVar(&noInstall, "no-install", false, "with --sync, copy the repository and skip the toolchain and dependency installs")
	cmd.Flags().StringVar(&syncDir, "dir", "", "comma-separated git repo paths to copy with --sync (default: current directory once)")
	cmd.Flags().BoolVar(&exposeAPI, "api", false, "publish an HTTP service on the sandbox URL (requires --api-cmd; Sprites, E2B, Docker and Daytona)")
	cmd.Flags().StringVar(&apiCmd, "api-cmd", "", "comma-separated argv; first item is the binary (example: .venv/bin/uvicorn,app.main:app,--host,0.0.0.0,--port,8000)")
	cmd.Flags().IntVar(&apiPort, "api-port", defaultAPIPort, "port for --api; the process must listen on 0.0.0.0 (8080 is reserved with --running)")
	cmd.Flags().StringArrayVar(&apiEnv, "api-env", nil, "environment variable for the --api service: KEY=VALUE, or KEY to copy it from this shell; repeat for more (no commas in values)")
	cmd.Flags().StringVar(&template, "template", "", "template to start the sandbox from (E2B, Docker and Daytona; default: KAXA_E2B_TEMPLATE, KAXA_DOCKER_TEMPLATE or KAXA_DAYTONA_TEMPLATE, then base)")
	cmd.Flags().BoolVar(&publicURL, "public", false, "make the Sprites URL reachable without authentication")
	cmd.Flags().Float64Var(&cpu, "cpu", 0, "CPU cores for the sandbox (Modal and Docker; fractional on Modal; omit for the provider default)")
	cmd.Flags().IntVar(&memoryMiB, "memory", 0, "memory for the sandbox in MiB (Modal, Docker, Blaxel; omit for the provider default)")
	markProviderFlags(cmd, "running", "api", "api-cmd", "api-port", "api-env", "template", "public", "cpu", "memory")
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
		if err := validateCreateComputeFlags(flags.CPU, flags.MemoryMiB); err != nil {
			return createRequest{}, err
		}
		req := createRequest{
			Name: args[0], Running: flags.Running, Sync: flags.Sync, Template: flags.Template, NoInstall: flags.NoInstall,
			CPU: flags.CPU, MemoryMiB: flags.MemoryMiB,
		}
		if err := applyAPIFlags(&req, flags, portSet); err != nil {
			return createRequest{}, err
		}
		return withRepo(req, flags)
	}
	cfg, err := configFromContext(cmd.Context())
	if err != nil {
		return createRequest{}, err
	}
	_, providerNames, err := lifecycle.ListTargets(reg, cfg)
	if err != nil {
		return createRequest{}, err
	}
	if cmd.Flags().Changed("provider") && !slices.Contains(providerNames, cfg.Provider) {
		if _, err := lifecycle.ResolveProvider(reg, cfg, cfg.Provider); err != nil {
			return createRequest{}, err
		}
	}
	defaultProvider := cfg.Provider
	if !slices.Contains(providerNames, defaultProvider) {
		defaultProvider = providerNames[0]
		cfg.Provider = defaultProvider
		cmd.SetContext(withConfig(cmd.Context(), cfg))
	}
	wizardProviders := providerNames
	if cmd.Flags().Changed("provider") && slices.Contains(providerNames, cfg.Provider) {
		wizardProviders = []string{cfg.Provider}
	}
	form, err := collectCreateForm(cmd.Context(), cmd.InOrStdin(), cmd.ErrOrStderr(), wizardProviders, defaultProvider, reg.Get, flags, portSet)
	if err != nil {
		return createRequest{}, err
	}
	req := createRequest{
		Name:      form.Name,
		Running:   form.Running,
		Sync:      form.Sync,
		API:       form.API,
		APIPort:   form.APIPort,
		APICmd:    form.APICmd,
		APIEnv:    form.APIEnv,
		Public:    form.Public,
		Template:  form.Template,
		NoInstall: form.NoInstall,
		CPU:       form.CPU,
		MemoryMiB: form.MemoryMiB,
	}
	cfg.Provider = form.Provider
	cmd.SetContext(withConfig(cmd.Context(), cfg))
	merged := flags
	merged.Dir = form.Dir
	merged.DirSet = form.DirSet
	return withRepo(req, merged)
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
