# kaxa url

Set Sprites URL authentication. This command is only supported for the sprites provider. Modal is rejected before any API call.

```bash
kaxa url <name> <public|sprite>
```

`public` lets anyone with the address open the URL. `sprite` requires a Sprites account. No other value is accepted. `public` is refused for a sandbox created with `--running` and no API, because its URL serves a tmux terminal that has no login. Sandboxes created before this check existed carry no marker, so keep their URL on `sprite`.

On success, kaxa prints the URL and the access mode. A failed update leaves the sandbox in place.

```bash
kaxa url my-sprite public
kaxa url my-sprite sprite
```

`--public` on [create](create.md) sets `public` once, after an API service is running. Use `kaxa url` to switch an existing sandbox. In [list](list.md), Enter on the detail view opens a Dracula select for `public` and `sprite`.
