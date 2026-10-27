# kaxa exec

Run a command in a sandbox.

```bash
kaxa exec <name> <command> [args...]
```

`exec` always runs the command you pass. For an interactive shell, use [kaxa console](console.md).

```bash
kaxa exec my-sprite -- ls -la
```

The remote exit code is the kaxa exit code, from 0 through 255.

On a sandbox created with `--sync`, the command starts in `/home/sprite/workspace`. Other sandboxes start in the sprite home.
