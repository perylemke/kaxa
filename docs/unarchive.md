# kaxa unarchive

Bring an archived Blaxel sandbox back.

```bash
kaxa unarchive [name]
```

With a name, unarchive asks `[y/N]` and then brings that sandbox back. Answering no prints `unarchive cancelled` and does not call the provider. Without a name, unarchive opens a picker. Choose a number or a name from the list. `q` cancels the picker.

```bash
kaxa unarchive my-box --provider blaxel
kaxa unarchive
```

Unarchive is available for Blaxel. Other providers print that it is not available and name the providers that have it.

Archive and standby are different. A sandbox in standby is already `running`: the next `exec` or `console` resumes it with its files and processes, and there is no `restart`. An archived sandbox keeps its filesystem and drops its processes. `exec`, `console`, and `sync` do not unarchive it. They tell you to run `kaxa unarchive`. After unarchive, start a published service again with `exec`.

See [Blaxel](blaxel-account.md).
