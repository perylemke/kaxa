# kaxa list

List sandboxes. `ls` is an alias. The command takes no arguments.

```bash
kaxa list
kaxa ls
kaxa list --provider modal
kaxa list --provider e2b
kaxa list --provider docker
```

Without `--provider`, list shows the sandboxes of every provider that has credentials, one table with a `Provider` column. A provider without credentials is skipped. `KAXA_DEFAULT_PROVIDER` does not narrow the list; only `--provider` does. With `--provider`, list shows that provider only, and missing credentials are an error.

If one provider fails while another succeeds, list prints the sandboxes it got and a `warning: could not list <provider>` line on stderr. If every provider fails, list returns the errors. With no provider configured, list says so and names the variables to set.

When there are no sandboxes, list prints `no sandboxes created` and exits.

In a terminal, list opens a table. Highlight a row and press Enter to show details. From the detail view:

| Key | Action |
| --- | --- |
| `enter` | Open the URL auth form (`public` or `sprite`) |
| `r` | Restart |
| `u` | Upgrade |
| `d` | Destroy, after confirmation |
| `esc` | Back to the table |
| `q` | Quit |

The auth form is the same kind of select as create, with the Dracula theme. It asks for `[y/N]` and uses the same update as [kaxa url](url.md). Sprites only. A failed update leaves the sandbox in place. `r` and `u` are also Sprites only (`r` works on E2B too, `u` does not); Modal reports that the feature is not available.

Each action runs on the provider of the sandbox you opened.

`d` still asks before it destroys. `--force` on [destroy](destroy.md) does not apply to this key.

When stdout is not a terminal, list prints plain columns and does not open the table.
