# kaxa upgrade

Upgrade a sandbox.

```bash
kaxa upgrade [name]
```

With a name, upgrade asks `[y/N]` and then upgrades that sandbox. Answering no prints `upgrade cancelled` and does not call the provider.

Without a name, upgrade opens a picker. Choose a number or a name from the list, confirm `[y/N]`, and the sandbox upgrades. `q` cancels the picker.

```bash
kaxa upgrade my-sprite
kaxa upgrade
```

Upgrade is not available for Modal or E2B: neither has a per-sandbox platform upgrade. kaxa says so and lists the providers that support it, such as Sprites.
