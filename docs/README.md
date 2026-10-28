# kaxa documentation

kaxa is a CLI for sandbox providers. Sprites is fully implemented. Modal supports create (including --sync), list, exec, console, and destroy; features that only exist for Sprites are rejected for Modal with a message that names the providers that have them. Create, destroy, and the list URL auth prompt use the Dracula theme.

Install kaxa with Homebrew, a curl download, or from source: [install](install.md).

Set up an account and token in [Sprites account and token](sprites-account.md) or [Modal account and token](modal-account.md).

## Configuration

Every command except `version` and `--help` loads configuration before it runs.

| Source | What it sets |
| --- | --- |
| `--provider` | `sprites` or `modal` for this invocation. `list` shows every provider when it is omitted |
| `KAXA_DEFAULT_PROVIDER` | Provider when `--provider` is omitted, except for `list` |
| `KAXA_SPRITES_TOKEN` | Sprites API token |
| `KAXA_MODAL_TOKEN_ID` | Modal token ID (`ak-...`) |
| `KAXA_MODAL_TOKEN_SECRET` | Modal token secret (`as-...`) |

`--provider` wins over `KAXA_DEFAULT_PROVIDER`. Tokens are never read from a flag, from the Sprites CLI config file, or from `~/.modal.toml`.

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
| `kaxa url` | [url](url.md) |
| `kaxa mcp` | [mcp](agent.md) |
