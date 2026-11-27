# kaxa console

Open an interactive bash shell in a sandbox. `ssh` is an alias.

```bash
kaxa console <name>
kaxa ssh <name>
```

The shell requires a terminal. Without a terminal, run a command with [kaxa exec](exec.md).

```bash
kaxa console my-sprite
kaxa ssh my-sprite
```

The remote exit code is the kaxa exit code, from 0 through 255.

## E2B

Pick the provider with `--provider e2b`. The shell runs as `user` with the terminal size of your window, and resizing the window is applied while the session is open. A paused sandbox is resumed first. E2B merges standard error into standard output in a terminal session.

```bash
kaxa --provider e2b console my-box
```

## Modal

Pick the provider with `--provider modal` or `KAXA_DEFAULT_PROVIDER=modal`. The sandbox must be running.

```bash
kaxa --provider modal console my-box
```

Differences from Sprites:

- The terminal size is applied when the shell starts. Resizing the window during the session is not applied, because the Modal SDK cannot resize a running terminal. Run `stty rows <n> cols <m>` in the shell, or reopen the console, after resizing.
- Modal merges standard error into standard output in a terminal session.
- Without `--sync` the shell runs as root in the root directory of the sandbox image. With `--sync` it runs as `modal` in `/home/modal/workspace`.

## Docker

Pick the provider with `--provider docker`. kaxa opens a process with a terminal in the sandbox, sizes it from your window and follows resizing. The shell runs as `agent`. A stopped sandbox is started first.

```bash
kaxa --provider docker console my-box
```

## Daytona

Pick the provider with `--provider daytona`. kaxa opens a process with a terminal in the sandbox in the sandbox, sizes it from your window and follows resizing. The shell runs as `daytona`, and the first line you see may be the echo of the command kaxa sends to start it. A stopped sandbox is started first.

```bash
kaxa --provider daytona console my-box
```

## Blaxel

Pick the provider with `--provider blaxel`. kaxa attaches to the terminal the sandbox starts. It does not type `bash`, so the shell is the one in the Blaxel dashboard. The window size is applied when the session opens, and resizing the window is forwarded. With `--sync` the shell starts in `/root/workspace`. Without it, the shell starts in `/root`. The shell runs as `root`.

Fifteen minutes without input closes the terminal, and the sandbox can return to standby. Files and processes remain. Open the console again to continue. The image must include the Blaxel sandbox API. The default image does.

```bash
kaxa --provider blaxel console my-box
```

## Vercel

`kaxa console my-box --provider vercel` (also `kaxa ssh`) opens the same terminal as the Connect tab in the Vercel dashboard. It is not SSH: kaxa asks the Vercel API for a terminal, checks its address, and connects to it over a secure WebSocket, so no key or port is needed. The shell is `bash` (`sh` if bash is missing) as `vercel-sandbox`, in `/vercel/sandbox/workspace` with `--sync` and in `/vercel/sandbox` without it, with the toolchain `PATH` set. The window size is applied at start and resizing is forwarded. `exit` ends the session; the sandbox keeps running and kaxa extends the session while the terminal is open. A stopped sandbox is resumed first, which can take a moment. The connection token stays in memory and is not printed. If the project has no interactive endpoint, kaxa says so and suggests `kaxa exec`.

```bash
kaxa --provider vercel console my-box
```
