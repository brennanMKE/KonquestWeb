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

### `scripts/deploy.sh`: photon and yoyo

```sh
scripts/deploy.sh photon            # build main, ship it, restart, check
scripts/deploy.sh yoyo
scripts/deploy.sh both
scripts/deploy.sh photon --check    # only check what's live
scripts/deploy.sh photon --rollback # the release before this one
scripts/deploy.sh yoyo --no-build   # ship bin/parallax-linux-arm64 as it is
```

**Only Brennan runs deploys.** They are production changes. An agent may write and test the script, build the binary,
and run `--check`, but must not deploy or roll back, and must not ssh to either host to change anything.

What a deploy does:

1. Builds from `main`. The checkout must be clean and match `origin/main`, so commit and push first.
2. Copies the binary to `/tmp/parallax-<commit>` on the host and checks its sha256 there.
3. Installs the binary the way the host expects, then restarts the `parallax` unit:
   - **photon:** `/opt/parallax/releases/<commit>/parallax` (root:root 0755), `restorecon`, then
     `/opt/parallax/current` links to it. It keeps the live release and the 3 before it.
   - **yoyo:** `/usr/local/bin/parallax`, with the previous build kept as `parallax.prev`.
4. Checks `/healthz` locally and through the public URL, `/og.jpg`, a 404 from an unknown room's `/replay`, and the
   unit's journal (`reopened saved rooms`, `System 1 model is up`, errors).

Safety:

- It refuses to restart a unit whose `ExecStart` lacks `-state`, because without it a restart would end every open game.
- It refuses `--rollback` to a build older than e847c8e. Those builds don't know `-state`, so they wouldn't start under
  the current unit.

| Host | URL | Port | Units, config and secrets owned by |
| --- | --- | --- | --- |
| photon (EC2, arm64) | https://parallax.sstools.co | 127.0.0.1:8114, behind Apache | the AWS repo, `~/Developer/AWS` (session aws-30) |
| yoyo (Raspberry Pi 5, tailnet) | https://yoyo.sawfish-byzantine.ts.net | 127.0.0.1:8080, behind `tailscale serve` | the Homelab repo, `~/Developer/Homelab` (session homelab-b8) |

The script replaces only the binary. Never touch these from this repo:

- systemd units and their drop-ins
- photon's port registry (`/etc/photon`)
- `/etc/parallax`: it holds the System 1 key. Never read, copy or log it.
- Apache vhosts and certificates
- `/etc/hosts`, or anything Tailscale
- the Laya model service on yoyo
- saved rooms in `/var/lib/parallax`: they hold seat tokens. Never copy or print them.

Changes to any of these go to the session that owns them. If a build needs a new flag in the unit, add it to the unit
there before the deploy.

On yoyo, the Homelab repo's own `yoyo/parallax/deploy.sh` pins a sha256. After a deploy from here, that pin is behind:
its script refuses to run until the pin is updated.

The System 1 model runs on yoyo (`deploy/laya/`). photon's computer players call it over the tailnet with a Bearer key.
If the model is unreachable, they play as Planner.
