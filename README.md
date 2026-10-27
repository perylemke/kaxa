# kaxa

[![CI](https://github.com/perylemke/kaxa/actions/workflows/ci.yml/badge.svg)](https://github.com/perylemke/kaxa/actions/workflows/ci.yml)
[![Lint](https://github.com/perylemke/kaxa/actions/workflows/lint.yml/badge.svg)](https://github.com/perylemke/kaxa/actions/workflows/lint.yml)
[![Sec](https://github.com/perylemke/kaxa/actions/workflows/sec.yml/badge.svg)](https://github.com/perylemke/kaxa/actions/workflows/sec.yml)

Unified CLI for sandbox providers. **kaxa** aggregates isolated execution environments behind one command-line interface, starting with [Fly.io Sprites](https://docs.fly.io/sprites) and [Modal Sandboxes](https://modal.com/docs/guide/sdk-javascript-go).

Sprites lifecycle commands (`create`, `list`, `exec`, `console`, `restart`, `upgrade`, `destroy`) are implemented when using `--provider sprites`. Modal remains a stub.

## Requirements

- Go 1.27+
- Accounts and API tokens for Sprites and/or Modal when you start using provider commands

## Install (from source)

```bash
go install ./cmd/kaxa
```

## Configuration

Configuration is loaded from environment variables. CLI flags override environment when set (highest priority first):

1. CLI flags (`--provider`)
2. Environment variables with prefix `KAXA_`

Never commit real tokens.

### Environment variables

| Variable | Description |
| --- | --- |
| `KAXA_DEFAULT_PROVIDER` | Default provider (`sprites` or `modal`) |
| `KAXA_SPRITES_TOKEN` | Sprites API token |
| `KAXA_MODAL_TOKEN_ID` | Modal token ID |
| `KAXA_MODAL_TOKEN_SECRET` | Modal token secret |

## Usage

```bash
kaxa --help
kaxa version

export KAXA_DEFAULT_PROVIDER=sprites
export KAXA_SPRITES_TOKEN=...   # never commit real tokens

kaxa create                     # form for name, provider, running, sync, and API
kaxa create my-sprite
kaxa create --running my-sprite  # stay up for 1 hour and open a tmux terminal in the browser
kaxa create --sync my-sprite     # copy this git repository into /home/sprite/workspace/<repo>
kaxa create --sync --dir ~/projects/api-python my-sprite
kaxa create --sync --api --public --api-cmd .venv/bin/uvicorn,app.main:app,--host,0.0.0.0,--port,8000 my-sprite
kaxa new my-sprite               # alias of create

kaxa list                        # interactive table in a terminal; plain columns when piped
kaxa ls                          # alias of list

kaxa exec my-sprite -- ls -la    # run a command; remote exit code is propagated
kaxa console my-sprite           # interactive bash (requires a terminal)
kaxa ssh my-sprite               # alias of console

kaxa restart                     # pick a sandbox when no name is given
kaxa restart my-sprite
kaxa upgrade my-sprite

kaxa url my-sprite public          # open the Sprites URL without an account
kaxa url my-sprite sprite          # require a Sprites account again

kaxa destroy                     # pick sandboxes when no name is given
kaxa destroy my-sprite
kaxa destroy --force my-sprite   # skip the confirmation
kaxa destroy -f --all
kaxa rm my-sprite                # alias of destroy
```

You can pass `--provider sprites` instead of `KAXA_DEFAULT_PROVIDER`.

### create

`create` labels the sandbox `kaxa`.

- `--running` keeps the sandbox up for one hour, serves tmux on its URL, and opens that URL.
- `--sync` copies the Git repository, including `.git` and excluding ignored files, into `/home/sprite/workspace/<repo>`. `--dir` selects another repository. A subdirectory still copies the whole repository. The root selects Go, Rust, Python, Node, Ruby (`Gemfile`), and Clojure (`deps.edn`); missing tools are installed, then project dependencies. A failed copy or install deletes the sandbox.
- `--api` publishes one HTTP service on the Sprites URL. `--api-cmd` is comma-separated argv, and the process must listen on `0.0.0.0` and on `--api-port` (8000 by default). With `--sync`, the repository must be an HTTP API; a CLI project is rejected before create. Create waits until the service is running, for up to 90 seconds, and deletes the sandbox if it does not. With `--running`, port `8080` stays with tmux and the URL forwards to the API. `--api-env KEY=VALUE` or `--api-env KEY` (value copied from your shell) sets service variables; see [create](docs/create.md).
- `--public` makes that URL reachable without a Sprites account. A failed auth change leaves the sandbox in place. Change it later with `kaxa url <name> public` or `kaxa url <name> sprite`. Only the sprites provider accepts `url`.

### exec and console

On a sandbox created with `--sync`, `exec` and `console` start in `/home/sprite/workspace`.

### list

With no sandboxes, `list` prints `no sandboxes created` and exits. In a terminal, highlight a row and press Enter for details. From the detail view, Enter opens the URL auth form, `r` restarts, `u` upgrades, `d` destroys, and `q` quits.

### restart, upgrade, and destroy

`restart` and `upgrade` without a name open a picker. `destroy` without a name opens a form where you can select one or more sandboxes. When stdin is not a terminal, enter numbers to toggle and `0` to confirm. Named sandboxes and `destroy --all` still ask for `[y/N]`, unless `--force` or `-f` is set.

### AI agents

`kaxa mcp` serves sandboxes to an AI agent over the Model Context Protocol. Cursor, Claude Code, Claude Desktop, and Gemini CLI each start that binary on stdin and stdout. The agent stays in the client and calls tools to create a sandbox from your Git repository, run commands in it, and destroy it.

Point `command` at the installed binary. `go install` puts it on `$(go env GOPATH)/bin`. A desktop app needs that absolute path. The token stays in the client's environment, not in a committed file. Client files, `cwd`, and the tool list are in [docs/agent.md](docs/agent.md).

## Documentation

| Manual | What it covers |
| --- | --- |
| [Sprites account](docs/sprites-account.md) | Account and API token |
| [version](docs/version.md) | `kaxa version` |
| [create](docs/create.md) | `kaxa create` (`new`) |
| [list](docs/list.md) | `kaxa list` (`ls`) |
| [exec](docs/exec.md) | `kaxa exec` |
| [console](docs/console.md) | `kaxa console` (`ssh`) |
| [restart](docs/restart.md) | `kaxa restart` |
| [upgrade](docs/upgrade.md) | `kaxa upgrade` |
| [url](docs/url.md) | `kaxa url` |
| [destroy](docs/destroy.md) | `kaxa destroy` (`rm`) |
| [mcp](docs/agent.md) | `kaxa mcp`, sandboxes for AI agents |

The same index, plus configuration, is in [docs/README.md](docs/README.md).

## Development

Run `make help` for all targets. Quick local build:

```bash
make build
./bin/kaxa --help
```

```bash
# Unit tests (default)
go test -race -shuffle=on ./...
# or: make test

# Integration tests
go test -race -count=1 -tags=integration ./internal/config/...

# End-to-end tests (builds binary)
go test -race -count=1 -tags=e2e ./test/e2e/...

# Acceptance tests
go test -race -count=1 -tags=acceptance ./test/acceptance/...
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Security reports: [SECURITY.md](SECURITY.md).

## License

Apache License 2.0 — see [LICENSE](LICENSE).
