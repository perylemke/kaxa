# Sprites account and token

sbox talks to [Sprites](https://sprites.dev) with a bearer token. The token lives in the environment. sbox does not read `sprite login` or `~/.sprites/sprites.json`.

## Create an account

Sprites uses a Fly.io account.

1. Open [sprites.dev](https://sprites.dev) and sign in with Fly.io, or use the [Sprites signup](https://fly.io/app/sign-up?s=sprites).
2. Finish account setup in the browser. A new organization can create sprites for testing.

The upstream `sprite` CLI is optional. Its login flow is documented in [Sprites authentication](https://docs.fly.io/sprites/cli/authentication). That login does not configure sbox.

## Create a token

1. Open [sprites.dev/account](https://sprites.dev/account).
2. Create an API token.
3. Copy the value the page shows. Treat that string as a secret.

Do not commit the token, put it in a flag, or paste it into an issue or pull request.

## Configure sbox

```bash
export SBOX_SPRITES_TOKEN='paste-the-dashboard-value'
export SBOX_DEFAULT_PROVIDER=sprites
```

`SBOX_DEFAULT_PROVIDER` is optional. You can pass `--provider sprites` on each command instead.

Check that the token works:

```bash
sbox list
```

An empty account prints `no sandboxes created`. A rejected token prints an error and does not create anything.

Command manuals are in the [docs index](README.md).
