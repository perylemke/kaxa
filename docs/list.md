# sbox list

List sandboxes for the selected provider. `ls` is an alias. The command takes no arguments.

```bash
sbox list
sbox ls
```

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

The auth form is the same kind of select as create, with the Dracula theme. It asks for `[y/N]` and uses the same update as [sbox url](url.md). Sprites only. A failed update leaves the sandbox in place.

`d` still asks before it destroys. `--force` on [destroy](destroy.md) does not apply to this key.

When stdout is not a terminal, list prints plain columns and does not open the table.
