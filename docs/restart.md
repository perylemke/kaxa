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

Restart is not available for Modal: a Modal sandbox cannot be rebooted in place. kaxa says so and lists the providers that support it, such as Sprites, E2B, and Docker.

On E2B, restart pauses the sandbox and boots it again from its disk. Files stay; running processes, including a service published with `--api`, stop. Start the service again with `exec`. See [E2B account and API key](e2b-account.md).

On Docker, restart stops the sandbox through the API and starts it again. Files stay; running processes, including a service published with `--api`, stop. Start the service again with `exec`. See [Docker Sandboxes](docker-account.md).
