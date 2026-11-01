# Docker Sandboxes and the Docker personal access token

The `docker` provider runs sandboxes in Docker's cloud with the [Docker Sandboxes API](https://docs.docker.com/ai/sandboxes-api/). Each sandbox is an isolated Linux environment that Docker hosts. kaxa talks to the REST API directly, with a Docker ID and a personal access token.

The API is experimental, and cloud sandboxes need an active Docker Agentic Platform subscription. Cloud compute is billed to that subscription while a sandbox runs; kaxa always requests the `small` size, two CPUs and 4 GiB of memory. Model-provider charges are separate, and kaxa does not start an agent.

## Create a token

1. Sign in to [Docker Hub](https://hub.docker.com/) with the account that has the subscription.
2. Open Account settings, then Personal access tokens, and create a token. Select **Allow Cloud Sandboxes access**, which grants the `sandbox:use` scope. Registry permissions alone do not grant it.
3. Copy the token. Docker shows it once.

See [Authentication and authorization](https://docs.docker.com/ai/sandboxes-api/authentication/).

## Configure kaxa

```bash
export KAXA_DOCKER_ID=your-docker-id
export KAXA_DOCKER_PAT=dckr_pat_...
export KAXA_DEFAULT_PROVIDER=docker
```

`KAXA_DEFAULT_PROVIDER` is optional. You can pass `--provider docker` on each command instead. The provider is used only when both `KAXA_DOCKER_ID` and `KAXA_DOCKER_PAT` are set. kaxa reads them from the environment only, never from a flag or a file, and keeps neither in output or error messages.

Check that it works:

```bash
kaxa list --provider docker
```

Without `--provider`, `kaxa list` includes Docker when both variables are set. A rejected token prints `Docker rejected the credentials`; a missing subscription or scope prints `Docker denied access to Cloud Sandboxes`.

## What works on Docker

`create` (with `--sync`, `--template`, and `--api`), `list`, `exec`, `console`, `restart`, `sync`, `snapshot`, `destroy`, and the detail view in `list`. `create --api` publishes one HTTP service; see [create](create.md#api-on-docker). `snapshot` saves the disk and restores into a new sandbox; see [snapshot](snapshot.md#docker). `--running`, public URLs, `url`, and `upgrade` are not available; kaxa names the providers that have them.

## How sandboxes live

- kaxa exchanges the token for a short-lived access token that the API accepts, and renews it when it expires. For each command it asks Docker for a short-lived credential scoped to that sandbox and to running processes. Neither credential is stored.
- A sandbox is created from an image with no host mount. `create --sync` copies your repository into `/home/agent/workspace/<repo>` and installs its toolchain, the same way as on the other providers. Create waits until the sandbox runs and deletes it if it fails to start.
- Commands run as `agent`, never as root. `exec` and `console` on a sandbox created with `--sync` start in `/home/agent/workspace` with the installed toolchains on `PATH`.
- Docker stops a sandbox an hour after it was last renewed. kaxa asks for a stop, never a deletion, when that time ends, renews it on every command, and starts a stopped sandbox again when you run `exec` or `console`. Files stay; running processes do not. `restart` stops and starts it on purpose. `destroy` deletes the sandbox and everything in it.
- An account has quotas, by default 10 running and 50 stored sandboxes. When Docker refuses a create for that reason, kaxa says so and suggests destroying sandboxes you no longer need. See [limits](https://docs.docker.com/ai/sandboxes-api/limits/).
- kaxa marks the sandboxes it creates with labels. `list` shows only those, and kaxa never destroys or runs commands in a sandbox it did not create.
- Names are DNS labels: lowercase letters, digits, and `-`.
- Sandbox ports are not reachable from outside. Only `create --api` publishes one port, and Docker gives it a URL. Anyone who has the URL can call the service. Destroy the sandbox when you are done.

## Templates

By default kaxa starts sandboxes from `docker/sandbox-templates:shell`, the image of the shell agent. Set `KAXA_DOCKER_TEMPLATE` (or pass `--template` to `create`) to use another image that Docker can pull. It must keep the layout of the Docker agent images: user `agent`, home `/home/agent`, and `tail` on `PATH`. See [create](create.md#templates-on-docker).

Command manuals are in the [docs index](README.md).
