# Blaxel account and API key

The `blaxel` provider runs sandboxes on [Blaxel](https://blaxel.ai). kaxa uses the official Blaxel Go SDK (`https://api.blaxel.ai/v0`), with an API key and a workspace name. Both live in the environment. kaxa does not read `BL_API_KEY`, `BL_WORKSPACE`, `~/.blaxel/config.yaml`, or the Blaxel CLI login.

## Create an account

1. Open [blaxel.ai](https://blaxel.ai) and sign up.
2. Create a workspace, or use the one the dashboard already made. Sandboxes are billed to that workspace while they run. Blaxel bills the account, not kaxa.

## Create an API key

1. Open the Blaxel dashboard for the workspace and create an API key.
2. Copy the key and the workspace name. The key is shown once.

Treat the key as a secret. Do not commit it, put it in a flag, or paste it into an issue or pull request. The workspace name is not a secret.

## Configure kaxa

```bash
export KAXA_BLAXEL_API_KEY='paste-the-key'
export KAXA_BLAXEL_WORKSPACE='your-workspace'
export KAXA_DEFAULT_PROVIDER=blaxel
```

`KAXA_DEFAULT_PROVIDER` is optional. You can pass `--provider blaxel` on each command instead. kaxa rejects the provider when the key or the workspace is empty, and keeps the key out of output and error messages.

Optional settings:

| Variable | Meaning |
| --- | --- |
| `KAXA_BLAXEL_TEMPLATE` | Image for new sandboxes. Default `blaxel/base-image:latest`. `create --template` overrides it |
| `KAXA_BLAXEL_REGION` | Region, such as `us-pdx-1` (the default) or `eu-lon-1` |
| `KAXA_BLAXEL_MEMORY` | Memory in megabytes. Default `4096`. The writable layer is memory, so a large repository may need more |
| `KAXA_BLAXEL_DISK_PERCENT` | Share of that memory for the filesystem, from 10 to 95. Empty leaves the API default of 50. `create --disk-percent` overrides it for that sandbox. The value is fixed at create and applies to the mk3.1 generation |

Check that the credentials work:

```bash
kaxa list --provider blaxel
```

Without `--provider`, `kaxa list` includes Blaxel once the key and workspace are set. An empty account prints `no sandboxes created`. A rejected key prints a message that Blaxel rejected the credentials and creates nothing.

## What works on Blaxel

`create` (with `--sync`, `--template`, `--api`, and `--disk-percent`), `list`, `exec`, `console`, `sync`, `snapshot`, `unarchive`, `destroy`, and the detail view in `list`. `create --sync` copies your repository to `/root/workspace/<repo>` and installs its toolchain; see [create](create.md#sync-on-blaxel). `create --api` publishes one HTTP service on a private preview unless `--public` is set; see [create](create.md#api-on-blaxel). `snapshot` saves the filesystem and restores the same sandbox; see [snapshot](snapshot.md). `unarchive` brings an archived sandbox back; see [unarchive](unarchive.md). `console` attaches to the terminal the sandbox starts; see [console](console.md#blaxel). `restart`, `upgrade`, `url`, and `--running` are not available; kaxa names the providers that have them. Standby is not a restart.

## Templates

Set `KAXA_BLAXEL_TEMPLATE` (or pass `--template` to `create`) to start sandboxes from another image. It must keep the layout of `blaxel/base-image:latest`: user `root`, home `/root`. Another user breaks `exec` and `--sync`, and kaxa does not detect that. The create wizard lists the visible Hub images; Provider default leaves the image empty.

## How sandboxes live

- A sandbox that is deployed, including one in standby, is `running`. Standby keeps files and processes and wakes on the next call. That resume is not a snapshot restore, and kaxa does not restart a sandbox.
- An archived sandbox stays archived until you run `kaxa unarchive`. `list` shows it. `exec`, `console`, and `sync` fail and tell you to unarchive it. Archive keeps the filesystem and starts processes again with new IDs.
- A failed sandbox is an error. Create deletes a sandbox that does not become ready.
- The writable layer is a tmpfs inside the memory from `KAXA_BLAXEL_MEMORY`. `KAXA_BLAXEL_DISK_PERCENT` is the share of that memory used by the filesystem (10–95; empty uses 50). A large repository can fail to unpack until you raise the memory or the disk share. The disk share is set at create and cannot be changed later.
- kaxa stores the sandbox name, labels, API port, and image as labels. It only lists and destroys sandboxes it created (`kaxa.managed=true`). Names are DNS labels of at most 49 characters: lowercase letters, digits, and `-`.
- `exec` quotes every argument, so a space, a quote, or `$(...)` stays one argument. Use `sh -c` when you want a pipe. Standard input is sent only when it is not a terminal. The toolchain `PATH` is set. With `--sync`, the command starts in `/root/workspace`; otherwise it starts in `/root`.
- `destroy` deletes the sandbox and everything in it.

## Sandbox URLs

Ports 80, 443, and 8080 belong to the sandbox. Pick another `--api-port`. The process must listen on `0.0.0.0` and on that port. Blaxel also accepts `$HOST` or `::`.

`create --api` opens a private preview named `kaxa-api`. The URL is printed, and the preview token is printed once. Without the token the URL does not open. The token is not stored on the sandbox, and `inspect` and `list` do not show it. `--public` makes that preview public and prints no token. Anyone who has a public URL can call the service. Destroy the sandbox when you are done.

Command manuals are in the [docs index](README.md).
