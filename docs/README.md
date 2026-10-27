# kaxa documentation

kaxa is a CLI for sandbox providers. Sprites is implemented. Modal is a stub: commands that need Sprites are rejected when the provider is Modal. Create, destroy, and the list URL auth prompt use the Dracula theme.

Set up a Sprites account and token in [Sprites account and token](sprites-account.md).

## Configuration

Every command except `version` and `--help` loads configuration before it runs.

| Source | What it sets |
| --- | --- |
| `--provider` | `sprites` or `modal` for this invocation |
| `KAXA_DEFAULT_PROVIDER` | Provider when `--provider` is omitted |
| `KAXA_SPRITES_TOKEN` | Sprites API token |

`--provider` wins over `KAXA_DEFAULT_PROVIDER`. The token is never read from a flag or from the Sprites CLI config file.

## Commands

| Command | Manual |
| --- | --- |
| `kaxa version` | [version](version.md) |
| `kaxa create` (`new`) | [create](create.md) |
| `kaxa list` (`ls`) | [list](list.md) |
| `kaxa exec` | [exec](exec.md) |
| `kaxa console` (`ssh`) | [console](console.md) |
| `kaxa destroy` (`rm`) | [destroy](destroy.md) |
| `kaxa restart` | [restart](restart.md) |
| `kaxa upgrade` | [upgrade](upgrade.md) |
| `kaxa url` | [url](url.md) |
| `kaxa mcp` | [mcp](agent.md) |
