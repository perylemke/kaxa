# kaxa sync

Copy another Git repository into a sandbox that already exists. This works for Sprites, Modal, E2B, Docker, Daytona, and Blaxel.

```bash
kaxa sync <name> [--dir <path>]
```

The sandbox must have been created with `--sync`; see [create](create.md#sync). `sync` copies the Git repository that contains `--dir`, or the current directory when `--dir` is omitted, including `.git` and excluding ignored files. A subdirectory still copies the whole repository. `--dir` may point at any local repository, and a relative path is resolved from the current directory.

```bash
kaxa sync my-sprite
kaxa sync --dir ~/projects/api-python my-sprite
kaxa --provider modal sync my-box
kaxa --provider e2b sync my-box
```

The repository lands in `/home/sprite/workspace/<repo>` on Sprites, `/home/modal/workspace/<repo>` on Modal, owned by `modal`, `/home/user/workspace/<repo>` on E2B, owned by `user`, `/home/agent/workspace/<repo>` on Docker, owned by `agent`, `/home/daytona/workspace/<repo>` on Daytona, owned by `daytona`, and `/root/workspace/<repo>` on Blaxel, owned by `root`. On Blaxel the writable layer is memory, so a large repository may need a higher `KAXA_BLAXEL_MEMORY`. `<repo>` is the name of the local directory. After the copy, kaxa installs the toolchain and dependencies it detects at the repository root, the same way as `create --sync`. A toolchain that is already installed is reused. If `curl` is missing, kaxa installs it with `apt-get` or `apk` before the download. An image that already has `curl` skips that step. On success kaxa prints the directory inside the sandbox.

`--no-install` copies the repository and skips the toolchain and the dependency commands, then prints the commands to run them by hand, for example `kaxa exec <name> -- sh -c 'cd <dir> && go mod download'`. The image has to provide the tools those commands need. Use it when the sandbox is too small to install everything at once, or when the image cannot download a toolchain. `create --sync --no-install` does the same at create time.

## Limits

- Every sandbox has a workspace, so one created without `--sync` accepts sync too. Modal is the exception: a Modal sandbox only has a workspace when it was created with `--sync`, because the image with the `modal` user, `git`, and the build tools is picked when the sandbox is created and kaxa cannot change it later. A Modal sandbox without it is rejected: `sandbox <name> was not created with sync`. Create a new one with `--sync`.
- `sync` never replaces a directory. If `<repo>` already exists in the sandbox, it stops before copying with `<repo> already exists in sandbox <name>`. Two local repositories with the same directory name collide; rename one, or remove the old copy with `kaxa exec <name> -- rm -rf <path>`.
- A failed copy or install keeps the sandbox, because it holds other work. `create --sync` deletes a sandbox it just made; `sync` does not. After a failed install the files are already in the sandbox. Fix the cause and run the install there with `kaxa exec`, or remove the directory and sync again.
- `sync` copies the repository once. It does not update a copy that is already there.
- A `.git` file, used by linked worktrees, is rejected, and submodule directories are omitted, as in `create --sync`.
- `sync` refuses an untracked file that looks like a credential: `.env` (but not `.env.example`), `*.pem`, `*.key`, `*.p12`, `id_rsa*` and the other SSH private keys, `.netrc`, `.npmrc`, `.pypirc`, and `credentials*.json`. The error names the file. Add it to `.gitignore` or remove it. A tracked file is copied, because it is already in the history.
- The copied `.git/config` has no credentials: user and password in `http(s)` URLs, `http.extraHeader`, and `credential` sections are removed, and `.git/credentials` is not copied. Remotes and branches stay, so `git fetch` works over SSH or after you sign in again in the sandbox.
- An AI agent does the same with the `sync_repository` tool. See [kaxa mcp](agent.md#sync_repository).
