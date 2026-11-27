# kaxa ui

Open a local page to manage sandboxes.

```bash
kaxa ui
kaxa ui --no-open
kaxa ui --port 4000
kaxa ui --provider sprites
```

The server listens on `127.0.0.1`. With no `--port`, the operating system picks a free port. The address is printed on stderr and opened in the browser. `--port` chooses the port and fails if it is already in use. `--no-open` skips the browser. Ctrl+C stops the server gracefully: it closes the open terminals, which ends their shells, cancels the creates in progress so they clean up, and logs what it closed. It waits up to 30 seconds. A second Ctrl+C stops it at once.

`--provider` limits the page to that provider, the same way it limits the other commands. Without it, the page lists every provider. A provider that is not available is shown disabled and cannot create a sandbox. `KAXA_DEFAULT_PROVIDER` selects the default option in the create form. It does not hide the other providers.

The page lists sandboxes and opens one for its details. From the detail view you can restart, upgrade, change URL auth, and destroy, when that provider supports the action. Restart, upgrade, and URL auth stay hidden otherwise. Destroy asks for confirmation on the sandbox page and names the sandbox being deleted. When the provider accepts the destroy but still lists the sandbox as stopped, as Blaxel does while it finishes the delete, the page says so: "still listed with status stopped; the provider may finish removing it".

Create asks for the name and provider, then the running, sync, and API options that provider supports. The name is checked as you type, with the same rule as `kaxa create`. Sync copies one or more Git repositories from this machine. Choose repository opens a folder browser on the page, starting at the working directory of `kaxa ui`; Git repositories are marked, and each one you add appears in a list you can edit. With none chosen, the working directory is copied. The browser only shows folders inside your home directory, and the server refuses any other path. The API command is comma-separated argv, as in `kaxa create --api-cmd`. Validation matches [create](create.md).

A provider that adjusts compute adds a Resources section with the same choices as the `kaxa create` wizard: Docker offers a size (micro to xl, as fixed CPU and memory pairs), Modal offers CPU and memory with an Other value for a custom number, and Blaxel offers memory plus the disk share of memory (10 to 95 percent). The default option leaves the provider default. A provider without these options shows no Resources section. Template, network block, and sandbox environment variables stay on the `create` command.

The sandbox page has a Shell section. Open terminal opens a new browser tab with a bash shell in the sandbox, the same one `kaxa console` gives you. The tab is the terminal and nothing else: it fills the window, connects on load, follows its size, and ends the shell when you close it. After `exit` or a dropped connection, the terminal says so; press Enter to open a new shell. Each tab is its own shell, with no limit per sandbox. The same section shows the command to copy for your own terminal, such as `kaxa --provider sprites ssh my-sandbox`; see [console](console.md).

The terminal is a WebSocket on `127.0.0.1`. The page asks for a single-use ticket that expires in 30 seconds, and the socket is refused without it, with a `Host` that is not the printed address, or with a foreign `Origin`. The log shows when a terminal opens and closes, never what was typed. The page serves xterm.js from the binary, with no network access. Its inline styles are the one reason `style-src` allows `'unsafe-inline'`; scripts stay limited to the page's own origin.

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

`kaxa ui` writes one line to stderr for each create and destroy step, for each action that changes a sandbox, and for each error a provider returns. A line has the time, a level, the action, `provider/sandbox`, and the detail:

```text
14:02:30  info   create    sprites/teste-ui          started
14:02:30  info   create    sprites/teste-ui          creating teste-ui on sprites…
14:03:05  info   create    sprites/teste-ui          done in 35.1s
14:03:40  info   destroy   sprites/teste-ui          started
14:03:40  info   destroy   sprites/teste-ui          confirmation matches teste-ui
14:03:40  info   destroy   sprites/teste-ui          looking up the sandbox
14:03:41  info   destroy   sprites/teste-ui          found, status running
14:03:41  info   destroy   sprites/teste-ui          destroying on sprites
14:03:43  info   destroy   sprites/teste-ui          provider accepted the destroy
14:03:43  info   destroy   sprites/teste-ui          checking that it is gone
14:03:44  info   destroy   sprites/teste-ui          gone from sprites
14:03:44  info   destroy   sprites/teste-ui          done in 3.9s
14:04:10  error  restart   e2b/api-1                 sandbox not found
14:04:12  warn   list      -                         daytona: <message from the provider>
14:05:00  warn   request   -                         missing csrf token
```

Reading the list or one sandbox does not log unless it fails. Requests rejected for their `Host`, session, or CSRF token log a `warn` line without the value they sent. Provider tokens, the session token, and the CSRF token never appear in the log.

The folder browser lists names of folders under your home directory, never file contents. Symlinks that leave your home directory are hidden and refused.

Provider tokens stay in the `kaxa ui` process. They are not sent to the browser, written into the address, or included in errors. A request whose `Host` header is not the printed address is rejected.
