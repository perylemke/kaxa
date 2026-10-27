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

On a sandbox created with `--sync`, the shell starts in `/home/sprite/workspace`. Other sandboxes start in the sprite home.
