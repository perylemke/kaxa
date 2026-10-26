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

`create` labels the sandbox `sbox`. `--running` registers a one-hour hold, serves a browser tmux session on the sprite URL, and opens that URL. `--sync` copies the current Git repository, including `.git` and excluding ignored files, into `/home/sprite/workspace/<repo>` on a Sprites sandbox. `--dir` selects another repository; without it, sync uses the directory where sbox is running. A subdirectory still copies the whole Git repository. If that copy fails, the sandbox is deleted. The repository root selects Go, Rust, Python, and Node; missing tools are installed in the sandbox and then the project dependencies, and a failure deletes the sandbox. `--api` then publishes one HTTP service on the Sprites URL: `--api-cmd` is comma-separated argv, as in `.venv/bin/uvicorn,app.main:app,--host,0.0.0.0,--port,8000`, and the process must listen on `0.0.0.0` and on `--api-port` (8000 by default). With `--sync`, `--api` requires an HTTP API in the repository; a CLI project is rejected and the sandbox is not created. Create waits until the service is running; if it fails or does not become running within 30s, the sandbox is deleted. `--public` makes that URL reachable without a Sprites account during create. `sbox url <name> public` and `sbox url <name> sprite` change that access later, and only the sprites provider accepts the command. With `--running` and `--api` together, tmux still starts and the URL forwards to the API; if publishing the service fails, the sandbox is deleted. `exec` and `console` on a sandbox created with `--sync` start in `/home/sprite/workspace`. In a terminal, `list` highlights a row: Enter shows sprite details, Enter again opens the URL auth form, `r` restarts, `u` upgrades, `d` destroys, and `q` quits. With no sandboxes, `list` prints `no sandboxes created` and exits. `restart` and `upgrade` without a name open the same kind of picker; `destroy` without a name opens a form where you can select one or more sandboxes (when stdin is not a terminal, enter numbers to toggle and `0` to confirm). Named sandboxes and `destroy --all` still ask for `[y/N]` before they run, unless `--force` or `-f` is set.

## Documentation

Command manuals and Sprites account setup are in [docs/README.md](docs/README.md). Create a token with [docs/sprites-account.md](docs/sprites-account.md).

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
