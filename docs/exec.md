# kaxa exec

Run a command in a sandbox. This works for Sprites and Modal.

```bash
kaxa exec <name> <command> [args...]
```

`exec` always runs the command you pass. For an interactive shell, use [kaxa console](console.md).

```bash
kaxa exec my-sprite -- ls -la
```

The remote exit code is the kaxa exit code, from 0 through 255.

On a sandbox created with `--sync`, the command starts in `/home/sprite/workspace`. Other sandboxes start in the sprite home. On Modal, a sandbox created with `--sync` runs the command as the `modal` user in `/home/modal/workspace`.

## Modal

Pick the provider with `--provider modal` or `KAXA_DEFAULT_PROVIDER=modal`. The sandbox must be running; a finished sandbox is reported as not found.

```bash
kaxa --provider modal exec my-box -- ls -la
kaxa --provider modal exec my-box -- sh -c "echo hi; echo oops >&2"
```

Standard output, standard error, and standard input are passed through, and the remote exit code is the kaxa exit code. A sandbox created without `--sync` runs the command as root in the image's default directory (the root of `debian:bookworm-slim`). With `--sync`, it runs as the `modal` user in `/home/modal/workspace`; see [create](create.md#sync-on-modal).
