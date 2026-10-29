# kaxa destroy

Destroy one or more sandboxes. `rm` is an alias. Destroying a sprite deletes its persisted state.

```bash
kaxa destroy [name]
kaxa rm [name]
```

## Flags

| Flag | Meaning |
| --- | --- |
| `--all` | Destroy every sandbox. Takes no name. Covers every configured provider unless `--provider` is set |
| `--provider` | Limit the scope to one provider (`sprites`, `modal`). Without `--all` it also picks the provider for a name |
| `--force`, `-f` | Skip every confirmation |

## How a target is chosen

| Command | What it destroys | Confirmation |
| --- | --- | --- |
| `kaxa destroy my-box` | That sandbox on the selected provider (`--provider`, `KAXA_DEFAULT_PROVIDER`, or `sprites`) | `[y/N]` |
| `kaxa destroy` | The sandboxes you select from every configured provider | The selection form |
| `kaxa destroy --all --provider modal` | Every sandbox on that provider | `[y/N]` |
| `kaxa destroy --all` | Every sandbox on every configured provider | Two `[y/N]` prompts |

Messages always name the provider, for example `sandbox demo on modal was destroyed`.

### No name: pick from every provider

With no name and without `--all`, a form lists the sandboxes of every provider that has credentials, as `name  status  provider`. Providers without credentials are skipped. Add `--provider` to see only one.

The form uses the Dracula theme. In a terminal, select rows and confirm in the form. When stdin is not a terminal, enter numbers to toggle and `0` to confirm. The form is the confirmation, so `--force` does not skip it.

### `--all`: two confirmations

`--all` without `--provider` is the widest scope kaxa has, so it asks twice:

```text
Destroy all 5 sandboxes across modal, sprites? [y/N]:
This cannot be undone. Really destroy all 5 sandboxes across modal, sprites? [y/N]:
```

Anything other than `y` or `yes` at either prompt prints `destroy cancelled` and nothing is destroyed. With `--provider`, only that provider is affected and a single prompt is asked.

If a provider fails to list, kaxa prints a warning and `--all` covers only the providers that answered. If one destroy fails, the run stops there and the error names the sandbox; the ones already destroyed stay destroyed.

The `d` key in [list](list.md) still asks. It does not read `--force`.

```bash
kaxa destroy                           # pick from every provider
kaxa destroy my-sprite
kaxa destroy --provider modal my-box
kaxa destroy --all                     # everything, two confirmations
kaxa destroy --all --provider modal    # only Modal, one confirmation
kaxa destroy -f --all                  # everything, no confirmation
```

Empty `--all` prints `no sandboxes to destroy`.

## Snapshots

`destroy` does not delete snapshots. A Modal snapshot is an image that outlives its sandbox, so you can restore it into a new sandbox after the source is gone; it stays until you run `kaxa snapshot delete` or Modal expires it after 30 days. A sprite takes its persisted state with it. See [snapshot](snapshot.md).
