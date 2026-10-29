# kaxa snapshot

Save the state of a sandbox and bring it back later.

```bash
kaxa snapshot create <name> [--comment text]
kaxa snapshot list [name]
kaxa snapshot restore <name> <snapshot-id> [--to <new-name>] [--force]
kaxa snapshot delete <name> <snapshot-id> [--force]
```

`list` has the alias `ls` and `delete` has the alias `rm`. Every message names the provider, for example `snapshot v7 of sandbox demo on sprites was created`. Add `--provider` or set `KAXA_DEFAULT_PROVIDER` to pick the provider. `list` is the exception: without `--provider` it searches every configured provider.

A provider without snapshots answers `snapshot is not available for <provider>` and names the providers that have it, as `restart` and `upgrade` do. The check runs before any request leaves your machine.

## Subcommands

| Subcommand | What it does |
| --- | --- |
| `create <name>` | Saves the filesystem of the sandbox. `--comment` stores a note of at most 256 bytes without control characters |
| `list [name]` | Prints `PROVIDER`, `SANDBOX`, `ID`, `CREATED` (UTC, RFC 3339), and `COMMENT`. Without a name it lists every snapshot of every configured provider; with a name, the snapshots of that sandbox in each provider that has it. `--provider` limits the search to one provider. Rows are grouped by provider, then sandbox, newest first. With none it prints `no snapshots` or `no snapshots for <name>` |
| `restore <name> <snapshot-id>` | Brings a snapshot back. What that means depends on the provider, see below |
| `delete <name> <snapshot-id>` | Deletes a snapshot. Asks `[y/N]` unless `--force` or `-f` is set |

```text
PROVIDER  SANDBOX    ID      CREATED               COMMENT
modal     my-box     im-abc  2026-01-02T04:04:05Z  nightly
sprites   my-sprite  v7      2026-01-02T03:04:05Z  before the migration
```

A provider that fails to answer is reported as `warning: could not list snapshots on <provider>: ...` on stderr, and the others are still listed. The command fails only when every provider fails. A provider without snapshots is skipped. Sprites checkpoints belong to a sprite, so listing without a name asks once per sprite.

A snapshot ID is one segment of letters, digits, dots, underscores, and hyphens. Anything else, such as `../x`, is rejected before a request is made.

## What restore means

| Provider | Restore | `--to` | Snapshot is |
| --- | --- | --- | --- |
| Sprites | In place: the same sandbox returns to the snapshot state and what changed after it is lost | Rejected | A checkpoint, such as `v7` |
| Modal | New sandbox: kaxa creates the sandbox named by `--to` and keeps the source | Required | A filesystem image, such as `im-...` |

`kaxa mcp` reports this as `snapshot_restore` (`in_place` or `new_sandbox`) in `list_providers`, so a new provider only has to declare its mode.

### Sprites

A restore overwrites the sandbox, so it asks `[y/N]` first; `--force` skips the question. Answering no prints `cancelled` and changes nothing.

```bash
kaxa snapshot create my-sprite --comment "before the migration" --provider sprites
kaxa snapshot list my-sprite --provider sprites
kaxa snapshot restore my-sprite v7 --provider sprites
kaxa snapshot delete my-sprite v7 --provider sprites
```

A Sprites checkpoint does not include `/tmp`, which is in memory. Keep what you want to save under `/home/sprite`, `/opt`, or `/var`. Modal snapshots hold the whole filesystem.

Deleting a checkpoint uses `DELETE /v1/sprites/{name}/checkpoints/{id}`. That route is not in the public Sprites API reference, so a failure names the status code. `create` finds the new checkpoint by listing before and after, because the create stream does not return its ID.

### Modal

Modal does not restore in place. `restore` creates a new sandbox from the image, so `--to` is required and must differ from the source. The source stays until you run `kaxa destroy`.

```bash
kaxa snapshot create my-box --provider modal
kaxa snapshot list my-box --provider modal
kaxa snapshot restore my-box im-abc123 --to my-box-copy --provider modal
kaxa snapshot delete my-box im-abc123 --provider modal
```

- A sandbox created with `--sync` is restored with the same labels, so `exec` and `console` still run as `modal` in `/home/modal/workspace`.
- Only filesystem snapshots are used. Modal's memory snapshots are experimental and are not supported.
- Modal keeps a snapshot image for 30 days from its creation. kaxa does not expose a TTL. A restore of an expired image fails and drops the stale entry.
- The Modal SDK cannot list images, so kaxa keeps its own index of the snapshots it took in `modal-snapshots.json` under your user config directory (`~/Library/Application Support/kaxa` on macOS, `$XDG_CONFIG_HOME/kaxa` or `~/.config/kaxa` on Linux, `%AppData%\kaxa` on Windows). The directory is mode `0700` and the file `0600`. Entries older than 30 days are dropped.
- The index is per machine. `list` shows only snapshots taken on this machine, and two kaxa processes writing at the same moment can lose an entry because the file is not locked. `list` works after the sandbox has been destroyed.

## destroy keeps snapshots

`kaxa destroy` does not delete snapshots. On Modal a snapshot is the way to recover a sandbox that has finished, so it stays until you run `snapshot delete` or Modal expires it. Sprites checkpoints belong to the sprite and go with it when you destroy it.

## Security

A snapshot copies the whole filesystem. Secrets written inside the sandbox, such as tokens in a file or in shell history, are stored in the snapshot and come back on restore. Do not put secrets in the comment. kaxa never prints Sprites or Modal tokens in messages or errors.

## Agents

`kaxa mcp` has the same operations as tools: `create_snapshot`, `list_snapshots`, `restore_snapshot`, and `delete_snapshot`. See [agent](agent.md).
