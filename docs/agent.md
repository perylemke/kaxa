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

`go install` writes it to `$(go env GOPATH)/bin/kaxa`, or to `$GOBIN` when that variable is set. `make build` writes `./bin/kaxa` inside this repository. A Homebrew install (see [install](install.md)) links it at `$(brew --prefix)/bin/kaxa`, for example `/opt/homebrew/bin/kaxa` on Apple Silicon. `command` is that path. The folder of the binary does not matter.

A client started from a terminal inherits your `PATH`, so `command` can be `kaxa`. Cursor and Claude Desktop, started from the desktop, do not inherit that `PATH`. Give them the absolute path, for example `/Users/you/go/bin/kaxa`. A missing file shows the server as disconnected.

`sync` uses the working directory of the `kaxa mcp` process, not the directory that contains the binary. The repository must contain that working directory or sit below it. A desktop client uses the opened project. Set `cwd` when the client has that field and might start elsewhere.

The tokens stay in the environment of that process. Put `KAXA_DEFAULT_PROVIDER` and the token of each provider you use in the client's `env` block: `KAXA_SPRITES_TOKEN` for Sprites, `KAXA_MODAL_TOKEN_ID` with `KAXA_MODAL_TOKEN_SECRET` for Modal, and `KAXA_E2B_API_KEY` for E2B. Do not commit a file that contains a token, and do not paste a token into a prompt. See [Sprites account and token](sprites-account.md) [Modal account and token](modal-account.md), and [E2B account and API key](e2b-account.md). `--provider` on the command also sets the default.

A bare `KEY` in `api_env` copies a variable from the `kaxa mcp` environment, so an agent could ask for any secret that environment holds. kaxa refuses it unless the name is in `KAXA_MCP_API_ENV`, a comma-separated list that you set in the same `env` block, for example `"KAXA_MCP_API_ENV": "DATABASE_URL,REDIS_URL"`. Without it, the agent can only pass `KEY=VALUE`.

After the server is connected, ask the agent to call `list_providers`. Sprites, Modal, E2B, and Docker are available. Modal supports create (with `sync`), list, inspect, exec, snapshots, and destroy; its `capabilities` report `sync`, `setup`, `workspace_layout`, and `snapshot` as true and the rest as false. On Modal, `exec` commands start in the image root unless the sandbox was created with `sync`; then they run as the `modal` user in `/home/modal/workspace`. E2B supports create (with `sync` and `api`), list, inspect, exec, restart, and destroy; its `capabilities` report `sync`, `setup`, `api`, `workspace_layout`, and `restart` as true and the rest as false. On E2B, `exec` commands run as `user`, starting in `/home/user`, or in `/home/user/workspace` when the sandbox was created with `sync`. Docker supports create (with `sync`, `template`, and `api`), list, inspect, exec, restart, snapshots, and destroy through the Docker Sandboxes API, with `KAXA_DOCKER_ID` and `KAXA_DOCKER_PAT` set in the environment of `kaxa mcp` and `KAXA_DOCKER_TEMPLATE` for a default template; its `capabilities` report `sync`, `setup`, `api`, `workspace_layout`, `restart`, `template`, and `snapshot` as true and the rest as false. `api` publishes one HTTP service on a URL that Docker chooses (anyone with the URL can call it). On Docker, `exec` commands run as `agent`, starting where the image starts, or in `/home/agent/workspace` when the sandbox was created with `sync`. A Docker sandbox is billed to your Docker subscription while it runs.

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
        "KAXA_SPRITES_TOKEN": "paste-the-dashboard-value",
        "KAXA_MODAL_TOKEN_ID": "ak-paste-the-token-id",
        "KAXA_MODAL_TOKEN_SECRET": "as-paste-the-token-secret",
        "KAXA_E2B_API_KEY": "e2b_paste-the-key"
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
        "KAXA_SPRITES_TOKEN": "paste-the-dashboard-value",
        "KAXA_MODAL_TOKEN_ID": "ak-paste-the-token-id",
        "KAXA_MODAL_TOKEN_SECRET": "as-paste-the-token-secret",
        "KAXA_E2B_API_KEY": "e2b_paste-the-key"
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
  --env KAXA_MODAL_TOKEN_ID=ak-paste-the-token-id \
  --env KAXA_MODAL_TOKEN_SECRET=as-paste-the-token-secret \
  --env KAXA_E2B_API_KEY=e2b_paste-the-key \
  kaxa -- /Users/you/go/bin/kaxa mcp
```

Check with `claude mcp list` or `/mcp`.

### Claude Desktop

On macOS the file is `~/Library/Application Support/Claude/claude_desktop_config.json`. On Windows it is `%APPDATA%\Claude\claude_desktop_config.json`. Use the same `mcpServers` object as Cursor. Quit and reopen Claude Desktop after saving.

### Gemini CLI

User config is `~/.gemini/settings.json`. Project config is `.gemini/settings.json`. `env` values expand `$KAXA_SPRITES_TOKEN` and `${KAXA_SPRITES_TOKEN}`, and the Modal and E2B variables the same way, from the environment that launches Gemini. Set `cwd` to the project so `sync` can see it.

```json
{
  "mcpServers": {
    "kaxa": {
      "command": "/Users/you/go/bin/kaxa",
      "args": ["mcp"],
      "cwd": "/Users/you/projects/your-repo",
      "env": {
        "KAXA_DEFAULT_PROVIDER": "sprites",
        "KAXA_SPRITES_TOKEN": "${KAXA_SPRITES_TOKEN}",
        "KAXA_MODAL_TOKEN_ID": "${KAXA_MODAL_TOKEN_ID}",
        "KAXA_MODAL_TOKEN_SECRET": "${KAXA_MODAL_TOKEN_SECRET}",
        "KAXA_E2B_API_KEY": "${KAXA_E2B_API_KEY}"
      }
    }
  }
}
```

### Another client

Use the stdio entry your client documents: a command, the argument `mcp`, and the `KAXA_` variables of the providers you use. An HTTP URL is a different transport. `kaxa mcp` does not listen on a port.

## Tools

Every tool that works on a sandbox takes an optional `provider`. Without it, the default provider is used.

| Tool | What it does |
| --- | --- |
| `list_providers` | Registered providers, whether each is available, which one is the default, and its capabilities |
| `create_sandbox` | Create a sandbox. Same flow as [create](create.md), without the form |
| `sync_repository` | Copy another Git repository into a sandbox created with `sync`. Same flow as [sync](sync.md) |
| `exec` | Run an argv and return `stdout`, `stderr`, `exit_code` and `truncated` |
| `list_sandboxes` | Sandboxes of every configured provider, or of one with `provider` |
| `inspect` | Status, labels, URL and URL auth of one sandbox |
| `restart`, `upgrade` | Same as [restart](restart.md) and [upgrade](upgrade.md) |
| `destroy` | Delete a sandbox. The name is required |
| `create_snapshot`, `list_snapshots`, `restore_snapshot`, `delete_snapshot` | Save, list, restore, and delete sandbox snapshots. See [snapshot](snapshot.md) and [Snapshots](#snapshots) |
| `publish_api` | Start one HTTP service in an existing sandbox |
| `set_url_auth` | Same as [url](url.md): `public` or `sprite` |

### Use E2B

Set `KAXA_E2B_API_KEY` in the client (and `KAXA_E2B_TEMPLATE` for a default template), reload the server, and pass `provider: "e2b"` (or set `KAXA_DEFAULT_PROVIDER=e2b`). The flow is the same as on Modal, with `/home/user/workspace` as the workspace and `user` as the user. Pass `template` to `create_sandbox` to start from a template you built with the E2B CLI or SDK, for example one with more memory than `base`. It must keep the `base` layout (user `user`). Other providers reject it. Two things differ:

- E2B sandboxes are private. `create_sandbox` with `api: true` is what makes the API port public, at `https://<port>-<sandbox id>.e2b.app`, and `publish_api` on an existing sandbox must use the port the sandbox was created with. Anyone with the URL can call the service.
- A sandbox pauses after one hour without activity and resumes on the next `exec`. `restart` reboots it from its disk, which stops running processes. `set_url_auth`, snapshots, and `running` are not available for E2B. See [E2B account and API key](e2b-account.md).

### Use Modal

Set `KAXA_MODAL_TOKEN_ID` and `KAXA_MODAL_TOKEN_SECRET` in the client, reload the server, and the flow is the same as on Sprites. Pass `provider: "modal"`, or set `KAXA_DEFAULT_PROVIDER=modal`:

1. `create_sandbox` with `name`, `provider: "modal"`, and `sync: true`. The result has `work_dir`, for example `/home/modal/workspace/your-repo`. The repository is copied as the `modal` user and its toolchain is installed.
2. `exec` with `command: ["bash", "-c", "cd <work_dir> && go test ./..."]`. The command runs as `modal` and starts in `/home/modal/workspace`.
3. `destroy` when the work is done.

Edit files in the sandbox with `exec`, for example `["bash", "-c", "cd <work_dir> && sed -i ... file"]`. The agent's local editor does not write there. Modal sandboxes live at most 24 hours and stop after one hour without activity; see [Modal account and token](modal-account.md). `--running`, `publish_api`, and public URLs are not available for Modal, and the error names the providers that have them.

### list_providers

Call this first. A provider is `available: false` when it cannot serve requests yet. The other tools reject an unavailable provider before calling it.

`capabilities` lists the optional features a provider implements:

| Capability | Needed for |
| --- | --- |
| `sync` | `create_sandbox` with `sync`, and `sync_repository` |
| `setup` | Dependency install after a sync |
| `api` | `create_sandbox` with `api`, and `publish_api` |
| `public_url` | `create_sandbox` with `public`, and `set_url_auth` |
| `restart` | `restart` |
| `upgrade` | `upgrade` |
| `running` | `create_sandbox` with `running` |
| `template` | `create_sandbox` with `template` (E2B) |
| `workspace_layout` | `work_dir` in the result, and the default directory of `publish_api` |
| `snapshot` | `create_snapshot`, `list_snapshots`, `restore_snapshot`, and `delete_snapshot` |
| `snapshot_restore` | Not a flag: `in_place` or `new_sandbox`, set when `snapshot` is true. It decides how `restore_snapshot` behaves |

A request that needs a missing capability fails before anything is created, with `<feature> is not available for <provider>`, and a hint that names the providers that do offer the feature.

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
| `api_env` | Service environment as a list. Each item is `KEY=VALUE`, or `KEY` to copy the value from the server environment, but only for names the user listed in `KAXA_MCP_API_ENV`. No commas in values. `HOME`, `USER`, `PATH`, and names that start with `KAXA_`, `MODAL_TOKEN_`, `FLY_`, or `SPRITES_` are reserved. Requires `api` |
| `public` | Make the URL reachable without a provider account. Refused with `running` unless `api` is set, because the tmux terminal has no login |

The result has `id`, `provider`, `work_dir`, `url` and `auth`. `work_dir` is where the repository lives inside the sandbox and is set only with `sync`. Use it instead of assuming a path.

A failed copy, install or API publish deletes the sandbox. A failed `public` change keeps it, and the error says so. Create can take minutes when `sync` installs toolchains. The server writes each step to stderr while it waits.

### sync_repository

Adds one more Git repository to a sandbox that was created with `sync`.

| Argument | Meaning |
| --- | --- |
| `name` | Required. The sandbox |
| `provider` | Provider name. Defaults to the server default |
| `dir` | A directory inside the repository to copy. Default: the server working directory |

The result has `id`, `provider` and `work_dir`, the directory of the new repository in the sandbox. The repository is subject to the same working directory rule as `create_sandbox`.

The tool never replaces a directory. It fails before copying when the sandbox was not created with `sync`, when the provider cannot sync, or when a directory with the repository's name is already in the sandbox. A failed copy or install keeps the sandbox, because it holds other work. The error says which step failed. It can take minutes while dependencies install.

### list_sandboxes

Without `provider`, the tool lists every provider that has credentials, in provider name order, and each item names its provider. A provider that fails while another works is reported in `warnings`; if all fail, the call is an error. With `provider`, only that one is listed.

### exec

`command` is an argv list. It never goes through a shell. To use shell syntax, pass `["bash", "-c", "..."]`.

A command that exits non-zero is a result: the tool succeeds and `exit_code` is set. A tool error means kaxa could not run the command, for example a missing sandbox or a provider failure.

Each of `stdout` and `stderr` keeps the first 256 KiB. When more is produced, the rest is dropped, a marker line ends the text, and `truncated` is true. The command is not interrupted. There is no stdin, so a command that reads input sees end of file.

`timeout_seconds` stops a command that runs too long. It defaults to 1800 (30 minutes) and cannot exceed 7200 (2 hours). A command stopped by the timeout is a tool error.

On a sandbox created with `sync`, the command starts in the provider workspace directory, which contains the repository. For Sprites that is `/home/sprite/workspace` and for Modal `/home/modal/workspace`, and `work_dir` is a subdirectory of it. Run project commands from `work_dir`, for example `["bash", "-c", "cd <work_dir> && go test ./..."]`.

### Snapshots

`create_snapshot` takes `name`, an optional `comment`, and an optional `provider`. It returns the snapshot `id`, `sandbox`, `provider`, `comment`, and `created_at`. `list_snapshots` takes an optional `name` and an optional `provider`, and returns `snapshots` with the `provider` and `sandbox` of each. Without them it lists every snapshot of every configured provider, and a provider that fails is reported in `warnings`. `delete_snapshot` takes `name` and `snapshot`, and is permanent.

`restore_snapshot` takes `name`, `snapshot`, and `target`. Read `snapshot_restore` in `list_providers` first:

| `snapshot_restore` | Provider | `target` | Effect |
| --- | --- | --- | --- |
| `in_place` | Sprites | Rejected | Overwrites the sandbox and discards what changed after the snapshot |
| `new_sandbox` | Modal, Docker | Required, and different from `name` | Creates `target` from the snapshot and keeps the source |

The result has `sandbox` (the one that holds the restored state), `snapshot`, `provider`, and `status`. `restore_snapshot` and `delete_snapshot` are marked destructive so a client can ask first; call them only when the user asks. A snapshot holds everything in the sandbox, including secrets written there. Modal snapshots are kept for 30 days and are listed only on the machine that took them. Docker snapshots are kept until deleted and are listed from any machine. See [snapshot](snapshot.md).

## Limits

- Edits made with the agent's local file tools are not in the sandbox. Edits made in the sandbox are not copied back. Move code with `git` inside `exec`, or create a new sandbox with `sync`. There is no pull tool yet.
- `sync_repository` adds a repository but never refreshes one. To send new local changes, create a new sandbox or move the code with `git` inside `exec`.
- `sync` and `sync_repository` copy only a repository that contains the server working directory or sits below it. This keeps a prompt from sending an unrelated repository to a remote provider. Start `kaxa mcp` from the project, or pass a `dir` inside it. A symlink that leaves the working directory is rejected. The server refuses to start sync when its working directory is the filesystem root or your home directory, because that would allow every repository under it; set `cwd` to the project.
- Network policy and connectors are not exposed. Use the provider's own tools for them. Memory snapshots are not supported.
- `destroy` is permanent, and its tool is marked destructive so a client can ask first. On Modal and Docker it does not delete snapshots, which outlive the sandbox; on Sprites the checkpoints are destroyed with the sprite.

## Errors

A failed tool call returns an error result with the message and, when there is one, the next step in parentheses. Tokens never appear in tool arguments or results. Provider errors are redacted before they reach the agent.
