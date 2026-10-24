# sbx

[![CI](https://github.com/perylemke/sbx/actions/workflows/ci.yml/badge.svg)](https://github.com/perylemke/sbx/actions/workflows/ci.yml)

Unified CLI for sandbox providers. **sbx** aggregates isolated execution environments behind one command-line interface, starting with [Fly.io Sprites](https://docs.fly.io/sprites) and [Modal Sandboxes](https://modal.com/docs/guide/sdk-javascript-go).

This repository is early-stage: the root command loads configuration and registers providers; sandbox lifecycle commands are not implemented yet.

## Requirements

- Go 1.27+
- Accounts and API tokens for Sprites and/or Modal when you start using provider commands

## Install (from source)

```bash
go install ./cmd/sbx
```

## Configuration

Configuration is layered (highest priority first):

1. CLI flags (`--provider`, `--config`)
2. Environment variables with prefix `SBX_` (dots become underscores, e.g. `SBX_SPRITES_TOKEN`)
3. Config file

Default config file locations (first existing file wins):

- `$XDG_CONFIG_HOME/sbx/config.yaml`
- `$HOME/.config/sbx/config.yaml`

See [config.example.yaml](config.example.yaml). Never commit real tokens.

### Environment variables

| Variable | Description |
| --- | --- |
| `SBX_PROVIDER` | Default provider (`sprites` or `modal`) |
| `SBX_SPRITES_TOKEN` | Sprites API token |
| `SBX_MODAL_TOKEN_ID` | Modal token ID |
| `SBX_MODAL_TOKEN_SECRET` | Modal token secret |

## Usage

```bash
sbx --help
sbx version
sbx --provider sprites --help
```

## Development

Run `make help` for all targets. Quick local build:

```bash
make build
./bin/sbx --help
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
