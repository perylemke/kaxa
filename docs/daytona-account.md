# Daytona account and API key

The `daytona` provider runs sandboxes on [Daytona](https://www.daytona.io). kaxa uses the official Daytona Go SDK (`https://app.daytona.io/api`), with one API key. It lives in the environment. kaxa does not read `DAYTONA_API_KEY`, the Daytona CLI login, or any config file.

## Create an account

1. Open [daytona.io](https://www.daytona.io) and sign up in the dashboard.
2. Check the plan and credits in the dashboard. Sandboxes are billed while they run, and a stopped sandbox keeps only its disk. Daytona bills the account, not kaxa.

## Create an API key

1. Open the [Daytona dashboard](https://app.daytona.io) and go to **API Keys**.
2. Create a key and copy it. Daytona shows it once.
3. Give it the scopes to create, read, start, stop, and delete sandboxes, and to manage images if you use `snapshot`.

Treat the key as a secret. Do not commit it, put it in a flag, or paste it into an issue or pull request.

## Configure kaxa

```bash
export KAXA_DAYTONA_API_KEY='paste-the-key'
export KAXA_DEFAULT_PROVIDER=daytona
```

`KAXA_DEFAULT_PROVIDER` is optional. You can pass `--provider daytona` on each command instead. kaxa rejects the provider when the key is empty, and keeps the key out of output and error messages.

Check that the key works:

```bash
kaxa list --provider daytona
```

Without `--provider`, `kaxa list` includes Daytona once the key is set. An empty account prints `no sandboxes created`. A rejected key prints a message that Daytona rejected the credentials and creates nothing.

## What works on Daytona

`create` (with `--sync`, `--template`, `--api`, and `--block-all-network-access`), `list`, `exec`, `console`, `restart`, `sync`, `snapshot`, `destroy`, and the detail view in `list`. `create --sync` copies your repository to `/home/daytona/workspace/<repo>` and installs its toolchain; see [create](create.md#sync-on-daytona). `create --api` publishes one HTTP service; see [create](create.md#api-on-daytona). `--block-all-network-access` blocks every outbound connection; a toolchain download and `go mod download` then fail, while `--no-install` or a create without `--sync` still works. `snapshot` saves the disk as an image and restores it into a new sandbox; see [snapshot](snapshot.md#daytona). `--running`, public URLs, `url`, and `upgrade` are not available; kaxa names the providers that have them.

## Templates

Set `KAXA_DAYTONA_TEMPLATE` (or pass `--template` to `create`) to start sandboxes from a Daytona snapshot you created. It must keep the layout of the Daytona default image: user `daytona`, home `/home/daytona`. Another user breaks `exec`, `console`, and `--sync`, and kaxa does not detect that. The create wizard lists the active platform snapshots; Provider default leaves the image empty. See [create](create.md#templates-on-daytona).

## How sandboxes live

- kaxa creates persistent sandboxes: files stay when Daytona stops one. Running processes, including a service published with `--api`, do not survive a stop.
- A stopped sandbox is started, and a paused one resumed, by the next `exec`, `console`, `sync`, or `restart`. `restart` stops the sandbox and starts it again on purpose.
- kaxa stores the sandbox name, labels, API port, and template as labels on the Daytona sandbox. Daytona generates its own sandbox ID, so `list` shows the kaxa name and `destroy` finds the sandbox by that name. kaxa only lists and destroys sandboxes it created. Names are DNS labels: lowercase letters, digits, and `-`.
- `exec` and `console` use the SDK process API. The service takes one command string, so kaxa quotes every word and nothing you type is parsed by a shell; use `sh -c` for pipes. Standard input is staged as a file (up to 64 MiB). `create --api` makes the whole sandbox public, not one port.
- `destroy` deletes the sandbox and everything in it. It does not delete snapshots.

## Snapshots

`snapshot create` captures the disk of the sandbox as a Daytona image named `kaxa-<sandbox>-<timestamp>`. That name is how kaxa tells its images from yours. Daytona has no field for the comment or for the source labels, so kaxa keeps them in `daytona-snapshots.json` under your user config directory (`~/Library/Application Support/kaxa` on macOS, `$XDG_CONFIG_HOME/kaxa` or `~/.config/kaxa` on Linux, `%AppData%\kaxa` on Windows; directory mode `0700`, file `0600`). Without that file `list` still shows the snapshots, without their comments. Images count against your Daytona storage until you run `snapshot delete`.

## Sandbox URLs

Sandbox ports are private by default. Only `create --api` makes the API port reachable, with a public preview URL that Daytona chooses. Anyone who has the URL can call the service. Destroy the sandbox when you are done.

Command manuals are in the [docs index](README.md).
