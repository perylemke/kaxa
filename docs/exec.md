# kaxa exec

Run a command in a sandbox. This works for Sprites, Modal, E2B, and Docker.

```bash
kaxa exec <name> <command> [args...]
```

`exec` always runs the command you pass. For an interactive shell, use [kaxa console](console.md).

```bash
kaxa exec my-sprite -- ls -la
```

The remote exit code is the kaxa exit code, from 0 through 255.

On a sandbox created with `--sync`, the command starts in `/home/sprite/workspace`. Other sandboxes start in the sprite home. On Modal, a sandbox created with `--sync` runs the command as the `modal` user in `/home/modal/workspace`.

## E2B

Pick the provider with `--provider e2b` or `KAXA_DEFAULT_PROVIDER=e2b`. A paused sandbox is resumed first, which takes a second or two.

```bash
kaxa --provider e2b exec my-box -- ls -la
```

Standard output, standard error, and standard input are passed through, and the remote exit code is the kaxa exit code. Commands run as `user`, starting in `/home/user/workspace` with `--sync` and in `/home/user` without it. The command is not run through a shell; use `sh -c` for pipes.

## Docker

Pick the provider with `--provider docker` or `KAXA_DEFAULT_PROVIDER=docker`. kaxa starts a stopped sandbox, extends its time to live, and runs the command through the process API of the sandbox.

```bash
kaxa --provider docker exec my-box -- ls -la
```

Standard output, standard error, and standard input are passed through, and the remote exit code is the kaxa exit code. Commands run as `agent`, starting in `/home/agent/workspace` with `--sync` and where the image starts without it. The command is not run through a shell; use `sh -c` for pipes. Cloud compute is billed to your Docker subscription while the sandbox runs.

## Modal

Pick the provider with `--provider modal` or `KAXA_DEFAULT_PROVIDER=modal`. The sandbox must be running; a finished sandbox is reported as not found.

```bash
kaxa --provider modal exec my-box -- ls -la
kaxa --provider modal exec my-box -- sh -c "echo hi; echo oops >&2"
```

Standard output, standard error, and standard input are passed through, and the remote exit code is the kaxa exit code. A sandbox created without `--sync` runs the command as root in the image's default directory (the root of `debian:bookworm-slim`). With `--sync`, it runs as the `modal` user in `/home/modal/workspace`; see [create](create.md#sync-on-modal).
