# Vercel account and access token

The `vercel` provider runs sandboxes on [Vercel Sandbox](https://vercel.com/docs/sandbox). Vercel has no Go SDK, so kaxa calls the REST API (`https://api.vercel.com`) with its own client. It needs an access token and a project. Both live in the environment. kaxa does not read `VERCEL_TOKEN`, `.vercel/`, or the Vercel CLI login.

## Create an account

1. Open [vercel.com](https://vercel.com) and sign up.
2. Create a project, or pick an existing one. Sandboxes belong to a project and are billed to its team while they run. Vercel bills the account, not kaxa.

## Create an access token

1. In the Vercel dashboard, open Account Settings, then Tokens, and create a token scoped to the team that owns the project.
2. Copy the token. It is shown once.

Treat the token as a secret. Do not commit it, put it in a flag, or paste it into an issue or pull request. The token goes only in the `Authorization` header and is kept out of output and error messages.

## Configure kaxa

```bash
export KAXA_VERCEL_TOKEN='paste-the-token'
export KAXA_VERCEL_PROJECT='your-project'
export KAXA_VERCEL_TEAM='team_xxx'      # or the team slug; optional for a personal account
export KAXA_DEFAULT_PROVIDER=vercel
```

`KAXA_DEFAULT_PROVIDER` is optional. You can pass `--provider vercel` on each command instead. kaxa rejects the provider when the token or the project is empty.

| Variable | Meaning |
| --- | --- |
| `KAXA_VERCEL_TOKEN` | Access token |
| `KAXA_VERCEL_PROJECT` | Project name or ID that owns the sandboxes |
| `KAXA_VERCEL_TEAM` | Team ID (`team_...`) or slug. Optional |
| `KAXA_VERCEL_TEMPLATE` | Snapshot ID to start new sandboxes from. Optional. `create --template` overrides it |

Check that the credentials work:

```bash
kaxa list --provider vercel
```

An empty project prints `no sandboxes created`. A rejected token prints a message that Vercel rejected the credentials.

## What works on Vercel

`create` (with `--sync`, `--template`, `--api`, `--cpu`, and `--block-all-network-access`), `list`, `exec`, `console` (also `ssh`), `sync`, `restart`, `snapshot`, `destroy`, and the detail view in `list`. `upgrade`, `url`, `--public`, and `--running` are not available. See [console](console.md#vercel).

## How sandboxes live

- Sandboxes are named and persistent. Stopping one saves its filesystem and resuming restores it, but running processes do not survive a stop. `exec` and `sync` resume a stopped sandbox.
- A resume (the first `exec` after a stop, or a snapshot restore) can take several minutes when Vercel has not cached the snapshot. kaxa waits up to 10 minutes for it.
- A session lasts 30 minutes and kaxa extends it when less than 10 minutes remain.
- The workspace is `/vercel/sandbox/workspace/<repo>`, as user `vercel-sandbox`. Without `--sync`, commands start in the home directory.
- kaxa tags every sandbox it creates (`kaxa-managed`) and only lists and destroys those. Names are lowercase letters, digits, `-`, and `_`, up to 63 characters, starting with a letter or digit.
- `exec` passes each argument as a separate argv item, so a space, a quote, or `$(...)` stays one argument. Use `sh -c` for pipes. The toolchain `PATH` is set.
- `console` (and `ssh`) opens the terminal Vercel hosts, over a secure WebSocket. It is not SSH and needs no key.
- `destroy` deletes the sandbox and its automatic snapshots. Manual snapshots stay until you delete them.

## Snapshots

`snapshot create` saves the filesystem as a manual snapshot, and `snapshot restore` returns the same sandbox to it, in place. Comments live in an index on this machine. See [snapshot](snapshot.md).

## Sandbox URLs

`create --api` publishes one HTTP service on a Vercel-hosted URL for the port. The port is declared at create, so a new one restarts the sandbox once. The process must listen on `0.0.0.0` and on that port. Port `23456` is reserved. kaxa waits up to 90 seconds for the port to listen, but cannot tell a crash from a slow start, so a failure shows as a timeout. Anyone who has the URL can call the service. Destroy the sandbox when you are done.

Command manuals are in the [docs index](README.md).
