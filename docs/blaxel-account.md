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

Check that the credentials work:

```bash
kaxa list --provider blaxel
```

Without `--provider`, `kaxa list` includes Blaxel once the key and workspace are set. An empty account prints `no sandboxes created`. A rejected key prints a message that Blaxel rejected the credentials and creates nothing.

## What works on Blaxel

`create` (with `--sync`, `--template`, and `--api`), `list`, `exec`, `console`, `sync`, `destroy`, and the detail view in `list`. `create --sync` copies your repository to `/root/workspace/<repo>` and installs its toolchain; see [create](create.md#sync-on-blaxel). `create --api` publishes one HTTP service on a public preview; see [create](create.md#api-on-blaxel). `console` attaches to the terminal the sandbox starts; see [console](console.md#blaxel). `restart`, `snapshot`, `upgrade`, `url`, and `--running` are not available; kaxa names the providers that have them.

## Templates

Set `KAXA_BLAXEL_TEMPLATE` (or pass `--template` to `create`) to start sandboxes from another image. It must keep the layout of `blaxel/base-image:latest`: user `root`, home `/root`. Another user breaks `exec` and `--sync`, and kaxa does not detect that. The create wizard lists the visible Hub images; Provider default leaves the image empty.

## How sandboxes live

- A sandbox that is deployed, including one in standby, is `running`. Standby keeps files and processes and wakes on the next call. kaxa does not restart a sandbox.
- An archived sandbox stays archived. `list` shows it. `exec`, `console`, and `sync` fail and tell you to unarchive it in the Blaxel console, or to create a new sandbox.
- A failed sandbox is an error. Create deletes a sandbox that does not become ready.
- The writable layer is a tmpfs inside the memory from `KAXA_BLAXEL_MEMORY`. A large repository can fail to unpack until you raise that value.
- kaxa stores the sandbox name, labels, API port, and image as labels. It only lists and destroys sandboxes it created (`kaxa.managed=true`). Names are DNS labels of at most 49 characters: lowercase letters, digits, and `-`.
- `exec` quotes every argument, so a space, a quote, or `$(...)` stays one argument. Use `sh -c` when you want a pipe. Standard input is sent only when it is not a terminal. The toolchain `PATH` is set. With `--sync`, the command starts in `/root/workspace`; otherwise it starts in `/root`.
- `destroy` deletes the sandbox and everything in it.

## Sandbox URLs

Ports 80, 443, and 8080 belong to the sandbox. Pick another `--api-port`. The process must listen on `0.0.0.0` and on that port. Blaxel also accepts `$HOST` or `::`.

`create --api` opens a public preview named `kaxa-api`. Anyone who has the URL can call the service. There is no private preview and no token on the URL. Destroy the sandbox when you are done.

Command manuals are in the [docs index](README.md).
