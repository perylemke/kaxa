# Vendored terminal assets

`kaxa ui` embeds these files so the page needs no network and no CDN. They are
loaded only when a terminal is opened (the CSS is linked from `index.html`).

| File | Package | Version | License |
| --- | --- | --- | --- |
| `xterm.js`, `xterm.css` | `@xterm/xterm` | 6.0.0 | MIT, `LICENSE-xterm` |
| `addon-fit.js` | `@xterm/addon-fit` | 0.11.0 | MIT, `LICENSE-addon-fit` |

Taken from the npm tarballs (sha256 of the tarball):

- `@xterm/xterm@6.0.0`: `908e66e04af6c8dc6b00dd3b54de088e2e81e5ed866284fd6c2fb3c2d1c7a3f6`
- `@xterm/addon-fit@0.11.0`: `26003b4517a132b64e4ff228fd88a5fda3fff5e606c76093f6dcff772e9ecec0`

The only change is that the trailing `//# sourceMappingURL=` comment was removed
from the two `.js` files, because the source maps are not shipped.

To update: `npm pack @xterm/xterm @xterm/addon-fit`, copy `lib/xterm.js`,
`css/xterm.css`, `lib/addon-fit.js` and the licenses, strip the source map
comment, update this table, and run `make test`.
