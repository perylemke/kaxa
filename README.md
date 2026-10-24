# sbox

[![CI](https://github.com/perylemke/sbox/actions/workflows/ci.yml/badge.svg)](https://github.com/perylemke/sbox/actions/workflows/ci.yml)

Unified CLI for sandbox providers. **sbox** aggregates isolated execution environments behind one command-line interface, starting with [Fly.io Sprites](https://docs.fly.io/sprites) and [Modal Sandboxes](https://modal.com/docs/guide/sdk-javascript-go).

Sprites lifecycle commands (`create`, `list`, `exec`, `restart`, `upgrade`, `destroy`) are implemented when using `--provider sprites`. Modal remains a stub.

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

sbox create                     # interactive form for name, provider, and running
sbox create my-sprite
sbox create --running my-sprite  # stay up for 1 hour and open a tmux terminal in the browser
sbox new my-sprite               # alias of create

sbox list                        # interactive table in a terminal; plain columns when piped
sbox ls                          # alias of list

sbox exec my-sprite              # interactive bash (requires a terminal)
sbox exec my-sprite -- ls -la    # run a command; remote exit code is propagated

sbox restart                     # pick a sandbox when no name is given
sbox restart my-sprite
sbox upgrade my-sprite

sbox destroy                     # pick sandboxes when no name is given
sbox destroy my-sprite
sbox destroy --all
sbox rm my-sprite                # alias of destroy
```

You can pass `--provider sprites` instead of `SBOX_DEFAULT_PROVIDER`.

`create` labels the sandbox `sbox`. `--running` registers a one-hour hold, serves a browser tmux session on the sprite URL, and opens that URL. In a terminal, `list` highlights a row: Enter shows sprite details, `r` restarts, `u` upgrades, `d` destroys, and `q` quits. With no sandboxes, `list` prints `no sandboxes created` and exits. `restart` and `upgrade` without a name open the same kind of picker; `destroy` without a name asks you to choose. Each of those actions confirms before it runs.

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
