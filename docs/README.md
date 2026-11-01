# kaxa documentation

kaxa is a CLI for sandbox providers. Sprites is fully implemented. Modal supports create (including --sync), list, exec, console, snapshot, and destroy. E2B supports create (including --sync and --api), list, exec, console, restart, sync, and destroy. Docker supports create (including --sync, --template, and --api), list, exec, console, restart, sync, snapshot, and destroy through the Docker Sandboxes API. Features that only exist for Sprites are rejected for Modal, E2B, and Docker with a message that names the providers that have them. Create, destroy, and the list URL auth prompt use the Dracula theme.

Install kaxa with Homebrew, a curl download, or from source: [install](install.md).

Set up an account and token in [Sprites account and token](sprites-account.md) [Modal account and token](modal-account.md), [E2B account and API key](e2b-account.md), or [Docker Sandboxes](docker-account.md).

## Configuration

Every command except `version` and `--help` loads configuration before it runs.

| Source | What it sets |
| --- | --- |
| `--provider` | `sprites`, `modal`, `e2b`, or `docker` for this invocation. `list` shows every provider when it is omitted |
| `KAXA_DEFAULT_PROVIDER` | Provider when `--provider` is omitted, except for `list` |
| `KAXA_SPRITES_TOKEN` | Sprites API token |
| `KAXA_MODAL_TOKEN_ID` | Modal token ID (`ak-...`) |
| `KAXA_MODAL_TOKEN_SECRET` | Modal token secret (`as-...`) |
| `KAXA_E2B_API_KEY` | E2B API key (`e2b_...`) |
| `KAXA_E2B_TEMPLATE` | E2B template for new sandboxes. Optional; default `base` |
| `KAXA_DOCKER_ID` | Docker ID |
| `KAXA_DOCKER_PAT` | Docker personal access token, created with Allow Cloud Sandboxes access |
| `KAXA_DOCKER_TEMPLATE` | Docker image for new sandboxes. Optional; default the shell agent image |

`--provider` wins over `KAXA_DEFAULT_PROVIDER`. Tokens are never read from a flag, from the Sprites CLI config file, from `~/.modal.toml`, or from `E2B_API_KEY`.

## Commands

| Command | Manual |
| --- | --- |
| `kaxa version` | [version](version.md) |
| `kaxa create` (`new`) | [create](create.md) |
| `kaxa list` (`ls`) | [list](list.md) |
| `kaxa exec` | [exec](exec.md) |
| `kaxa console` (`ssh`) | [console](console.md) |
| `kaxa sync` | [sync](sync.md) |
| `kaxa destroy` (`rm`) | [destroy](destroy.md) |
| `kaxa restart` | [restart](restart.md) |
| `kaxa upgrade` | [upgrade](upgrade.md) |
| `kaxa snapshot` | [snapshot](snapshot.md) |
| `kaxa url` | [url](url.md) |
| `kaxa mcp` | [mcp](agent.md) |
