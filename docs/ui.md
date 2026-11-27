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

Create asks for the name and provider, then the running, sync, and API options that provider supports. The name is checked as you type, with the same rule as `kaxa create`. Sync copies one or more Git repositories from this machine. Choose repository opens a folder browser on the page, starting at the working directory of `kaxa ui`; Git repositories are marked, and each one you add appears in a list you can edit. With none chosen, the working directory is copied. The browser only shows folders inside your home directory, and the server refuses any other path. The API command is comma-separated argv, as in `kaxa create --api-cmd`. Validation matches [create](create.md). Template, CPU, memory, disk, network block, and sandbox environment variables stay on the `create` command.

Once a create is accepted, the page lists each step as the server runs it: creating the sandbox, copying the repository, installing dependencies, and publishing the API. On success the page opens the sandbox. A panel there shows the preview token once, if the provider issues one, with a Copy button. On failure the page shows the error and the step that failed. Edit and retry keeps what you typed. Keep the tab open while a create runs: closing it cancels the request, and a failed copy, install, or publish removes the sandbox.

## Navigation

Each view has its own address, so the back button, reload, and a pasted link work:

| Address | View |
| --- | --- |
| `#/` | The sandbox list |
| `#/new` | Create a sandbox |
| `#/sandboxes/{provider}/{name}` | One sandbox |

The list shows when it was last updated and has a Refresh button. With more than one provider, or more than eight sandboxes, it also offers a search by name and a provider filter. Restart, upgrade, and URL auth report their result next to the button and in a short message.

## Log

`kaxa ui` writes one line to stderr for each create step, for each action that changes a sandbox, and for each error a provider returns. A line has the time, a level, the action, `provider/sandbox`, and the detail:

```text
14:02:30  info   create    sprites/teste-ui          started
14:02:30  info   create    sprites/teste-ui          creating teste-ui on sprites…
14:03:05  info   create    sprites/teste-ui          done in 35.1s
14:04:10  error  restart   e2b/api-1                 sandbox not found
14:04:12  warn   list      -                         daytona: <message from the provider>
14:05:00  warn   request   -                         missing csrf token
```

Reading the list or one sandbox does not log unless it fails. Requests rejected for their `Host`, session, or CSRF token log a `warn` line without the value they sent. Provider tokens, the session token, and the CSRF token never appear in the log.

The folder browser lists names of folders under your home directory, never file contents. Symlinks that leave your home directory are hidden and refused.

Provider tokens stay in the `kaxa ui` process. They are not sent to the browser, written into the address, or included in errors. A request whose `Host` header is not the printed address is rejected.
