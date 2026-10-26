# sbox console

Open an interactive bash shell in a sandbox. `ssh` is an alias.

```bash
sbox console <name>
sbox ssh <name>
```

The shell requires a terminal. Without a terminal, run a command with [sbox exec](exec.md).

```bash
sbox console my-sprite
sbox ssh my-sprite
```

The remote exit code is the sbox exit code, from 0 through 255.

On a sandbox created with `--sync`, the shell starts in `/home/sprite/workspace`. Other sandboxes start in the sprite home.
