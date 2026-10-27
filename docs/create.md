# kaxa create

Create a sandbox. `new` is an alias.

```bash
kaxa create [name]
kaxa new [name]
```

Without a name, a form asks for the name, provider, and the same yes/no choices as the flags. When API is yes, the form asks for the command and the port. The form uses the Dracula theme. It does not ask for `--dir`; pass that flag if you need it.

Sprites implements create. Sync, API publish, and public URLs are rejected for Modal.

## Flags

| Flag | Meaning |
| --- | --- |
| `--running` | Keep the sandbox up for one hour, serve tmux on its URL, and open that URL in a browser |
| `--sync` | Copy the current Git repository into `/home/sprite/workspace/<repo>` |
| `--dir` | Git repository to copy. Requires `--sync`. Default: the current directory |
| `--api` | Publish one HTTP service on the Sprites URL. Requires `--api-cmd` |
| `--api-cmd` | Comma-separated argv. The first item is the binary |
| `--api-port` | Port the process listens on. Default `8000`. `8080` is reserved when `--running` is also set |
| `--api-env` | Environment variable for the service: `KEY=VALUE`, or `KEY` to copy the value from your shell. Repeat for more. Requires `--api` |
| `--public` | Make the Sprites URL reachable without an account |

`--api-cmd`, `--api-env`, and a changed `--api-port` require `--api`. `--dir` requires `--sync`.

## Sync

`--sync` copies the Git repository that contains the start directory, including `.git` and excluding ignored files. A subdirectory still copies the whole repository. The root selects Go, Rust, Python, Node, Ruby, and Clojure; missing tools are installed, then project dependencies. A `Gemfile` runs `bundle install` (`--frozen` with a `Gemfile.lock`), and installs Ruby when `bundle` is missing. A `deps.edn` runs `clojure -P`, and installs the Clojure CLI, plus a JDK when `java` is missing. `project.clj` alone is not detected. A failed copy or install deletes the sandbox.

`--dir` must exist and be a directory. A relative path is resolved from the current directory. A missing path, a file, or a directory outside a Git repository fails before create.

```bash
kaxa create --sync my-sprite
kaxa create --sync --dir ~/projects/api-python my-sprite
```

`exec` and `console` on a sandbox created with `--sync` start in `/home/sprite/workspace`.

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

## API environment

`--api-env` adds variables to the `kaxa-api` process, after the toolchain `PATH`. Use `KEY=VALUE`, or `KEY` to copy the value from the shell that runs kaxa. The second form keeps passwords out of the command line and shell history. A missing `KEY` fails before the sandbox is created.

```bash
export DATABASE_URL=postgres://app:secret@db.example.com:5432/app
kaxa create --sync --api --api-env DATABASE_URL --api-env REDIS_URL \
  --api-cmd go,run,./cmd/api my-sprite
```

Values cannot contain commas or control characters, because the provider joins variables with commas. `HOME`, `USER`, `PATH`, and any name starting with `KAXA_` are rejected, so the provider token never reaches the sandbox. Errors name the variable, never the value.

## Public URL

`--public` sets the Sprites URL auth to `public` after the service is running. A failed auth change leaves the sandbox in place. Change it later with [kaxa url](url.md).

## What gets deleted

A failed sync, dependency install, or API publish deletes the sandbox. A failed `--public` update does not.
