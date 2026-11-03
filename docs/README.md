# kaxa documentation

kaxa is a CLI for sandbox providers. Sprites is fully implemented. Modal supports create (including --sync), list, exec, console, snapshot, and destroy. E2B supports create (including --sync and --api), list, exec, console, restart, sync, and destroy. Docker supports create (including --sync, --template, and --api), list, exec, console, restart, sync, snapshot, and destroy through the Docker Sandboxes API. Daytona supports create (including --sync, --template, and --api), list, exec, console, restart, sync, snapshot, and destroy through the Daytona API. Blaxel supports create (including --sync, --template, and --api), list, exec, console, sync, and destroy through the Blaxel API. Features that only exist for Sprites are rejected for Modal, E2B, Docker, Daytona, and Blaxel with a message that names the providers that have them. Create, destroy, and the list URL auth prompt use the Dracula theme.

Install kaxa with Homebrew, a curl download, or from source: [install](install.md).

Set up an account and token in [Sprites account and token](sprites-account.md) [Modal account and token](modal-account.md), [E2B account and API key](e2b-account.md), [Docker Sandboxes](docker-account.md), [Daytona](daytona-account.md), or [Blaxel](blaxel-account.md).

## Configuration

Every command except `version` and `--help` loads configuration before it runs.

| Source | What it sets |
| --- | --- |
| `--provider` | `sprites`, `modal`, `e2b`, `docker`, `daytona`, or `blaxel` for this invocation. `list` shows every provider when it is omitted |
| `KAXA_DEFAULT_PROVIDER` | Provider when `--provider` is omitted, except for `list` |
| `KAXA_SPRITES_TOKEN` | Sprites API token |
| `KAXA_MODAL_TOKEN_ID` | Modal token ID (`ak-...`) |
| `KAXA_MODAL_TOKEN_SECRET` | Modal token secret (`as-...`) |
| `KAXA_E2B_API_KEY` | E2B API key (`e2b_...`) |
| `KAXA_E2B_TEMPLATE` | E2B template for new sandboxes. Optional; default `base` |
| `KAXA_DOCKER_ID` | Docker ID |
| `KAXA_DOCKER_PAT` | Docker personal access token, created with Allow Cloud Sandboxes access |
| `KAXA_DOCKER_TEMPLATE` | Docker image for new sandboxes. Optional; default the shell agent image |
| `KAXA_DAYTONA_API_KEY` | Daytona API key |
| `KAXA_DAYTONA_TEMPLATE` | Daytona snapshot for new sandboxes. Optional; default the Daytona default snapshot |
| `KAXA_BLAXEL_API_KEY` | Blaxel API key |
| `KAXA_BLAXEL_WORKSPACE` | Blaxel workspace name |
| `KAXA_BLAXEL_TEMPLATE` | Blaxel image for new sandboxes. Optional; default `blaxel/base-image:latest` |
| `KAXA_BLAXEL_REGION` | Blaxel region. Optional; default `us-pdx-1` |
| `KAXA_BLAXEL_MEMORY` | Blaxel memory in megabytes. Optional; default `4096` |

`--provider` wins over `KAXA_DEFAULT_PROVIDER`. Tokens are never read from a flag, from the Sprites CLI config file, from `~/.modal.toml`, from `E2B_API_KEY`, or from `BL_API_KEY`, `BL_WORKSPACE`, and `~/.blaxel`.

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
| `kaxa mcp` (`start`, `init`, `config`) | [mcp](agent.md) |
