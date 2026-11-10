# AGENTS.md

Notes for coding agents working on Parallax. [README.md](README.md) explains the game, [SPEC.md](SPEC.md) the design and
the API, and [RULES.md](RULES.md) the exact rules.

## Ground rules

- MIT licensed and written clean-room. Never read, copy or adapt GPL source, including KDE Konquest's.
- Go server in `cmd/` and `internal/`; Svelte client in `web/`. The server is authoritative: the client only shows
  what the server sends and sends intents.

## Build and test

```sh
make test                 # Go tests (use `go test -race ./...` before committing)
cd web && npm run check   # TypeScript and Svelte checks
make e2e                  # Playwright browser tests: builds everything and runs a server on :8787
make build                # bin/server with the client embedded, and bin/sim
make linux-arm64          # bin/parallax-linux-arm64, the build both hosts run
make shots                # regenerate web/public/og.jpg and docs/images/ from a seeded game (needs ffmpeg for the GIF)
```

## Scripts

Scripts live in `scripts/`.

### `scripts/restart.sh`: the local server

It stops the running local server, rebuilds, and serves on http://localhost:8080. It also starts the local System 1
model (Laya) on :8799 if it's installed, and keeps open rooms in `.run/rooms` (`-state`) so they survive the restart.

- `--no-build` restarts the current build. Any other arguments go to the server, for example `-system1 off`.
- Brennan plays on this server while work goes on. Rooms come back after a restart, but a game is still interrupted
  briefly. Check the server's log for a game in progress (`game started` without a later `game over`) and ask before
  restarting.

### `scripts/deploy.sh`: deployed servers

```sh
scripts/deploy.sh <target>            # build main, ship it, restart, check
scripts/deploy.sh all                 # every target
scripts/deploy.sh <target> --check    # only check what's live
scripts/deploy.sh <target> --rollback # the build before this one
scripts/deploy.sh <target> --no-build # ship the existing build as it is
```

The targets, and their ssh aliases, URLs, ports and install layouts, are in `.env` at the repo root (gitignored).
[.env.example](.env.example) shows the format. Host names, addresses, ports and network details never go in tracked
files: not in scripts, docs, tests or commit messages. Write `game.example.com` in examples. [DEPLOY.md](DEPLOY.md) is
the guide for anyone running their own server; keep it working for a stranger.

**Only Brennan runs deploys.** They are production changes. An agent may write and test the script, build the binary,
and run `--check`, but must not deploy or roll back, and must not ssh to either host to change anything.

What a deploy does:

1. Builds from `main`. The checkout must be clean and match `origin/main`, so commit and push first.
2. Copies the binary to `/tmp/parallax-<commit>` on the host and checks its sha256 there.
3. Installs it according to the target's layout, then restarts the unit:
   - **releases:** `DIR/releases/<commit>`, then `DIR/current` links to it, with `restorecon` where SELinux runs.
     It keeps the live release and the 3 before it.
   - **single:** the binary is replaced, and the previous one is kept as `.prev`.
4. Checks `/healthz` on the host and through the public URL, `/og.jpg`, `/api/demo`, a 404 from an unknown room's
   `/replay`, and the unit's journal. URLs are left out of the printed journal lines.

Safety:

- It refuses to restart a unit whose `ExecStart` lacks `-state`, because without it a restart would end every open game.
- It refuses `--rollback` to a build older than e847c8e. Those builds don't know `-state`, so they wouldn't start under
  the current unit.

The script replaces only the binary. Never touch any of these from this repo:

- systemd units and their drop-ins
- port registries
- the System 1 key's environment file: never read, copy or log it
- proxy or VPN configuration, and certificates
- `/etc/hosts`
- the model service
- saved rooms: they hold seat tokens. Never copy or print them.

Brennan's own hosts have their configuration in separate ops repos, each looked after by its own agent session. Changes
to any of the above go there. If a build needs a new flag in a unit, add it there before the deploy.
