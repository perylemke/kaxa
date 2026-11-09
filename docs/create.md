# kaxa create

Create a sandbox. `new` is an alias.

```bash
kaxa create [name]
kaxa new [name]
```

Without a name, a wizard asks for the provider and name, then only the options that provider supports. The provider list includes only backends that have credentials set (`KAXA_*`); if none are configured, the command fails like `kaxa list`. When you pass `--provider` and that backend has credentials, the wizard lists only that provider, already selected, and shows the name field under it. Modal uses dropdowns for CPU and memory (with an **Other** line for custom values); Docker uses one **Size** dropdown for the fixed CPU and memory pairs that the Cloud Sandboxes API allows; Blaxel uses a memory dropdown with preset MiB sizes and lists the visible Hub images in a **Template** dropdown (Provider default uses `KAXA_BLAXEL_TEMPLATE` or `blaxel/base-image:latest`; a `--template` value outside that list is an extra option). If that list cannot be loaded, the field stays a text input. Daytona lists its active platform snapshots in a **Template** dropdown (Provider default leaves the Daytona image; a `--template` value outside that list is an extra option). If that list cannot be loaded, the field stays a text input. When the provider can block outbound network, the wizard also asks **Block all network**. A value you already passed with `--memory` that is not in the list appears as an extra option so the wizard matches the flag. When Sync is Yes, a Directories field opens on that same step (comma-separated paths; empty uses the current directory once). The next step asks whether to skip install. With API enabled it asks for the command, the port, and optional environment variables. The wizard uses the Dracula theme. In a terminal it occupies the alternate screen and returns a clean prompt when it finishes; only the result is printed in your scrollback. Ctrl+C cancels; the footer shows that shortcut. Flags you already passed stay as the initial answer; an empty line keeps that default.

Sprites, Modal, E2B, Docker, Daytona, and Blaxel implement create. `--running` and public URLs exist only for Sprites; the others reject them with a message that names the providers that have them. API publish works on Sprites, E2B, Docker, Daytona, and Blaxel. `--sync` works on all six; see [Sync](#sync).

Modal sandboxes use a Debian image, live for up to 24 hours, and stop after one idle hour. Names are DNS labels. See [Modal account and token](modal-account.md).

E2B sandboxes live for one hour after the last command and then pause, keeping their disk. Names are DNS labels. See [E2B account and API key](e2b-account.md).

Daytona sandboxes are persistent sandboxes created through the Daytona API with `KAXA_DAYTONA_API_KEY`. Names are DNS labels. Create waits until the sandbox runs and deletes it if it fails to start. Daytona bills the account while a sandbox runs. See [Daytona](daytona-account.md).

Blaxel sandboxes are created through the Blaxel API with `KAXA_BLAXEL_API_KEY` and `KAXA_BLAXEL_WORKSPACE`. Names are DNS labels of at most 49 characters. Create waits until the sandbox runs and deletes it if it fails to start. A deployed sandbox, including one in standby, is running: standby keeps files and processes. The writable layer is memory, so a large repository may need a higher `KAXA_BLAXEL_MEMORY`. Blaxel bills the workspace while a sandbox runs. See [Blaxel](blaxel-account.md).

Docker sandboxes run in Docker's cloud, created through the Docker Sandboxes API with `KAXA_DOCKER_ID` and `KAXA_DOCKER_PAT`. Names are DNS labels. Create waits until the sandbox runs and deletes it if it fails to start. Cloud compute is billed to your Docker subscription, and Docker stops a sandbox after an hour; kaxa renews that time on every command, and `exec` starts a stopped sandbox again. See [Docker Sandboxes](docker-account.md).

## Flags

| Flag | Meaning |
| --- | --- |
| `--running` | Keep the sandbox up for one hour, serve tmux on its URL, and open that URL in a browser |
| `--sync` | Copy the current Git repository into `/home/sprite/workspace/<repo>` (Sprites) `/home/modal/workspace/<repo>` (Modal), `/home/user/workspace/<repo>` (E2B), `/home/agent/workspace/<repo>` (Docker), `/home/daytona/workspace/<repo>` (Daytona), or `/root/workspace/<repo>` (Blaxel) |
| `--dir` | Comma-separated Git repositories to copy. Requires `--sync`. Default: the current directory once |
| `--api` | Publish one HTTP service on the Sprites URL, on the E2B sandbox URL for the API port, on the Docker URL that Docker gives the published port, on the public preview URL of the API port on Daytona, or on a public preview named `kaxa-api` on Blaxel. Requires `--api-cmd` |
| `--api-cmd` | Comma-separated argv. The first item is the binary |
| `--api-port` | Port the process listens on. Default `8000`. `8080` is reserved when `--running` is also set |
| `--api-env` | Environment variable for the service: `KEY=VALUE`, or `KEY` to copy the value from your shell. Repeat for more. Requires `--api` |
| `--public` | Make the Sprites URL reachable without an account. Refused with `--running` unless `--api` is set |
| `--template` | E2B, Docker, Daytona, and Blaxel: the template to start the sandbox from. Default: `KAXA_E2B_TEMPLATE`, `KAXA_DOCKER_TEMPLATE`, `KAXA_DAYTONA_TEMPLATE`, or `KAXA_BLAXEL_TEMPLATE`, then the base image. See [Templates on E2B](#templates-on-e2b) and [Templates on Docker](#templates-on-docker) |
| `--cpu` | CPU cores for the sandbox (Modal and Docker; fractional values are allowed on Modal; integer-only on Docker). Omit for the provider default. Unsupported providers reject the flag |
| `--memory` | Memory in MiB (Modal, Docker, Blaxel). Overrides `KAXA_BLAXEL_MEMORY` for that Blaxel create only. Omit for the provider default |
| `--block-all-network-access` | Block all outbound network access from the sandbox. Daytona only. The default is off |

`--api-cmd`, `--api-env`, and a changed `--api-port` require `--api`. `--dir` requires `--sync`.

With `--block-all-network-access`, the sandbox cannot reach the network, so a toolchain download and `go mod download` fail. `--no-install`, or a create without `--sync`, still works. The public preview from `--api` is unchanged: the block is outbound only.

`list_providers` reports `cpu`, `memory`, and `network_block` in each provider's capabilities when the backend can honor `--cpu`, `--memory`, or `--block-all-network-access`. The MCP `create_sandbox` tool accepts optional `cpu`, `memory_mib`, and `block_all_network_access` with the same rules.

## Sync

`--sync` copies the Git repository that contains the start directory, including `.git` and excluding ignored files. A subdirectory still copies the whole repository. The root selects Go, Rust, Python, Node, Ruby, and Clojure; missing tools are installed, then project dependencies. A `Gemfile` runs `bundle install` (`--frozen` with a `Gemfile.lock`), and installs Ruby when `bundle` is missing. A `deps.edn` runs `clojure -P`, and installs the Clojure CLI, plus a JDK when `java` is missing. `project.clj` alone is not detected. A failed copy or install deletes the sandbox.

The copy leaves out credentials: untracked files such as `.env` or `id_rsa` stop the sync with an error, and `.git/config` is stripped of URL credentials and `http.extraHeader`. See [sync](sync.md#limits).

`--dir` lists one or more paths separated by commas (no space-only segments). Each path must exist and be a directory inside a Git repository. A relative path is resolved from the current directory. Listing the same repository twice, or two repos whose directory name would collide in `workspace/<repo>`, fails before create.

```bash
kaxa create --sync my-sprite
kaxa create --sync --dir ~/projects/api-python my-sprite
```

Every sandbox has a workspace directory (`$HOME/workspace`), created at create time, so `kaxa sync` works on a sandbox created without `--sync`. Modal is the exception: only a sandbox created with `--sync` has one.

`--no-install`, together with `--sync`, copies the repository and skips the toolchain and the dependency commands. kaxa prints the dependency commands to run them by hand. The image has to provide the tools those commands need.

`exec` and `console` on a sandbox created with `--sync` start in the workspace: `/home/sprite/workspace` on Sprites, `/home/modal/workspace` on Modal, `/home/user/workspace` on E2B, `/home/agent/workspace` on Docker, `/home/daytona/workspace` on Daytona, `/root/workspace` on Blaxel. Without `--sync` they start in the home directory, with the toolchain `PATH` set.

### Sync on Modal

```bash
kaxa create --provider modal --sync my-box
```

A Modal sandbox created with `--sync` uses a larger image than a plain one: Debian with `ca-certificates`, `curl`, `git`, and `build-essential`, plus an unprivileged user `modal` whose home is `/home/modal`. The repository lands in `/home/modal/workspace/<repo>`, owned by `modal`. Toolchains (Go, Rust, Python `uv`, Node, Ruby, Clojure) are installed under `/home/modal/.local` and `/home/modal/.cargo` and put on the `PATH` of the shell, the same way as on Sprites. Downloads are checked against pinned SHA-256 hashes.

`exec` and `console` on a synced Modal sandbox run as `modal`, not root, starting in `/home/modal/workspace`. A Modal sandbox created without `--sync` is unchanged: root, starting in the image root.

Modal builds the sync image the first time you use `--sync` and caches it, so that first create can take a few minutes. Later creates start as fast as a plain sandbox, plus the copy and the install.

### Sync on E2B

```bash
kaxa create --provider e2b --sync my-box
```

An E2B sandbox uses E2B's `base` template, which already has `curl`, `git`, and build tools. The repository lands in `/home/user/workspace/<repo>`, owned by `user`. Toolchains are installed under `/home/user/.local` and `/home/user/.cargo` and put on the `PATH` of `exec` and `console`, the same way as on Modal. Downloads are checked against pinned SHA-256 hashes.

The E2B `base` template has 512 MiB of memory, and E2B sets memory per template, not per sandbox. Compiling a large package there is killed by the kernel (`signal: killed`). So the install step downloads dependencies instead of building them: Go always runs `go mod download`, and on the `base` template Rust runs `cargo fetch` instead of `cargo build`. The first build runs later, with `exec`; if it is killed too, use a template with more memory (see below). With `--template`, the Rust install step runs the normal build.

### Templates on E2B

```bash
kaxa create --provider e2b --template my-big-template --sync my-box
export KAXA_E2B_TEMPLATE=my-big-template   # default for every E2B create
```

A template is a sandbox image you build outside kaxa, with the [E2B CLI or SDK](https://e2b.dev/docs/template/quickstart), where you pick the CPU, the memory, and the tools already installed. kaxa only starts sandboxes from it. The `--template` flag wins over `KAXA_E2B_TEMPLATE`, and `base` (or neither) means E2B's default.

kaxa expects the same layout as `base`: user `user`, home `/home/user`, workspace `/home/user/workspace`. A template made with `fromBaseImage()` has it. Another user breaks `exec`, `console`, `--sync`, and `--api`, and kaxa does not detect that. A name that does not exist fails with `template "<name>" not found`; run `e2b template list` to see the names. Other providers reject `--template`, and `list_providers` reports `template` as a capability only for E2B.

`exec` and `console` on an E2B sandbox created with `--sync` start in `/home/user/workspace`. Without `--sync` they start in `/home/user`. The sandbox pauses an hour after the last command; the next `exec`, `console`, or `sync` resumes it.

## API

`--api` starts one service named `kaxa-api` after the copy. `--api-cmd` is comma-separated argv. The process must listen on `0.0.0.0` and on `--api-port`.

With `--sync`, every listed repository must look like an HTTP API. A CLI project is rejected before the sandbox is created. `--api` without `--sync` does not inspect a local project. The API service and reported `WorkDir` use the first repository.

Create waits until `sprite-env services get kaxa-api` reports `running`, for up to 90 seconds, because Rails and the JVM start slowly. `failed` stops immediately. If the service does not become `running`, the sandbox is deleted.

With `--running` and `--api` together, tmux still starts and the URL forwards to the API. Port `8080` stays with tmux.

```bash
kaxa create --sync --api --public \
  --api-cmd .venv/bin/uvicorn,app.main:app,--host,0.0.0.0,--port,8000 \
  my-sprite
```

### Sync on Docker

```bash
kaxa create --provider docker --sync my-box
```

kaxa creates the sandbox through the Docker Sandboxes API and waits until it runs, then streams the repository, as a gzip tar, to `tar` inside it over the process API. The repository lands in `/home/agent/workspace/<repo>`, owned by `agent`. The same checks as on the other providers apply: credentials in `.git/config` are removed and untracked secret files stop the copy. Toolchains are installed under `/home/agent/.local` and `/home/agent/.cargo` and put on the `PATH` of `exec` and `console`. Downloads are checked against pinned SHA-256 hashes. Docker's network policy for the sandbox must allow the download hosts. See [Docker Sandboxes](docker-account.md).

### Sync on Daytona

```bash
kaxa create --provider daytona --sync my-box
```

kaxa creates the sandbox through the Daytona API and waits until it runs, then uploads the repository, as a gzip tar, and unpacks it with `tar` inside the sandbox. The repository lands in `/home/daytona/workspace/<repo>`, owned by `daytona`. The same checks as on the other providers apply: credentials in `.git/config` are removed and untracked secret files stop the copy. Toolchains are installed under `/home/daytona/.local` and `/home/daytona/.cargo` and put on the `PATH` of `exec` and `console`. Downloads are checked against pinned SHA-256 hashes. See [Daytona](daytona-account.md).

### Sync on Blaxel

```bash
kaxa create --provider blaxel --sync my-box
```

kaxa creates the sandbox through the Blaxel API and waits until it runs, then uploads the repository, as a gzip tar, and unpacks it with `tar` inside the sandbox. The repository lands in `/root/workspace/<repo>`, owned by `root`. The writable layer is a tmpfs inside `KAXA_BLAXEL_MEMORY` (4096 MB unless you set it), so a large repository may need more memory. The same checks as on the other providers apply: credentials in `.git/config` are removed and untracked secret files stop the copy. Toolchains are installed under `/root/.local` and `/root/.cargo` and put on the `PATH` of `exec`. Downloads are checked against pinned SHA-256 hashes. See [Blaxel](blaxel-account.md).

### Templates on Docker

```bash
kaxa create --provider docker --template ghcr.io/me/img:v1 --sync my-box
export KAXA_DOCKER_TEMPLATE=ghcr.io/me/img:v1   # default for every Docker create
```

A template is an image reference that Docker can pull. The `--template` flag wins over `KAXA_DOCKER_TEMPLATE`; without either, kaxa uses `docker/sandbox-templates:shell`, the image of the shell agent. kaxa expects the layout of the Docker agent images: user `agent`, home `/home/agent`, workspace `/home/agent/workspace`, and `tail` on `PATH`; kaxa starts the sandbox with `tail -f /dev/null` so it stays up. Another user breaks `exec`, `console`, and `--sync`, and kaxa does not detect that.

### Templates on Daytona

```bash
kaxa create --provider daytona --template my-image --sync my-box
export KAXA_DAYTONA_TEMPLATE=my-image   # default for every Daytona create
```

A template is the name of a Daytona snapshot. The `--template` flag wins over `KAXA_DAYTONA_TEMPLATE`; without either, kaxa uses the Daytona default image. In the wizard the Template field is a dropdown of the active platform snapshots (`general` and active). Provider default leaves the name empty so Daytona picks its image. A name you already passed that is not in that list is added as an extra option. If the snapshot list cannot be loaded, the field is a text input. kaxa expects its layout: user `daytona`, home `/home/daytona`, workspace `/home/daytona/workspace`. Another user breaks `exec`, `console`, and `--sync`, and kaxa does not detect that.

### Templates on Blaxel

```bash
kaxa create --provider blaxel --template blaxel/py-app:latest --sync my-box
export KAXA_BLAXEL_TEMPLATE=blaxel/py-app:latest   # default for every Blaxel create
```

A template is an image reference: letters, digits, and `. _ : / -`. The `--template` flag wins over `KAXA_BLAXEL_TEMPLATE`; without either, kaxa uses `blaxel/base-image:latest`. In the wizard the Template field is a dropdown of the visible Hub images (not hidden and not coming soon). Provider default leaves the name empty so `KAXA_BLAXEL_TEMPLATE` or `blaxel/base-image:latest` is used. A name you already passed that is not in that list is added as an extra option. If the Hub list cannot be loaded, the field is a text input. kaxa expects that image's layout: user `root`, home `/root`, workspace `/root/workspace`. Another user breaks `exec` and `--sync`, and kaxa does not detect that.

### API on E2B

On E2B, `--api` runs the service as `user` in the repository directory (or `/home/user` without `--sync`) and waits for the port to accept connections, for up to 90 seconds. If the process exits first, kaxa deletes the sandbox and shows its last output. The service log is not kept.

E2B sandboxes are private. `--api` is what opens the API port to the internet, at `https://<port>-<sandbox id>.e2b.app`, and E2B fixes that when the sandbox is created. `list` shows the URL. Anyone who has the URL can call the service. `--public` does not apply, because the URL is already open on that one port. `restart` stops the service; start it again with `exec`, or create the sandbox again.

### API on Docker

```bash
kaxa create --provider docker --sync --api \
  --api-cmd .venv/bin/uvicorn,app.main:app,--host,0.0.0.0,--port,8000 my-box
kaxa list --provider docker   # the detail view shows the URL
```

On Docker, `--api` runs the service as `agent` in the repository directory (or `/home/agent` without `--sync`), waits for the port to accept connections, for up to 90 seconds, and then asks Docker to publish that port. If the process exits first, or Docker refuses to publish the port, kaxa deletes the sandbox and shows the reason. The service log is not kept.

Docker chooses the URL, and kaxa does not change who may open it, so treat anyone who has the URL as able to call the service. The token needs permission to publish sandbox ports; a refusal prints `Docker denied publishing the port`. The service ends when Docker stops the sandbox, an hour after the last command, and `restart` stops it too. `exec` starts the sandbox again but not the service, so start it again with `exec`, or create the sandbox again. `--public` and `--running` do not apply.

### API on Daytona

```bash
kaxa create --provider daytona --sync --api \
  --api-cmd .venv/bin/uvicorn,app.main:app,--host,0.0.0.0,--port,8000 my-box
kaxa list --provider daytona   # the detail view shows the URL
```

On Daytona, `--api` runs the service as `daytona` in the repository directory (or `/home/daytona` without `--sync`), waits for the port to accept connections, for up to 90 seconds, and then makes that port public. If the process exits first, or Daytona refuses, kaxa deletes the sandbox and shows the reason. The service log is not kept.

The preview URL is public, so treat anyone who has it as able to call the service. The service ends when Daytona stops the sandbox, and `restart` stops it too. `exec` starts the sandbox again but not the service, so start it again with `exec`, or create the sandbox again. `--public` and `--running` do not apply.

### API on Blaxel

```bash
kaxa create --provider blaxel --sync --api \
  --api-cmd .venv/bin/uvicorn,app.main:app,--host,0.0.0.0,--port,8000 my-box
kaxa list --provider blaxel   # the detail view shows the URL
```

On Blaxel, `--api` runs the service as `root` in the repository directory (or `/root` without `--sync`), waits for the port to accept connections, for up to 90 seconds, and then creates a public preview named `kaxa-api`. If the process exits first, kaxa deletes the sandbox and shows the reason. The service log is not kept.

Ports 80, 443, and 8080 are reserved by the sandbox. The process must listen on `0.0.0.0` and on `--api-port`. Blaxel also accepts `$HOST` or `::`. The preview URL is public, so treat anyone who has it as able to call the service. There is no private preview. Standby keeps the process. `--public` and `--running` do not apply, and there is no `restart`.

## API environment

`--api-env` adds variables to the `kaxa-api` process, after the toolchain `PATH`. Use `KEY=VALUE`, or `KEY` to copy the value from the shell that runs kaxa. The second form keeps passwords out of the command line and shell history. A missing `KEY` fails before the sandbox is created.

```bash
export DATABASE_URL=postgres://app:secret@db.example.com:5432/app
kaxa create --sync --api --api-env DATABASE_URL --api-env REDIS_URL \
  --api-cmd go,run,./cmd/api my-sprite
```

Values cannot contain commas or control characters, because the provider joins variables with commas. `HOME`, `USER`, `PATH`, and any name starting with `KAXA_` are rejected, so the provider token never reaches the sandbox. Errors name the variable, never the value.

## Public URL

`--public` sets the Sprites URL auth to `public` after the service is running. `--running` without `--api` serves a tmux terminal on the URL that has no login, so `--public` is refused for it. A failed auth change leaves the sandbox in place. Change it later with [kaxa url](url.md).

## What gets deleted

A failed sync, dependency install, or API publish deletes the sandbox. A failed `--public` update does not.
