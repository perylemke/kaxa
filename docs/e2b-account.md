# E2B account and API key

kaxa talks to [E2B](https://e2b.dev) with one API key. It lives in the environment. kaxa does not read `E2B_API_KEY`, the E2B CLI login, or any config file.

## Create an account

1. Open [e2b.dev](https://e2b.dev) and choose **Sign up**.
2. Sign in with GitHub or email and finish the setup in the browser.

A new account gets a one-time usage credit and no monthly fee on the Hobby plan. Sandboxes are billed by the second while they run, and a paused sandbox is not billed for compute. See [E2B pricing](https://e2b.dev/pricing).

## Create an API key

1. Open the [E2B dashboard](https://e2b.dev/dashboard) and go to **Keys**.
2. Create a key and copy it. It starts with `e2b_`.

Treat the key as a secret. Do not commit it, put it in a flag, or paste it into an issue or pull request.

## Configure kaxa

```bash
export KAXA_E2B_API_KEY='e2b_paste-the-key'
export KAXA_DEFAULT_PROVIDER=e2b
```

`KAXA_DEFAULT_PROVIDER` is optional. You can pass `--provider e2b` on each command instead. kaxa rejects the provider when the key is empty.

Check that the key works:

```bash
kaxa list --provider e2b
```

Without `--provider`, `kaxa list` includes E2B once the key is set. An empty account prints `no sandboxes created`. A rejected key prints `E2B rejected the API key` and creates nothing.

## What works on E2B

E2B supports `create`, `list`, `exec`, `console`, `restart`, `sync`, `destroy`, and the detail view in `list`. `create --sync` copies your repository to `/home/user/workspace/<repo>` and installs its toolchain; see [create](create.md#sync-on-e2b). `create --api` publishes one HTTP service; see [create](create.md#api-on-e2b). `url`, `upgrade`, `snapshot`, `--running`, and public URLs are not available for E2B, and kaxa names the providers that have them.

## Templates

Set `KAXA_E2B_TEMPLATE` (or pass `--template` to `create`) to start sandboxes from a template you built with the E2B CLI or SDK, for example one with more memory than the 512 MiB of `base`. It must keep the `base` layout (user `user`). See [create](create.md#templates-on-e2b).

## How sandboxes live

- The `base` template has 512 MiB of memory, and you cannot raise it per sandbox. Large builds can be killed with `signal: killed`. On `base`, `create --sync` therefore downloads Go and Rust dependencies without compiling them; use a larger template to build.

- A sandbox lives for one hour after the last command, then E2B pauses it. A paused sandbox keeps its filesystem and appears in `list` as `paused`. The next `exec`, `console`, `sync`, or `restart` resumes it and extends the hour.
- kaxa stores the sandbox name, labels, and API port as metadata on the E2B sandbox. E2B generates its own sandbox ID, so `list` shows the kaxa name and `destroy` finds the sandbox by that name.
- `restart` pauses the sandbox and boots it again from its disk. Files stay. Running processes, including a published API, do not survive; run `create --api` again, or start the service with `exec`.
- Names are DNS labels: lowercase letters, digits, and `-`.

## Sandbox URLs

Sandbox ports are private by default. Only `create --api` makes the sandbox reachable from the internet, and then only on the API port (8000 by default), with the URL `https://<port>-<sandbox id>.e2b.app`. E2B fixes that choice when the sandbox is created, so `publish_api` on an existing sandbox must use the same port. Anyone who has the URL can call the service. Destroy the sandbox when you are done.

Command manuals are in the [docs index](README.md).
