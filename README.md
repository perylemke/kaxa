# sbox

[![CI](https://github.com/perylemke/sbox/actions/workflows/ci.yml/badge.svg)](https://github.com/perylemke/sbox/actions/workflows/ci.yml)
[![Lint](https://github.com/perylemke/sbox/actions/workflows/lint.yml/badge.svg)](https://github.com/perylemke/sbox/actions/workflows/lint.yml)
[![Sec](https://github.com/perylemke/sbox/actions/workflows/sec.yml/badge.svg)](https://github.com/perylemke/sbox/actions/workflows/sec.yml)

Unified CLI for sandbox providers. **sbox** aggregates isolated execution environments behind one command-line interface, starting with [Fly.io Sprites](https://docs.fly.io/sprites) and [Modal Sandboxes](https://modal.com/docs/guide/sdk-javascript-go).

Sprites lifecycle commands (`create`, `list`, `exec`, `console`, `restart`, `upgrade`, `destroy`) are implemented when using `--provider sprites`. Modal remains a stub.

## Requirements

- Go 1.27+
- Accounts and API tokens for Sprites and/or Modal when you start using provider commands

## Install (from source)

```bash
go install ./cmd/sbox
```

## Configuration

Configuration is loaded from environment variables. CLI flags override environment when set (highest priority first):

1. CLI flags (`--provider`)
2. Environment variables with prefix `SBOX_`

Never commit real tokens.

### Environment variables

| Variable | Description |
| --- | --- |
| `SBOX_DEFAULT_PROVIDER` | Default provider (`sprites` or `modal`) |
| `SBOX_SPRITES_TOKEN` | Sprites API token |
| `SBOX_MODAL_TOKEN_ID` | Modal token ID |
| `SBOX_MODAL_TOKEN_SECRET` | Modal token secret |

## Usage

```bash
sbox --help
sbox version

export SBOX_DEFAULT_PROVIDER=sprites
export SBOX_SPRITES_TOKEN=...   # never commit real tokens

sbox create                     # form for name, provider, running, sync, and API
sbox create my-sprite
sbox create --running my-sprite  # stay up for 1 hour and open a tmux terminal in the browser
sbox create --sync my-sprite     # copy this git repository into /home/sprite/workspace/<repo>
sbox create --sync --dir ~/projects/api-python my-sprite
sbox create --sync --api --public --api-cmd .venv/bin/uvicorn,app.main:app,--host,0.0.0.0,--port,8000 my-sprite
sbox new my-sprite               # alias of create

sbox list                        # interactive table in a terminal; plain columns when piped
sbox ls                          # alias of list

sbox exec my-sprite -- ls -la    # run a command; remote exit code is propagated
sbox console my-sprite           # interactive bash (requires a terminal)
sbox ssh my-sprite               # alias of console

sbox restart                     # pick a sandbox when no name is given
sbox restart my-sprite
sbox upgrade my-sprite

sbox url my-sprite public          # open the Sprites URL without an account
sbox url my-sprite sprite          # require a Sprites account again

sbox destroy                     # pick sandboxes when no name is given
sbox destroy my-sprite
sbox destroy --force my-sprite   # skip the confirmation
sbox destroy -f --all
sbox rm my-sprite                # alias of destroy
```

You can pass `--provider sprites` instead of `SBOX_DEFAULT_PROVIDER`.

### create

`create` labels the sandbox `sbox`.

- `--running` keeps the sandbox up for one hour, serves tmux on its URL, and opens that URL.
- `--sync` copies the Git repository, including `.git` and excluding ignored files, into `/home/sprite/workspace/<repo>`. `--dir` selects another repository. A subdirectory still copies the whole repository. The root selects Go, Rust, Python, and Node; missing tools are installed, then project dependencies. A failed copy or install deletes the sandbox.
- `--api` publishes one HTTP service on the Sprites URL. `--api-cmd` is comma-separated argv, and the process must listen on `0.0.0.0` and on `--api-port` (8000 by default). With `--sync`, the repository must be an HTTP API; a CLI project is rejected before create. Create waits until the service is running, for up to 30 seconds, and deletes the sandbox if it does not. With `--running`, port `8080` stays with tmux and the URL forwards to the API.
- `--public` makes that URL reachable without a Sprites account. A failed auth change leaves the sandbox in place. Change it later with `sbox url <name> public` or `sbox url <name> sprite`. Only the sprites provider accepts `url`.

### exec and console

On a sandbox created with `--sync`, `exec` and `console` start in `/home/sprite/workspace`.

### list

With no sandboxes, `list` prints `no sandboxes created` and exits. In a terminal, highlight a row and press Enter for details. From the detail view, Enter opens the URL auth form, `r` restarts, `u` upgrades, `d` destroys, and `q` quits.

### restart, upgrade, and destroy

`restart` and `upgrade` without a name open a picker. `destroy` without a name opens a form where you can select one or more sandboxes. When stdin is not a terminal, enter numbers to toggle and `0` to confirm. Named sandboxes and `destroy --all` still ask for `[y/N]`, unless `--force` or `-f` is set.

## Documentation

| Manual | What it covers |
| --- | --- |
| [Sprites account](docs/sprites-account.md) | Account and API token |
| [version](docs/version.md) | `sbox version` |
| [create](docs/create.md) | `sbox create` (`new`) |
| [list](docs/list.md) | `sbox list` (`ls`) |
| [exec](docs/exec.md) | `sbox exec` |
| [console](docs/console.md) | `sbox console` (`ssh`) |
| [restart](docs/restart.md) | `sbox restart` |
| [upgrade](docs/upgrade.md) | `sbox upgrade` |
| [url](docs/url.md) | `sbox url` |
| [destroy](docs/destroy.md) | `sbox destroy` (`rm`) |

The same index, plus configuration, is in [docs/README.md](docs/README.md).

## Development

Run `make help` for all targets. Quick local build:

```bash
make build
./bin/sbox --help
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
