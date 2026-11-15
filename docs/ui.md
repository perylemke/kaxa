# kaxa ui

Open a local page to manage sandboxes.

```bash
kaxa ui
kaxa ui --no-open
kaxa ui --port 4000
kaxa ui --provider sprites
```

The server listens on `127.0.0.1`. With no `--port`, the operating system picks a free port. The address is printed on stderr and opened in the browser. `--port` chooses the port and fails if it is already in use. `--no-open` skips the browser. Ctrl+C stops the server.

`--provider` limits the page to that provider, the same way it limits the other commands. Without it, the page lists every provider. A provider that is not available is shown disabled and cannot create a sandbox. `KAXA_DEFAULT_PROVIDER` selects the default option in the create form. It does not hide the other providers.

The page lists sandboxes and opens one for its details. From the detail view you can restart, upgrade, change URL auth, and destroy, when that provider supports the action. Restart, upgrade, and URL auth stay hidden otherwise. Destroy asks for confirmation on the sandbox page and names the sandbox being deleted.

Create asks for the name and provider, then the running, sync, and API options that provider supports. Sync copies one or more Git repositories from this machine; an empty directories field uses the working directory of `kaxa ui`. The API command is comma-separated argv, as in `kaxa create --api-cmd`. Validation matches [create](create.md). Template, CPU, memory, disk, network block, and sandbox environment variables stay on the `create` command.

Provider tokens stay in the `kaxa ui` process. They are not sent to the browser, written into the address, or included in errors. A request whose `Host` header is not the printed address is rejected.
