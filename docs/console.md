# kaxa console

Open an interactive bash shell in a sandbox. `ssh` is an alias.

```bash
kaxa console <name>
kaxa ssh <name>
```

The shell requires a terminal. Without a terminal, run a command with [kaxa exec](exec.md).

```bash
kaxa console my-sprite
kaxa ssh my-sprite
```

The remote exit code is the kaxa exit code, from 0 through 255.

On a sandbox created with `--sync`, the shell starts in `/home/sprite/workspace`. Other sandboxes start in the sprite home. On Modal, a sandbox created with `--sync` opens the shell as the `modal` user in `/home/modal/workspace`.

## E2B

Pick the provider with `--provider e2b`. The shell runs as `user` with the terminal size of your window, and resizing the window is applied while the session is open. A paused sandbox is resumed first. E2B merges standard error into standard output in a terminal session.

```bash
kaxa --provider e2b console my-box
```

## Modal

Pick the provider with `--provider modal` or `KAXA_DEFAULT_PROVIDER=modal`. The sandbox must be running.

```bash
kaxa --provider modal console my-box
```

Differences from Sprites:

- The terminal size is applied when the shell starts. Resizing the window during the session is not applied, because the Modal SDK cannot resize a running terminal. Run `stty rows <n> cols <m>` in the shell, or reopen the console, after resizing.
- Modal merges standard error into standard output in a terminal session.
- Without `--sync` the shell runs as root in the root directory of the sandbox image. With `--sync` it runs as `modal` in `/home/modal/workspace`.

## Docker

Pick the provider with `--provider docker`. kaxa opens a process with a terminal in the sandbox, sizes it from your window and follows resizing. The shell runs as `agent`. A stopped sandbox is started first.

```bash
kaxa --provider docker console my-box
```

