# kaxa list

List sandboxes. `ls` is an alias. The command takes no arguments.

```bash
kaxa list
kaxa ls
kaxa list --provider modal
kaxa list --provider e2b
kaxa list --provider docker
kaxa list --provider daytona
kaxa list --provider blaxel
```

Without `--provider`, list shows the sandboxes of every provider that has credentials, one table with a `Provider` column. A provider without credentials is skipped. `KAXA_DEFAULT_PROVIDER` does not narrow the list; only `--provider` does. With `--provider`, list shows that provider only, and missing credentials are an error.

If one provider fails while another succeeds, list prints the sandboxes it got and a `warning: could not list <provider>` line on stderr. If every provider fails, list returns the errors. With no provider configured, list says so and names the variables to set.

When there are no sandboxes, list prints `no sandboxes created` and exits.

In a terminal, list opens a table. Highlight a row and press Enter to show details. The fields and the help line depend on the provider of that sandbox. Sprites shows region, CPU, URL auth, restart, and upgrade. Modal shows the Modal sandbox ID only, without restart, upgrade, or URL auth. E2B shows template, resources, and restart when the list API returns them, but not URL auth or upgrade. Docker, Daytona, and Blaxel show what their GET returns (image, lifecycle, previews, and so on). Labels appear when the provider reports them.

| Key | Action |
| --- | --- |
| `enter` | Open the URL auth form when the provider supports it (`public` or `sprite`; Sprites only today) |
| `r` | Restart when the provider supports it |
| `u` | Upgrade when the provider supports it (Sprites only today) |
| `c` | Open an interactive shell, the same as [kaxa console](console.md) (`ssh`); when the shell closes, list returns to the table on that row |
| `d` | Destroy, after confirmation |
| `esc` | Back to the table |
| `q` | Quit |

When a key is not available for the provider, list stays on the detail screen and prints the same message as the standalone command (for example `url auth is not available for modal`). Press `esc` to clear it and return to the table.

The URL auth form is the same kind of select as create, with the Dracula theme. It asks for `[y/N]` and uses the same update as [kaxa url](url.md). A failed update leaves the sandbox in place.

Each action runs on the provider of the sandbox you opened.

`d` still asks before it destroys. `--force` on [destroy](destroy.md) does not apply to this key.

When stdout is not a terminal, list prints plain columns and does not open the table.
