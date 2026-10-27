# sbox url

Set Sprites URL authentication. This command is only supported for the sprites provider. Modal is rejected before any API call.

```bash
sbox url <name> <public|sprite>
```

`public` lets anyone with the address open the URL. `sprite` requires a Sprites account. No other value is accepted.

On success, sbox prints the URL and the access mode. A failed update leaves the sandbox in place.

```bash
sbox url my-sprite public
sbox url my-sprite sprite
```

`--public` on [create](create.md) sets `public` once, after an API service is running. Use `sbox url` to switch an existing sandbox. In [list](list.md), Enter on the detail view opens a Dracula select for `public` and `sprite`.
