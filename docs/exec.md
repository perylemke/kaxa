# sbox exec

Run a shell or a command in a sandbox.

```bash
sbox exec <name>
sbox exec <name> [command...]
```

With only the name, exec opens an interactive `bash`. That requires a terminal. Without a terminal, pass a command.

```bash
sbox exec my-sprite
sbox exec my-sprite -- ls -la
```

The remote exit code is the sbox exit code, from 0 through 255.

On a sandbox created with `--sync`, the command starts in `/home/sprite/workspace`. Other sandboxes start in the sprite home.
