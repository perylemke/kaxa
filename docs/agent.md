# kaxa mcp

Serve sandboxes to AI agents over the [Model Context Protocol](https://modelcontextprotocol.io).

```bash
kaxa mcp
```

The server talks JSON-RPC on stdin and stdout. An MCP client starts it as a child process. Stdout carries only the protocol. Progress lines and logs go to stderr. Run it by hand only to see that it starts; it waits for a client.

The agent keeps running in your client, such as Cursor. It calls kaxa tools, and the work happens in the sandbox. The tools use sandbox terms, and the provider is an argument. A workflow written for one provider works with another that supports the same features.

## Connect a client

Any MCP client that can start a local program can use `kaxa mcp`. The client runs the binary and speaks JSON-RPC on its stdin and stdout. Cursor, Claude, and Gemini each store that command in their own file. The tools are the same.

Install the binary wherever you keep commands:

```bash
go install ./cmd/kaxa
```

`go install` writes it to `$(go env GOPATH)/bin/kaxa`, or to `$GOBIN` when that variable is set. `make build` writes `./bin/kaxa` inside this repository. `command` is that path. The folder of the binary does not matter.

A client started from a terminal inherits your `PATH`, so `command` can be `kaxa`. Cursor and Claude Desktop, started from the desktop, do not inherit that `PATH`. Give them the absolute path, for example `/Users/you/go/bin/kaxa`. A missing file shows the server as disconnected.

`sync` uses the working directory of the `kaxa mcp` process, not the directory that contains the binary. The repository must contain that working directory or sit below it. A desktop client uses the opened project. Set `cwd` when the client has that field and might start elsewhere.

The token stays in the environment of that process. Put `KAXA_DEFAULT_PROVIDER` and `KAXA_SPRITES_TOKEN` in the client's `env` block. Do not commit a file that contains the token, and do not paste the token into a prompt. See [Sprites account and token](sprites-account.md). `--provider` on the command also sets the default.

After the server is connected, ask the agent to call `list_providers`. Sprites is available. Modal is registered and reports `available: false`.

### Cursor

User config is `~/.cursor/mcp.json`. Project config is `.cursor/mcp.json`. Prefer the user file when `env` holds a token, and do not commit it.

```json
{
  "mcpServers": {
    "kaxa": {
      "command": "/Users/you/go/bin/kaxa",
      "args": ["mcp"],
      "env": {
        "KAXA_DEFAULT_PROVIDER": "sprites",
        "KAXA_SPRITES_TOKEN": "paste-the-dashboard-value"
      }
    }
  }
}
```

Open Cursor Settings, MCP, and enable the kaxa tools. Reload the server after changing the file.

### Claude Code

Project config is `.mcp.json` at the repository root. Claude Code asks you to approve it once. User config is `~/.claude.json`.

```json
{
  "mcpServers": {
    "kaxa": {
      "type": "stdio",
      "command": "/Users/you/go/bin/kaxa",
      "args": ["mcp"],
      "env": {
        "KAXA_DEFAULT_PROVIDER": "sprites",
        "KAXA_SPRITES_TOKEN": "paste-the-dashboard-value"
      }
    }
  }
}
```

From a terminal:

```bash
claude mcp add --transport stdio \
  --env KAXA_DEFAULT_PROVIDER=sprites \
  --env KAXA_SPRITES_TOKEN=paste-the-dashboard-value \
  kaxa -- /Users/you/go/bin/kaxa mcp
```

Check with `claude mcp list` or `/mcp`.

### Claude Desktop

On macOS the file is `~/Library/Application Support/Claude/claude_desktop_config.json`. On Windows it is `%APPDATA%\Claude\claude_desktop_config.json`. Use the same `mcpServers` object as Cursor. Quit and reopen Claude Desktop after saving.

### Gemini CLI

User config is `~/.gemini/settings.json`. Project config is `.gemini/settings.json`. `env` values expand `$KAXA_SPRITES_TOKEN` and `${KAXA_SPRITES_TOKEN}` from the environment that launches Gemini. Set `cwd` to the project so `sync` can see it.

```json
{
  "mcpServers": {
    "kaxa": {
      "command": "/Users/you/go/bin/kaxa",
      "args": ["mcp"],
      "cwd": "/Users/you/projects/your-repo",
      "env": {
        "KAXA_DEFAULT_PROVIDER": "sprites",
        "KAXA_SPRITES_TOKEN": "${KAXA_SPRITES_TOKEN}"
      }
    }
  }
}
```

### Another client

Use the stdio entry your client documents: a command, the argument `mcp`, and the two `KAXA_` variables. An HTTP URL is a different transport. `kaxa mcp` does not listen on a port.

## Tools

Every tool that works on a sandbox takes an optional `provider`. Without it, the default provider is used.

| Tool | What it does |
| --- | --- |
| `list_providers` | Registered providers, whether each is available, which one is the default, and its capabilities |
| `create_sandbox` | Create a sandbox. Same flow as [create](create.md), without the form |
| `exec` | Run an argv and return `stdout`, `stderr`, `exit_code` and `truncated` |
| `list_sandboxes` | Sandboxes of one provider |
| `inspect` | Status, labels, URL and URL auth of one sandbox |
| `restart`, `upgrade` | Same as [restart](restart.md) and [upgrade](upgrade.md) |
| `destroy` | Delete a sandbox. The name is required |
| `publish_api` | Start one HTTP service in an existing sandbox |
| `set_url_auth` | Same as [url](url.md): `public` or `sprite` |

### list_providers

Call this first. A provider is `available: false` when it cannot serve requests yet. Modal is a stub today, so it reports false and the other tools reject it before calling it.

`capabilities` lists the optional features a provider implements:

| Capability | Needed for |
| --- | --- |
| `sync` | `create_sandbox` with `sync` |
| `setup` | Dependency install after a sync |
| `api` | `create_sandbox` with `api`, and `publish_api` |
| `public_url` | `create_sandbox` with `public`, and `set_url_auth` |
| `workspace_layout` | `work_dir` in the result, and the default directory of `publish_api` |

A request that needs a missing capability fails before anything is created, with `the <provider> provider does not support <feature>`.

### create_sandbox

| Argument | Meaning |
| --- | --- |
| `name` | Required. Sandbox name |
| `provider` | Provider name. Defaults to the server default |
| `sync` | Copy a Git repository into the sandbox and install its dependencies |
| `dir` | A directory inside the repository to copy. Requires `sync`. Default: the server working directory |
| `running` | Keep the sandbox up for one hour and serve tmux on its URL. No browser is opened |
| `api` | Publish one HTTP service. Requires `api_cmd` |
| `api_cmd` | Argv of the service as a list. The first item is the binary |
| `api_port` | Port the service listens on. Default `8000`. `8080` is reserved with `running` |
| `api_env` | Service environment as a list. Each item is `KEY=VALUE`, or `KEY` to copy the value from the server environment. No commas in values. `HOME`, `USER`, `PATH`, and `KAXA_` names are reserved. Requires `api` |
| `public` | Make the URL reachable without a provider account |

The result has `id`, `provider`, `work_dir`, `url` and `auth`. `work_dir` is where the repository lives inside the sandbox and is set only with `sync`. Use it instead of assuming a path.

A failed copy, install or API publish deletes the sandbox. A failed `public` change keeps it, and the error says so. Create can take minutes when `sync` installs toolchains. The server writes each step to stderr while it waits.

### exec

`command` is an argv list. It never goes through a shell. To use shell syntax, pass `["bash", "-c", "..."]`.

A command that exits non-zero is a result: the tool succeeds and `exit_code` is set. A tool error means kaxa could not run the command, for example a missing sandbox or a provider failure.

Each of `stdout` and `stderr` keeps the first 256 KiB. When more is produced, the rest is dropped, a marker line ends the text, and `truncated` is true. The command is not interrupted. There is no stdin, so a command that reads input sees end of file.

On a sandbox created with `sync`, the command starts in the provider workspace directory, which contains the repository. For Sprites that is `/home/sprite/workspace`, and `work_dir` is a subdirectory of it. Run project commands from `work_dir`, for example `["bash", "-c", "cd <work_dir> && go test ./..."]`.

## Limits

- Edits made with the agent's local file tools are not in the sandbox. Edits made in the sandbox are not copied back. Move code with `git` inside `exec`, or run `sync` again by creating a new sandbox. There is no pull tool yet.
- `sync` copies the Git repository only at create time.
- `sync` copies only a repository that contains the server working directory or sits below it. This keeps a prompt from sending an unrelated repository to a remote provider. Start `kaxa mcp` from the project, or pass a `dir` inside it. A symlink that leaves the working directory is rejected.
- Checkpoints, network policy and connectors are not exposed. Use the provider's own tools for them.
- Only Sprites is implemented. `destroy` is permanent, and its tool is marked destructive so a client can ask first.

## Errors

A failed tool call returns an error result with the message and, when there is one, the next step in parentheses. Tokens never appear in tool arguments or results. Provider errors are redacted before they reach the agent.
