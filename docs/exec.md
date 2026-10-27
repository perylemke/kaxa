# sbox exec

Run a command in a sandbox.

```bash
sbox exec <name> <command> [args...]
```

`exec` always runs the command you pass. For an interactive shell, use [sbox console](console.md).

```bash
sbox exec my-sprite -- ls -la
```

The remote exit code is the sbox exit code, from 0 through 255.

On a sandbox created with `--sync`, the command starts in `/home/sprite/workspace`. Other sandboxes start in the sprite home.
