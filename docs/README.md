# sbox documentation

sbox is a CLI for sandbox providers. Sprites is implemented. Modal is a stub: commands that need Sprites are rejected when the provider is Modal. Create, destroy, and the list URL auth prompt use the Dracula theme.

Set up a Sprites account and token in [Sprites account and token](sprites-account.md).

## Configuration

Every command except `version` and `--help` loads configuration before it runs.

| Source | What it sets |
| --- | --- |
| `--provider` | `sprites` or `modal` for this invocation |
| `SBOX_DEFAULT_PROVIDER` | Provider when `--provider` is omitted |
| `SBOX_SPRITES_TOKEN` | Sprites API token |

`--provider` wins over `SBOX_DEFAULT_PROVIDER`. The token is never read from a flag or from the Sprites CLI config file.

## Commands

| Command | Manual |
| --- | --- |
| `sbox version` | [version](version.md) |
| `sbox create` (`new`) | [create](create.md) |
| `sbox list` (`ls`) | [list](list.md) |
| `sbox exec` | [exec](exec.md) |
| `sbox console` (`ssh`) | [console](console.md) |
| `sbox destroy` (`rm`) | [destroy](destroy.md) |
| `sbox restart` | [restart](restart.md) |
| `sbox upgrade` | [upgrade](upgrade.md) |
| `sbox url` | [url](url.md) |
| `sbox mcp` | [mcp](agent.md) |
