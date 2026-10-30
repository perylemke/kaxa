# kaxa create

Create a sandbox. `new` is an alias.

```bash
kaxa create [name]
kaxa new [name]
```

Without a name, a form asks for the name, provider, and the same yes/no choices as the flags. When API is yes, the form asks for the command and the port. The form uses the Dracula theme. It does not ask for `--dir`; pass that flag if you need it.

Sprites, Modal, and E2B implement create. `--running` and public URLs exist only for Sprites; Modal and E2B reject them with a message that names the providers that have them. API publish works on Sprites and E2B. `--sync` works on all three; see [Sync](#sync).

Modal sandboxes use a Debian image, live for up to 24 hours, and stop after one idle hour. Names are DNS labels. See [Modal account and token](modal-account.md).

E2B sandboxes live for one hour after the last command and then pause, keeping their disk. Names are DNS labels. See [E2B account and API key](e2b-account.md).

## Flags

| Flag | Meaning |
| --- | --- |
| `--running` | Keep the sandbox up for one hour, serve tmux on its URL, and open that URL in a browser |
| `--sync` | Copy the current Git repository into `/home/sprite/workspace/<repo>` (Sprites) `/home/modal/workspace/<repo>` (Modal), or `/home/user/workspace/<repo>` (E2B) |
| `--dir` | Git repository to copy. Requires `--sync`. Default: the current directory |
| `--api` | Publish one HTTP service on the Sprites URL, or on the E2B sandbox URL for the API port. Requires `--api-cmd` |
| `--api-cmd` | Comma-separated argv. The first item is the binary |
| `--api-port` | Port the process listens on. Default `8000`. `8080` is reserved when `--running` is also set |
| `--api-env` | Environment variable for the service: `KEY=VALUE`, or `KEY` to copy the value from your shell. Repeat for more. Requires `--api` |
| `--public` | Make the Sprites URL reachable without an account. Refused with `--running` unless `--api` is set |
| `--template` | E2B only: the template to start the sandbox from. Default: `KAXA_E2B_TEMPLATE`, then `base`. See [Templates on E2B](#templates-on-e2b) |

`--api-cmd`, `--api-env`, and a changed `--api-port` require `--api`. `--dir` requires `--sync`.

## Sync

`--sync` copies the Git repository that contains the start directory, including `.git` and excluding ignored files. A subdirectory still copies the whole repository. The root selects Go, Rust, Python, Node, Ruby, and Clojure; missing tools are installed, then project dependencies. A `Gemfile` runs `bundle install` (`--frozen` with a `Gemfile.lock`), and installs Ruby when `bundle` is missing. A `deps.edn` runs `clojure -P`, and installs the Clojure CLI, plus a JDK when `java` is missing. `project.clj` alone is not detected. A failed copy or install deletes the sandbox.

The copy leaves out credentials: untracked files such as `.env` or `id_rsa` stop the sync with an error, and `.git/config` is stripped of URL credentials and `http.extraHeader`. See [sync](sync.md#limits).

`--dir` must exist and be a directory. A relative path is resolved from the current directory. A missing path, a file, or a directory outside a Git repository fails before create.

```bash
kaxa create --sync my-sprite
kaxa create --sync --dir ~/projects/api-python my-sprite
```

`exec` and `console` on a sandbox created with `--sync` start in the workspace: `/home/sprite/workspace` on Sprites, `/home/modal/workspace` on Modal, `/home/user/workspace` on E2B.

### Sync on Modal

```bash
kaxa create --provider modal --sync my-box
```

A Modal sandbox created with `--sync` uses a larger image than a plain one: Debian with `ca-certificates`, `curl`, `git`, and `build-essential`, plus an unprivileged user `modal` whose home is `/home/modal`. The repository lands in `/home/modal/workspace/<repo>`, owned by `modal`. Toolchains (Go, Rust, Python `uv`, Node, Ruby, Clojure) are installed under `/home/modal/.local` and `/home/modal/.cargo` and put on the `PATH` of the shell, the same way as on Sprites. Downloads are checked against pinned SHA-256 hashes.

`exec` and `console` on a synced Modal sandbox run as `modal`, not root, starting in `/home/modal/workspace`. A Modal sandbox created without `--sync` is unchanged: root, starting in the image root.

Modal builds the sync image the first time you use `--sync` and caches it, so that first create can take a few minutes. Later creates start as fast as a plain sandbox, plus the copy and the install.

### Sync on E2B

```bash
kaxa create --provider e2b --sync my-box
```

An E2B sandbox uses E2B's `base` template, which already has `curl`, `git`, and build tools. The repository lands in `/home/user/workspace/<repo>`, owned by `user`. Toolchains are installed under `/home/user/.local` and `/home/user/.cargo` and put on the `PATH` of `exec` and `console`, the same way as on Modal. Downloads are checked against pinned SHA-256 hashes.

The E2B `base` template has 512 MiB of memory, and E2B sets memory per template, not per sandbox. Compiling a large package there is killed by the kernel (`signal: killed`). So on the `base` template the install step only downloads Go and Rust dependencies (`go mod download`, `cargo fetch`) instead of building them. The first build runs later, with `exec`; if it is killed too, use a template with more memory (see below). With `--template`, the install step runs the normal build.

### Templates on E2B

```bash
kaxa create --provider e2b --template my-big-template --sync my-box
export KAXA_E2B_TEMPLATE=my-big-template   # default for every E2B create
```

A template is a sandbox image you build outside kaxa, with the [E2B CLI or SDK](https://e2b.dev/docs/template/quickstart), where you pick the CPU, the memory, and the tools already installed. kaxa only starts sandboxes from it. The `--template` flag wins over `KAXA_E2B_TEMPLATE`, and `base` (or neither) means E2B's default.

kaxa expects the same layout as `base`: user `user`, home `/home/user`, workspace `/home/user/workspace`. A template made with `fromBaseImage()` has it. Another user breaks `exec`, `console`, `--sync`, and `--api`, and kaxa does not detect that. A name that does not exist fails with `template "<name>" not found`; run `e2b template list` to see the names. Other providers reject `--template`, and `list_providers` reports `template` as a capability only for E2B.

`exec` and `console` on an E2B sandbox created with `--sync` start in `/home/user/workspace`. Without `--sync` they start in `/home/user`. The sandbox pauses an hour after the last command; the next `exec`, `console`, or `sync` resumes it.

## API

`--api` starts one service named `kaxa-api` after the copy. `--api-cmd` is comma-separated argv. The process must listen on `0.0.0.0` and on `--api-port`.

With `--sync`, the repository must look like an HTTP API. A CLI project is rejected before the sandbox is created. `--api` without `--sync` does not inspect a local project.

Create waits until `sprite-env services get kaxa-api` reports `running`, for up to 90 seconds, because Rails and the JVM start slowly. `failed` stops immediately. If the service does not become `running`, the sandbox is deleted.

With `--running` and `--api` together, tmux still starts and the URL forwards to the API. Port `8080` stays with tmux.

```bash
kaxa create --sync --api --public \
  --api-cmd .venv/bin/uvicorn,app.main:app,--host,0.0.0.0,--port,8000 \
  my-sprite
```

### API on E2B

On E2B, `--api` runs the service as `user` in the repository directory (or `/home/user` without `--sync`) and waits for the port to accept connections, for up to 90 seconds. If the process exits first, kaxa deletes the sandbox and shows its last output. The service log is not kept.

E2B sandboxes are private. `--api` is what opens the API port to the internet, at `https://<port>-<sandbox id>.e2b.app`, and E2B fixes that when the sandbox is created. `list` shows the URL. Anyone who has the URL can call the service. `--public` does not apply, because the URL is already open on that one port. `restart` stops the service; start it again with `exec`, or create the sandbox again.

## API environment

`--api-env` adds variables to the `kaxa-api` process, after the toolchain `PATH`. Use `KEY=VALUE`, or `KEY` to copy the value from the shell that runs kaxa. The second form keeps passwords out of the command line and shell history. A missing `KEY` fails before the sandbox is created.

```bash
export DATABASE_URL=postgres://app:secret@db.example.com:5432/app
kaxa create --sync --api --api-env DATABASE_URL --api-env REDIS_URL \
  --api-cmd go,run,./cmd/api my-sprite
```

Values cannot contain commas or control characters, because the provider joins variables with commas. `HOME`, `USER`, `PATH`, and any name starting with `KAXA_` are rejected, so the provider token never reaches the sandbox. Errors name the variable, never the value.

## Public URL

`--public` sets the Sprites URL auth to `public` after the service is running. `--running` without `--api` serves a tmux terminal on the URL that has no login, so `--public` is refused for it. A failed auth change leaves the sandbox in place. Change it later with [kaxa url](url.md).

## What gets deleted

A failed sync, dependency install, or API publish deletes the sandbox. A failed `--public` update does not.
