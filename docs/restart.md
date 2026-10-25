# sbox restart

Restart a sandbox.

```bash
sbox restart [name]
```

With a name, restart asks `[y/N]` and then restarts that sandbox. Answering no prints `restart cancelled` and does not call the provider.

Without a name, restart opens a picker. Choose a number or a name from the list, confirm `[y/N]`, and the sandbox restarts. `q` cancels the picker.

```bash
sbox restart my-sprite
sbox restart
```
