# kaxa destroy

Destroy one or more sandboxes. `rm` is an alias. Destroying a sprite deletes its persisted state.

```bash
kaxa destroy [name]
kaxa rm [name]
```

## Flags

| Flag | Meaning |
| --- | --- |
| `--all` | Destroy every sandbox. Takes no name |
| `--force`, `-f` | Skip the `[y/N]` confirmation |

## How a target is chosen

A name destroys that sandbox after `[y/N]`, unless `--force` or `-f` is set.

`--all` lists every sandbox and asks `Destroy all N sandboxes?` unless forced.

With no name and without `--all`, a form lets you select one or more sandboxes. The form uses the Dracula theme. In a terminal, select rows and confirm in the form. When stdin is not a terminal, enter numbers to toggle and `0` to confirm. That form is the confirmation; `--force` does not skip it.

The `d` key in [list](list.md) still asks. It does not read `--force`.

```bash
kaxa destroy my-sprite
kaxa destroy --force my-sprite
kaxa destroy -f --all
```

Empty `--all` prints `no sandboxes to destroy`.
