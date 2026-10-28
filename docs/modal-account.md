# Modal account and token

kaxa talks to [Modal](https://modal.com) with an API token made of two parts: a token ID and a token secret. Both live in the environment. kaxa does not read `modal token new`, `MODAL_TOKEN_ID`, `MODAL_TOKEN_SECRET`, or `~/.modal.toml`.

## Create an account

1. Open [modal.com](https://modal.com) and choose **Sign up**.
2. Sign in with GitHub, Google, or email, and finish the workspace setup in the browser.

A new workspace starts on the Starter plan. Starter has no monthly fee and includes US$30 of compute each month. Sandboxes are billed by the second for CPU and memory, at the non-preemptible rate. See [Modal pricing](https://modal.com/pricing).

## Add a card for the US$30 credit

1. Open [modal.com/settings](https://modal.com/settings) and go to **Billing**.
2. Add a credit card as the payment method.
3. Optional: set a workspace budget so a forgotten sandbox cannot spend more than you accept.

Usage beyond the included US$30 is charged to the card. kaxa creates sandboxes with a 24 hour lifetime and a 1 hour idle timeout, but destroy them with `kaxa destroy` when you are done.

## Create a token

1. Open [modal.com/settings/tokens](https://modal.com/settings/tokens).
2. Choose **New Token**.
3. Copy both values the page shows:
   - **Token ID**, which starts with `ak-`.
   - **Token secret**, which starts with `as-`. Modal shows it once.

Treat both strings as secrets. Do not commit them, put them in a flag, or paste them into an issue or pull request.

## Configure kaxa

```bash
export KAXA_MODAL_TOKEN_ID='ak-paste-the-token-id'
export KAXA_MODAL_TOKEN_SECRET='as-paste-the-token-secret'
export KAXA_DEFAULT_PROVIDER=modal
```

`KAXA_DEFAULT_PROVIDER` is optional. You can pass `--provider modal` on each command instead. Both variables are required: kaxa rejects the provider when either one is empty.

Check that the token works:

```bash
kaxa list --provider modal
```

Without `--provider`, `kaxa list` includes Modal once both variables are set.

An empty workspace prints `no sandboxes created`. A rejected token prints an error and does not create anything.

## What works on Modal

Modal supports `create`, `list`, `exec`, `console`, and `destroy`, and the detail view in `list`. Sprites-only commands such as `restart`, `upgrade`, and `url` print that the feature is not available for Modal and name the providers that have it. `create --sync` copies your repository to `/home/modal/workspace/<repo>` and installs its toolchain; see [create](create.md#sync-on-modal). `--running`, API publish, and public URLs are not available for Modal. Run commands with `kaxa --provider modal exec <name> ...` or set `KAXA_DEFAULT_PROVIDER=modal`; see [exec](exec.md) and [console](console.md) for what differs on Modal.

Command manuals are in the [docs index](README.md).
