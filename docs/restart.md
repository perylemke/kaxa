# kaxa restart

Restart a sandbox.

```bash
kaxa restart [name]
```

With a name, restart asks `[y/N]` and then restarts that sandbox. Answering no prints `restart cancelled` and does not call the provider.

Without a name, restart opens a picker. Choose a number or a name from the list, confirm `[y/N]`, and the sandbox restarts. `q` cancels the picker.

```bash
kaxa restart my-sprite
kaxa restart
```

Restart is not available for Modal: a Modal sandbox cannot be rebooted in place. kaxa says so and lists the providers that support it, such as Sprites.
