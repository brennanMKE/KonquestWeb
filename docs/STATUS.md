# Status and open work

Where the work stands, for picking it up in a new session. Host details stay in `.env` (gitignored).

## Done recently
- Replay viewer, playlists of saved games, full-page loop.
- Rooms survive restarts (`-state`).
- Live demo on the start page (`/api/demo`).
- Self-hosting: `DEPLOY.md`, `scripts/deploy.sh` with `.env`.
- Order bar for keyboard and gamepad play: From → To → Ships → Send (README, "No mouse needed").
- System 1 load limits:
  - `-system1-concurrency`, `-system1-wait`, `-system1-games 10`
  - Laya `--max-queue`, 503 busy, `X-Deadline-Ms`
  - counts in Laya's `/stats` and the server's "System 1 use" log line

## Not deployed yet
- The order bar and the System 1 limits are on `main` but not on either deployed host.
  `scripts/deploy.sh all` ships the Parallax side.
- Laya's new queue limit needs `deploy/laya/` redeployed by whoever runs the model host.

## Open ideas
- Arcade cabinet mode: see [arcade-plan.md](arcade-plan.md). The order bar covers keyboard and gamepad play. The rest
  (attract mode, letter entry, hot-seat) waits on whether a cabinet is planned.
- Git history still holds old host details. Removing them needs a history rewrite and a force push. Not done.
- A computers-only game once seemed to stall on a computer's turn during a parallel e2e run. It didn't reproduce.
- Earlier ideas: more aggressive AI, starting clusters, an animated forces chart, a lobby faction picker.

## Resuming on another Mac
1. `git clone git@github.com:brennanMKE/Parallax.git` (or `git pull`), then `cd web && npm ci` and `make build`.
2. Copy `.env` from the old machine; it's gitignored and holds the deploy targets.
3. Optional, for local System 1: install System 1 Arcade's Laya; `scripts/restart.sh` finds it.
4. The ssh aliases in `.env` must resolve on the new machine, with the same keys.
